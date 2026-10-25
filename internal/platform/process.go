package platform

import (
	"context"
	"errors"
	"os"
	"time"
)

type ProcessCommand struct {
	Path                  string
	Args                  []string
	Dir                   string
	Env                   []string
	Stdin, Stdout, Stderr *os.File
}

type ProcessResources struct {
	UserCPU    time.Duration `json:"user_cpu_ns"`
	SystemCPU  time.Duration `json:"system_cpu_ns"`
	MaxRSS     int64         `json:"reported_maxrss"`
	MaxRSSUnit string        `json:"maxrss_unit"`
}

type ProcessEvidence struct {
	Started   bool              `json:"started"`
	Duration  time.Duration     `json:"duration_ns"`
	Resources *ProcessResources `json:"resources,omitempty"`
}

/**
 * stdout、stderr 使用本次分析的文件，避免子进程继承管道造成 Wait 悬挂。
 * 每条命令结束也回收其进程组，不让后台子进程存活到下一条变异。
 */
func RunProcess(ctx context.Context, command ProcessCommand) (code int, evidence ProcessEvidence, resultErr error) {
	started := time.Now()
	defer func() { evidence.Duration = time.Since(started) }()
	if err := ctx.Err(); err != nil {
		return -1, evidence, err
	}
	process, err := startProcess(command)
	if err != nil {
		return -1, evidence, err
	}
	evidence.Started = true
	defer process.close()
	defer func() { evidence.Resources = process.resources() }()
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() { code, err := process.wait(); done <- result{code, err} }()
	select {
	case completed := <-done:
		return completed.code, evidence, errors.Join(completed.err, process.terminate())
	case <-ctx.Done():
		killErr := process.terminate()
		completed := <-done
		return completed.code, evidence, errors.Join(ctx.Err(), killErr)
	}
}
