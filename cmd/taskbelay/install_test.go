package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestLifecycleCommandsWithoutTerminal(t *testing.T) {
	root := t.TempDir()
	env := platform.Env{HomeDir: root, DataDir: filepath.Join(root, ".taskbelay")}
	for _, args := range [][]string{nil, {"--help"}, {"--version"}} {
		var output bytes.Buffer
		if err := run(t.Context(), args, env, root, &output); err != nil || !strings.Contains(output.String(), "TaskBelay") {
			t.Fatalf("args=%v, output=%q, err=%v", args, output.String(), err)
		}
	}
	var output bytes.Buffer
	if err := run(t.Context(), []string{"menu"}, env, root, &output); err == nil {
		t.Fatal("menu accepted non-terminal output")
	}
	if _, err := os.Stat(env.DataDir); !os.IsNotExist(err) {
		t.Fatalf("help/version/menu created a ledger: %v", err)
	}
}

func TestShellInstallerRepeatAndFailedDownloadPreserveUserFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer; Windows uses install.ps1")
	}
	root := filepath.Join(t.TempDir(), "用户 space ' path")
	fakeBin := filepath.Join(root, "fixture-bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(root, "downloads")
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("#!/bin/sh\nprintf 'TaskBelay v0.5.1 (local)\\n'\n")
	asset := "taskbelay-" + runtime.GOOS + "-" + runtime.GOARCH
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(fixture, asset), payload)
	write(filepath.Join(fixture, "SHA256SUMS"), fmt.Appendf(nil, "%x  %s\n", sha256.Sum256(payload), asset))
	metadata := fmt.Sprintf(`{"version":"v0.5.1","revision":%q,"assets":[%q,"SHA256SUMS"]}`, strings.Repeat("a", 40), asset)
	write(filepath.Join(fixture, "release-manifest.json"), []byte(metadata))
	write(filepath.Join(fakeBin, "curl"), []byte(`#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) destination="$2"; shift 2 ;;
    https://*) url="$1"; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */release-manifest.json|*/download/v0.5.1/*) ;;
  *) echo 'Download was not pinned to metadata version' >&2; exit 1 ;;
esac
cp "$TASKBELAY_TEST_DOWNLOADS/${url##*/}" "$destination"
`))
	write(filepath.Join(fakeBin, "go"), []byte(`#!/bin/sh
if [ "$1" = version ]; then
  printf '\tpath\t%s\n' "$TASKBELAY_TEST_MODULE"
  exit 0
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then destination="$2"; break; fi
  shift
done
cp "$TASKBELAY_TEST_BINARY" "$destination"
`))
	write(filepath.Join(root, ".zshrc"), []byte("# existing user profile\n"))
	write(filepath.Join(root, ".bash_profile"), []byte("# existing login profile\n"))
	script, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	modulePath := "github.com/Innocent-children/taskbelay/cmd/taskbelay"
	runInstaller := func(args ...string) error {
		t.Helper()
		cmd := exec.Command("bash", append([]string{script}, args...)...)
		cmd.Env = append(os.Environ(), "HOME="+root, "ZDOTDIR="+root, "PATH="+fakeBin+":/usr/bin:/bin", "TASKBELAY_TEST_DOWNLOADS="+fixture, "TASKBELAY_TEST_BINARY="+filepath.Join(fixture, asset), "TASKBELAY_TEST_MODULE="+modulePath)
		output, err := cmd.CombinedOutput()
		if !utf8.Valid(output) {
			t.Fatalf("installer output is not UTF-8: %q", output)
		}
		if err != nil {
			t.Logf("installer output: %s", output)
		}
		return err
	}
	for _, args := range [][]string{nil, nil, {"--source"}} {
		if err := runInstaller(args...); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(root, ".local", "bin", "taskbelay")
	for _, invalid := range []string{
		strings.Replace(metadata, "v0.5.1", "v00.5.1", 1),
		strings.Replace(metadata, fmt.Sprintf("%q,", asset), "", 1),
		strings.Replace(metadata, `"version":`, `"unknown":`, 1),
		strings.TrimSuffix(metadata, "}"),
		metadata + `{}`,
	} {
		write(filepath.Join(fixture, "release-manifest.json"), []byte(invalid))
		if err := runInstaller(); err == nil {
			t.Fatalf("accepted invalid metadata: %s", invalid)
		}
		if installed, err := os.ReadFile(target); err != nil || !bytes.Equal(installed, payload) {
			t.Fatalf("metadata failure changed installation: %q, %v", installed, err)
		}
	}
	write(filepath.Join(fixture, "release-manifest.json"), []byte(metadata))
	profile, err := os.ReadFile(filepath.Join(root, ".zshrc"))
	if err != nil || !strings.Contains(string(profile), "# existing user profile") || strings.Count(string(profile), "export PATH=") != 1 {
		t.Fatalf("profile not preserved or PATH duplicated: %q, %v", profile, err)
	}
	login, err := os.ReadFile(filepath.Join(root, ".bash_profile"))
	if err != nil || !strings.Contains(string(login), "# existing login profile") || strings.Count(string(login), "export PATH=") != 1 {
		t.Fatalf("active Bash login profile not updated: %q, %v", login, err)
	}
	write(filepath.Join(fixture, asset), []byte("corrupted download"))
	if err := runInstaller(); err == nil {
		t.Fatal("accepted corrupt package")
	}
	installed, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(installed, payload) {
		t.Fatalf("previous executable not preserved: %q, %v", installed, err)
	}
	legacy := filepath.Join(root, "previous-go-program")
	if err := os.Rename(target, legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(target), ".taskbelay-install")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacy, target); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(fixture, asset), payload)
	modulePath = "example.com/other-program"
	if err := runInstaller(); err == nil {
		t.Fatal("replaced an unrecognized program link")
	}
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unrecognized link not preserved: %v", err)
	}
	modulePath = "github.com/Innocent-children/taskbelay/cmd/taskbelay"
	if err := runInstaller(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("known TaskBelay link not replaced: %v", err)
	}
	if data, err := os.ReadFile(legacy); err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("original Go program changed: %q, %v", data, err)
	}
	if files, _ := filepath.Glob(filepath.Join(root, ".local", "bin", ".taskbelay-stage.*")); len(files) != 0 {
		t.Fatalf("staging directory retained: %v", files)
	}
	if _, err := os.Stat(filepath.Join(root, ".local", "bin", ".taskbelay-install.lock")); !os.IsNotExist(err) {
		t.Fatalf("installer lock retained: %v", err)
	}
}
