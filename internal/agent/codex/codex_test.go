package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestParseEvent(t *testing.T) {
	patch := "*** Begin Patch\r\n*** Update File: a.go\r\n@@\r\n-x\r\n+y\r\n*** Update File: old.go\n*** Move to: new.go\n*** Add File: 新.go\n+z\n*** Delete File: b.go\n*** End Patch"
	input := `{"session_id":"s1","turn_id":"t1","cwd":"/repo","tool_name":"apply_patch","tool_input":{"command":` + quote(patch) + `}}`
	got, err := Adapter{}.ParseEvent("pre-tool-use", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	want := event.Event{
		Kind: event.EditRequest, Agent: "codex", Session: "s1", Turn: "t1", Dir: "/repo",
		Files: []string{"a.go", "old.go", "new.go", "新.go", "b.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("得到 %+v\n应为 %+v", got, want)
	}
}

// quote 把 s 写成 JSON 字符串。
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// 安装两次不重复；卸载后 hooks.json 里没有 TaskBelay 的条目，也不留下空的 hooks。
func TestInstallUninstall(t *testing.T) {
	env := platform.Env{Home: t.TempDir()}
	exe := filepath.Join(env.Home, "张 三", "go", "bin", platform.ExeName)
	for range 2 {
		if _, err := (Adapter{}).Install(env, exe); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(hooksPath(env))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), " hook codex "); n != 6 {
		t.Errorf("TaskBelay 的 hook 有 %d 个，应为 6 个：\n%s", n, data)
	}

	if _, err := (Adapter{}).Uninstall(env); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(hooksPath(env))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Errorf("卸载后应为空对象：\n%s", data)
	}
}
