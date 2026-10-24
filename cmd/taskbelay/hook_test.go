package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 走一遍 taskbelay hook 和 taskbelay status：记下的消息能在 status 里看到原文；
// 不在 git 工作区里时什么都不输出；账本写不进去时照常退出、不输出内容，错误写进日志。
func TestHookAndStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init：%v %s", err, out)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	hook := func(cwd, prompt string) (code int, stdout, stderr string) {
		input := `{"session_id":"s-123","prompt_id":"p1","cwd":` + jsonString(cwd) + `,"hook_event_name":"UserPromptSubmit","prompt":` + jsonString(prompt) + `}`
		var out, errOut bytes.Buffer
		code = run([]string{"hook", "claude", "user-prompt-submit"}, strings.NewReader(input), &out, &errOut)
		return code, out.String(), errOut.String()
	}

	if code, out, errOut := hook(sub, "给 export 命令加一个导出 CSV 的选项"); code != 0 || out != "" || errOut != "" {
		t.Fatalf("hook 退出码 %d，输出 %q，错误输出 %q", code, out, errOut)
	}
	t.Chdir(repo)
	var out bytes.Buffer
	if code := run([]string{"status"}, nil, &out, &out); code != 0 {
		t.Fatalf("status 退出码 %d：%s", code, out.String())
	}
	for _, want := range []string{"s-123", "给 export 命令加一个导出 CSV 的选项"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status 里没有 %q：\n%s", want, out.String())
		}
	}

	if code, out, errOut := hook(t.TempDir(), "不在工作区里"); code != 0 || out != "" || errOut != "" {
		t.Errorf("不在工作区里：退出码 %d，输出 %q，错误输出 %q", code, out, errOut)
	}

	if os.Geteuid() == 0 {
		t.Skip("root 用户不受只读权限限制")
	}
	if err := os.Chmod(filepath.Join(home, ".taskbelay", "taskbelay.db"), 0o444); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := hook(repo, "账本只读"); code != 0 || out != "" || errOut != "" {
		t.Errorf("账本只读：退出码 %d，输出 %q，错误输出 %q", code, out, errOut)
	}
	logFile := filepath.Join(home, ".taskbelay", "logs", time.Now().Format(time.DateOnly)+".log")
	if data, _ := os.ReadFile(logFile); !strings.Contains(string(data), "hook 失败") {
		t.Errorf("日志里没有这次错误：\n%s", data)
	}
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
