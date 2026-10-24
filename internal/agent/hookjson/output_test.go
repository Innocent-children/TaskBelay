package hookjson

import (
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func TestRender(t *testing.T) {
	cases := []struct {
		name string
		hook string
		d    event.Decision
		want string
	}{
		{"不做什么时不输出", "PreToolUse", event.Decision{}, ""},
		{
			"拒绝工具调用，中文写成转义", "PreToolUse",
			event.Decision{Deny: "代码没变😀", Notice: "已拦下"},
			`{"systemMessage":"\u5df2\u62e6\u4e0b","hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"\u4ee3\u7801\u6ca1\u53d8\ud83d\ude00"}}`,
		},
		{
			"阻止结束", "Stop",
			event.Decision{Block: "x", Notice: "y"},
			`{"decision":"block","reason":"x","systemMessage":"y"}`,
		},
		{
			"补充上下文", "SessionStart",
			event.Decision{Context: "c"},
			`{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"c"}}`,
		},
	}
	for _, c := range cases {
		got, err := Render(c.hook, c.d)
		if err != nil {
			t.Fatalf("%s：%v", c.name, err)
		}
		if string(got) != c.want {
			t.Errorf("%s：\n得到 %s\n应为 %s", c.name, got, c.want)
		}
	}

	if _, err := Render("Stop", event.Decision{Context: "c"}); err == nil {
		t.Error("Stop 补充上下文应该报错")
	}
	if _, err := Render("UserPromptSubmit", event.Decision{Deny: "d"}); err == nil {
		t.Error("UserPromptSubmit 拒绝工具调用应该报错")
	}
}
