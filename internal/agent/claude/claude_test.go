package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestParseEvent(t *testing.T) {
	exit := func(n int) *int { return &n }
	common := `"session_id":"s1","prompt_id":"p1","cwd":"/repo",`
	cases := []struct {
		hook, input string
		want        event.Event
	}{
		{
			"pre-tool-use", `{` + common + `"tool_name":"PowerShell","tool_input":{"command":"go test ./..."},"tool_use_id":"u1"}`,
			event.Event{Kind: event.CommandRequest, Command: "go test ./...", Syntax: event.PowerShell, ToolUseID: "u1"},
		},
		{
			"pre-tool-use", `{` + common + `"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/repo/a.ipynb"}}`,
			event.Event{Kind: event.EditRequest, Files: []string{"/repo/a.ipynb"}},
		},
		{
			"post-tool-use-failure", `{` + common + `"tool_name":"Bash","tool_use_id":"u1","error":"Exit code 2\nFAIL","is_interrupt":false}`,
			event.Event{Kind: event.CommandDone, ToolUseID: "u1", Result: event.Fail, ExitCode: exit(2)},
		},
		{
			"post-tool-use-failure", `{` + common + `"tool_name":"Bash","tool_use_id":"u1","error":"Exit code 130","is_interrupt":true}`,
			event.Event{Kind: event.CommandDone, ToolUseID: "u1", Result: event.Unknown},
		},
		{
			"post-tool-use", `{` + common + `"tool_name":"Bash","tool_use_id":"u1","tool_response":{"stdout":"ok","interrupted":false}}`,
			event.Event{Kind: event.CommandDone, ToolUseID: "u1", Result: event.Pass, ExitCode: exit(0)},
		},
	}
	for _, c := range cases {
		got, err := Adapter{}.ParseEvent(c.hook, []byte(c.input))
		if err != nil {
			t.Fatalf("%s：%v", c.input, err)
		}
		c.want.Agent, c.want.Session, c.want.Turn, c.want.Dir = "claude", "s1", "p1", "/repo"
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s：\n得到 %+v\n应为 %+v", c.input, got, c.want)
		}
	}
}

// 写入两次不重复，用户原有的配置、键的顺序和文件权限保持原样；卸载后回到安装前的样子。
// 直接写配置文件，不检查本机的 Claude Code 版本。
func TestInstallUninstall(t *testing.T) {
	env := platform.Env{Home: t.TempDir()}
	path := settingsPath(env)
	original := `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "my-guard.sh"}]}
    ]
  },
  "permissions": {"allow": ["Bash(go test:*)"]}
}`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(env.Home, "go", "bin", platform.ExeName)
	for range 2 {
		if err := hookjson.EditHooks(path, ours, entries(exe)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"hook",`); n != 7 {
		t.Errorf("TaskBelay 的 hook 有 %d 个，应为 7 个：\n%s", n, data)
	}
	if !strings.Contains(string(data), "my-guard.sh") {
		t.Errorf("用户原有的 hook 不见了：\n%s", data)
	}
	if strings.Index(string(data), `"model"`) > strings.Index(string(data), `"permissions"`) {
		t.Errorf("键的顺序变了：\n%s", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("文件权限变成了 %v", info.Mode().Perm())
	}

	if _, err := (Adapter{}).Uninstall(env); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(original), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("卸载后和安装前不同：\n%s", data)
	}
}
