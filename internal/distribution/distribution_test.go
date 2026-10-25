package distribution

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

func managedFixture(t *testing.T) (platform.Env, string) {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", filepath.Join(root, "AppData", "Local"))
	}
	env := platform.Env{HomeDir: root, DataDir: filepath.Join(root, ".taskbelay")}
	bin := platform.UserBinDir(env)
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, platform.ExecutableName())
	for path, data := range map[string]string{exe: "old version", filepath.Join(bin, ".taskbelay-install"): marker} {
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return env, exe
}

func TestUpdateVerifiesDownloadBeforeReplacingProgram(t *testing.T) {
	for _, scenario := range []string{"success", "download failure", "checksum mismatch", "missing checksum", "duplicate checksum", "busy"} {
		t.Run(scenario, func(t *testing.T) {
			env, exe := managedFixture(t)
			asset := "taskbelay-" + runtime.GOOS + "-" + runtime.GOARCH
			if runtime.GOOS == "windows" {
				asset += ".exe"
			}
			payload := []byte("new program")
			manifest := fmt.Sprintf("%x  %s\n", sha256.Sum256(payload), asset)
			switch scenario {
			case "checksum mismatch":
				payload = []byte("corrupted download")
			case "missing checksum":
				manifest = strings.ReplaceAll(manifest, asset, "other-platform")
			case "duplicate checksum":
				manifest += manifest
			case "busy":
				if err := os.Mkdir(filepath.Join(filepath.Dir(exe), ".taskbelay-install.lock"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/SHA256SUMS" {
					fmt.Fprint(w, manifest)
				} else if r.URL.Path == "/"+asset && scenario != "download failure" {
					w.Write(payload)
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			_, err := update(t.Context(), env, exe, server.URL+"/", server.Client())
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected update result: %v", err)
			}
			want := "old version"
			if scenario == "success" {
				want = string(payload)
			}
			data, err := os.ReadFile(exe)
			if err != nil || string(data) != want {
				t.Fatalf("program = %q, error = %v", data, err)
			}
			if files, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".taskbelay-download-*")); len(files) != 0 {
				t.Fatalf("temporary downloads retained: %v", files)
			}
		})
	}
}

func TestUninstallPreservesDataAndRejectsUnmanagedProgram(t *testing.T) {
	env, exe := managedFixture(t)
	if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(env.DataDir, "taskbelay.db")
	if err := os.WriteFile(dataFile, []byte("user ledger"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(env.HomeDir, platform.ExecutableName())
	if err := os.WriteFile(other, []byte("other install"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(env, other); err == nil {
		t.Fatal("accepted unmanaged executable")
	}
	mark := filepath.Join(filepath.Dir(exe), ".taskbelay-install")
	if err := os.WriteFile(mark, []byte("foreign marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(env, exe); err == nil {
		t.Fatal("accepted foreign marker")
	}
	if err := os.WriteFile(mark, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(env, exe); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Fatalf("program not removed: %v", err)
	}
	if data, err := os.ReadFile(dataFile); err != nil || string(data) != "user ledger" {
		t.Fatalf("ledger changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(other); err != nil || string(data) != "other install" {
		t.Fatalf("other program changed: %q, %v", data, err)
	}
}
