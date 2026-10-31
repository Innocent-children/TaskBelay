package distribution

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installationProgram(t *testing.T, version string) []byte {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	binary := filepath.Join(dir, "taskbelay.exe")
	code := fmt.Sprintf("package main\nimport \"fmt\"\nfunc main(){fmt.Println(%q)}\n", "TaskBelay "+version+" (installation fixture)")
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %s: %v", out, err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestUpdateVersionOrderingAndInvalidBuilds(t *testing.T) {
	payload := installationProgram(t, "v0.5.1")
	for _, current := range []string{"v0.5.0", "v0.5.1-0.20261027023236-bee390c559a8", "v0.5.1", "v0.5.1+dirty", "v0.6.0", "dev", "(devel)", "not-a-version"} {
		t.Run(current, func(t *testing.T) {
			env, exe := managedFixture(t)
			asset := "taskbelay-" + runtime.GOOS + "-" + runtime.GOARCH
			if runtime.GOOS == "windows" {
				asset += ".exe"
			}
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/latest/download/release-manifest.json":
					fmt.Fprintf(w, `{"version":"v0.5.1","revision":%q,"assets":[%q,"SHA256SUMS"]}`, strings.Repeat("a", 40), asset)
				case "/download/v0.5.1/SHA256SUMS":
					fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(payload), asset)
				case "/download/v0.5.1/" + asset:
					w.Write(payload)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			message, err := update(t.Context(), env, exe, current, server.URL+"/", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			upgraded := current == "v0.5.0" || strings.HasPrefix(current, "v0.5.1-0.")
			data, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			if upgraded {
				if string(data) != string(payload) || len(requests) != 3 {
					t.Fatal(message, requests)
				}
			} else if string(data) != "old version" || len(requests) > 1 {
				t.Fatal(message, requests)
			}
			if (current == "dev" || current == "(devel)" || current == "not-a-version") && len(requests) != 0 {
				t.Fatal("invalid current build must not download")
			}
		})
	}
}
