package main

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

/**
 * 仅供原生 Agent 验收隔离账本目录，正式 CLI 的配置路径保持不变。
 * 会话寿命交给客户端管理，避免单次 MCP 单测的期限截断真实模型调用。
 */
func TestEvaluationCLIHelper(t *testing.T) {
	dataDir := os.Getenv("TASKBELAY_EVAL_DATA")
	if dataDir == "" {
		return
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 || separator == len(os.Args)-1 {
		t.Fatal("evaluation CLI arguments are missing")
	}
	workingDir, err := os.Getwd()
	if err == nil {
		err = run(context.Background(), os.Args[separator+1:], platform.Env{DataDir: dataDir}, workingDir, os.Stdout)
	}
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(0)
}
