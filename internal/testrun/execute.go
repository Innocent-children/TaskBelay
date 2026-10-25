package testrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

type cancelReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r cancelReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

const outputLimit = 4 << 20

type Executor struct {
	Root, Output string
	Workspace    string
	Env          []string
	mu           sync.Mutex
	stages       []Stage
}

func (e *Executor) Records() []Stage {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Stage(nil), e.stages...)
}

func (e *Executor) Execute(ctx context.Context, stage, dir string, argv, env []string) (result Result) {
	started := time.Now()
	result.ExitCode = -1
	result.Process = &platform.ProcessEvidence{}
	defer func() {
		result.Duration = time.Since(started)
		e.mu.Lock()
		e.stages = append(e.stages, Stage{Name: stage, Result: result})
		e.mu.Unlock()
	}()
	fail := func(err error) Result { result.Failure, result.Reason = EnvironmentFailure, err.Error(); return result }
	if len(argv) == 0 {
		return fail(fmt.Errorf("没有可执行的测试命令"))
	}
	stdout, err := os.CreateTemp(e.Output, "stdout-*")
	if err != nil {
		return fail(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(e.Output, "stderr-*")
	if err != nil {
		return fail(err)
	}
	defer stderr.Close()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return fail(err)
	}
	defer stdin.Close()
	environment := mergeEnvironment(e.Env, env)
	program, err := lookupProgram(argv[0], dir, environment)
	if err != nil {
		return fail(err)
	}
	if runtime.GOOS == "windows" && (strings.EqualFold(filepath.Ext(program), ".cmd") || strings.EqualFold(filepath.Ext(program), ".bat")) {
		var quoted []string
		for _, argument := range append([]string{program}, argv[1:]...) {
			quoted = append(quoted, "'"+strings.ReplaceAll(argument, "'", "''")+"'")
		}
		program, err = lookupProgram("powershell", dir, environment)
		if err != nil {
			return fail(err)
		}
		argv = []string{program, "-NoProfile", "-NonInteractive", "-Command", "& " + strings.Join(quoted, " ") + "; exit $LASTEXITCODE"}
	}
	result.ExitCode, *result.Process, err = platform.RunProcess(ctx, platform.ProcessCommand{Path: program, Args: argv[1:], Dir: dir, Env: environment, Stdin: stdin, Stdout: stdout, Stderr: stderr})
	var truncated bool
	result.Output, result.Truncated = readOutput(stdout)
	result.ErrorOutput, truncated = readOutput(stderr)
	result.Truncated = result.Truncated || truncated
	if err != nil {
		result.Failure, result.Reason = EnvironmentFailure, err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			result.Failure = Timeout
		}
		if errors.Is(err, context.Canceled) {
			result.Failure = Canceled
		}
	} else if result.ExitCode != 0 {
		result.Failure, result.Reason = EnvironmentFailure, "命令未成功完成"
		if stage == "compile" {
			result.Failure = CompileFailure
		}
	}
	return result
}

func readOutput(file *os.File) (string, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", true
	}
	data, err := io.ReadAll(io.LimitReader(file, outputLimit+1))
	if err != nil {
		return string(data), true
	}
	if len(data) > outputLimit {
		return string(data[:outputLimit]), true
	}
	return string(data), false
}

func mergeEnvironment(base, extra []string) []string {
	values := map[string]string{}
	for _, entries := range [][]string{base, extra} {
		for _, entry := range entries {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				if runtime.GOOS == "windows" {
					key = strings.ToUpper(key)
				}
				values[key] = value
			}
		}
	}
	var result []string
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func lookupProgram(name, dir string, env []string) (string, error) {
	if filepath.IsAbs(name) {
		return name, nil
	}
	if strings.ContainsAny(name, `/\`) {
		return filepath.Join(dir, name), nil
	}
	var path, extensions string
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") {
			path = value
		}
		if strings.EqualFold(key, "PATHEXT") {
			extensions = value
		}
	}
	suffixes := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		if extensions == "" {
			extensions = ".COM;.EXE;.BAT;.CMD"
		}
		suffixes = strings.Split(extensions, ";")
	}
	for _, folder := range filepath.SplitList(path) {
		if folder == "" {
			continue
		}
		if !filepath.IsAbs(folder) {
			folder = filepath.Join(dir, folder)
		}
		for _, suffix := range suffixes {
			candidate := filepath.Join(folder, name+suffix)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0111 != 0) {
				return candidate, nil
			}
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func (e *Executor) Prepare(ctx context.Context, command string) Result {
	if strings.TrimSpace(command) == "" {
		return Result{}
	}
	args := []string{"sh", "-c", command}
	if runtime.GOOS == "windows" {
		args = []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", command}
	}
	return e.Execute(ctx, "prepare", e.Root, args, nil)
}
