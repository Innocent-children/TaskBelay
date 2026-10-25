package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestProcessTreeHelper(t *testing.T) {
	mode := os.Getenv("TASKBELAY_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "resources" || mode == "failed" {
		_, _ = os.Stdout.WriteString("native output\n")
		_, _ = os.Stderr.WriteString("native error\n")
		if mode == "failed" {
			os.Exit(7)
		}
		os.Exit(0)
	}
	if mode == "child" {
		time.Sleep(600 * time.Millisecond)
		_ = os.WriteFile(os.Getenv("TASKBELAY_PROCESS_MARKER"), []byte("escaped"), 0600)
		os.Exit(0)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProcessTreeHelper$")
	command.Env = append(os.Environ(), "TASKBELAY_PROCESS_HELPER=child")
	if err := command.Start(); err != nil {
		os.Exit(2)
	}
	_ = os.WriteFile(os.Getenv("TASKBELAY_PROCESS_READY"), []byte("ready"), 0600)
	if mode == "exit" {
		os.Exit(0)
	}
	time.Sleep(10 * time.Second)
	os.Exit(0)
}

func TestProcessGroupEndsDescendantsOnCancelAndLeaderExit(t *testing.T) {
	for _, mode := range []string{"cancel", "exit"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker, ready := filepath.Join(dir, "escaped"), filepath.Join(dir, "ready")
			input, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := os.Create(filepath.Join(dir, "output"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			type completion struct {
				evidence ProcessEvidence
				err      error
			}
			done := make(chan completion, 1)
			go func() {
				// race 默认在退出前等待一秒；此处需要辅助父进程实际立即退出。
				_, evidence, err := RunProcess(ctx, ProcessCommand{Path: os.Args[0], Args: []string{"-test.run=^TestProcessTreeHelper$"}, Dir: dir, Env: append(os.Environ(), "GORACE=atexit_sleep_ms=0", "TASKBELAY_PROCESS_HELPER="+mode, "TASKBELAY_PROCESS_MARKER="+marker, "TASKBELAY_PROCESS_READY="+ready), Stdin: input, Stdout: output, Stderr: output})
				done <- completion{evidence, err}
			}()
			deadline := time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("helper did not start child")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if mode == "cancel" {
				cancel()
			}
			completed := <-done
			err = completed.err
			checkProcessEvidence(t, completed.evidence)
			if mode == "cancel" && !errors.Is(err, context.Canceled) || mode == "exit" && err != nil {
				t.Fatal(err)
			}
			time.Sleep(650 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("descendant survived command", err)
			}
		})
	}
}

func checkProcessEvidence(t *testing.T, evidence ProcessEvidence) {
	t.Helper()
	if !evidence.Started || evidence.Duration <= 0 {
		t.Fatal("native start or elapsed time missing", evidence)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if evidence.Resources != nil {
			t.Fatal("unverified platform resources were filled", evidence)
		}
		return
	}
	usage := evidence.Resources
	if usage == nil || usage.UserCPU < 0 || usage.SystemCPU < 0 || usage.UserCPU+usage.SystemCPU <= 0 || usage.MaxRSS <= 0 {
		t.Fatal("native resource evidence missing", evidence)
	}
	want := "KiB"
	if runtime.GOOS == "darwin" {
		want = "bytes"
	}
	if usage.MaxRSSUnit != want {
		t.Fatal("native RSS unit changed", usage)
	}
	t.Logf("native process evidence: elapsed=%s user=%s system=%s maxrss=%d %s", evidence.Duration, usage.UserCPU, usage.SystemCPU, usage.MaxRSS, usage.MaxRSSUnit)
}

func TestProcessExecutionEvidenceDistinguishesLaunchAndExit(t *testing.T) {
	for _, mode := range []string{"resources", "failed", "missing", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			input, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := os.Create(filepath.Join(dir, "output"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			command := ProcessCommand{Path: os.Args[0], Args: []string{"-test.run=^TestProcessTreeHelper$"}, Dir: dir,
				Env: append(os.Environ(), "GORACE=atexit_sleep_ms=0", "TASKBELAY_PROCESS_HELPER="+mode), Stdin: input, Stdout: output, Stderr: output}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "missing" {
				command.Path = filepath.Join(dir, "absent-program")
			}
			if mode == "canceled" {
				cancel()
			}
			code, evidence, err := RunProcess(ctx, command)
			if mode == "missing" || mode == "canceled" {
				if err == nil || code != -1 || evidence.Started || evidence.Resources != nil {
					t.Fatal("failed launch counted as execution", code, evidence, err)
				}
				return
			}
			checkProcessEvidence(t, evidence)
			want := 0
			if mode == "failed" {
				want = 7
			}
			if err != nil || code != want {
				t.Fatal("native exit changed", code, err)
			}
			data, err := os.ReadFile(output.Name())
			if err != nil || string(data) != "native output\nnative error\n" {
				t.Fatal("native output changed", string(data), err)
			}
		})
	}
}
