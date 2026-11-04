package dynamic

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func environmentID(values []string) string {
	m := map[string]string{}
	for _, entry := range values {
		k, v, ok := strings.Cut(entry, "=")
		if ok {
			m[k] = v
		}
	}
	raw, _ := json.Marshal(m)
	one := fmt.Sprintf("%x", sha256.Sum256(raw))
	raw, _ = json.Marshal([]string{one})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (r *runner) commands(ctx context.Context, phase, base string, patches []Patch, commands []Command) ([]RunResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := r.input.Workspace.CaptureTree(ctx)
	if err != nil || !snapshot.Complete() {
		return nil, fmt.Errorf("真实工作区状态无法核对：%v", err)
	}
	if snapshot.Tree != r.input.Tree {
		return nil, fmt.Errorf("真实工作区已变化，停止副本运行")
	}
	limit := RunLimit
	if ctx != r.ctx {
		limit = int(float64(limit) * (1 - ConfirmReserve))
	}
	if r.started+len(commands) > limit {
		return nil, fmt.Errorf("副本进程运行预算已用完")
	}
	r.started += len(commands)
	out := make([]RunResult, len(commands))
	parallel := max(ParallelMin, min(ParallelMax, r.input.Parallel))
	slots := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, c := range commands {
		wg.Add(1)
		go func(i int, c Command) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				out[i] = RunResult{Command: c.ID, Phase: phase, State: "unknown", Reason: ctx.Err().Error()}
				return
			}
			out[i] = r.execute(ctx, phase, base, patches, c)
		}(i, c)
	}
	wg.Wait()
	for _, run := range out {
		r.samples = append(r.samples, repostats.Run{ID: run.Command, Output: run.Output, ExitCode: run.ExitCode, Started: run.Started})
	}
	r.model, err = repostats.ResultsFor(r.input.After, r.samples)
	if err != nil {
		r.result.Runs = append(r.result.Runs, out...)
		return out, err
	}
	for i := range out {
		parsed, parseErr := repostats.ParseResults(ctx, r.model, repostats.Run{ID: out[i].Command, Output: out[i].Output, ExitCode: out[i].ExitCode, Started: out[i].Started})
		if parseErr != nil {
			r.result.Runs = append(r.result.Runs, out...)
			return out, parseErr
		}
		out[i].Cases = parsed
	}
	r.result.Runs = append(r.result.Runs, out...)
	if r.publish != nil {
		if err = r.publish(r.result); err != nil {
			return out, err
		}
	}
	current, err := r.input.Workspace.CaptureTree(ctx)
	if err != nil || !current.Complete() || current.Tree != r.input.Tree {
		return out, fmt.Errorf("副本运行期间真实工作区发生变化或无法核对")
	}
	return out, nil
}

func (r *runner) execute(ctx context.Context, phase, base string, patches []Patch, c Command) RunResult {
	result := RunResult{ID: units.Hash(phase, c.ID, fmt.Sprint(patches)), Command: c.ID, Phase: phase, State: "unknown"}
	if !c.Trusted {
		result.Reason = "命令没有本任务成功运行记录"
		return result
	}
	if environmentID(os.Environ()) != c.Environment {
		result.Reason = "当前环境与已成功命令不同"
		return result
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if reason := commandBudget(ctx, c); reason != "" {
		result.Reason = reason
		return result
	}
	for r.input.Pause != nil {
		paused, err := r.input.Pause(ctx)
		if err != nil {
			result.Reason = err.Error()
			return result
		}
		if !paused {
			break
		}
		timer := time.NewTimer(PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			result.Reason = ctx.Err().Error()
			return result
		case <-timer.C:
		}
	}
	dir, err := os.MkdirTemp(r.root, "run-")
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer os.RemoveAll(dir)
	copyDir := filepath.Join(dir, "tree")
	if err = platform.CloneTree(ctx, base, copyDir); err != nil {
		result.Reason = err.Error()
		return result
	}
	if err = apply(copyDir, patches); err != nil {
		result.Reason = err.Error()
		result.State = "invalid"
		return result
	}
	actualDirectory, err := filepath.EvalSymlinks(c.Directory)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	rel, err := filepath.Rel(r.input.Workspace.Root, actualDirectory)
	if err != nil || rel != "." && !filepath.IsLocal(rel) {
		result.Reason = "命令目录不在当前工作区内"
		return result
	}
	cwd := filepath.Join(copyDir, rel)
	aliasRoot := filepath.Clean(c.Directory)
	if rel != "." {
		for range strings.Split(rel, string(filepath.Separator)) {
			aliasRoot = filepath.Dir(aliasRoot)
		}
	}
	move := func(value string) string {
		return relocate(relocate(value, r.input.Workspace.Root, copyDir), aliasRoot, copyDir)
	}
	program := move(c.Program)
	if !filepath.IsAbs(program) {
		if strings.ContainsAny(program, "/\\") {
			program = filepath.Join(cwd, program)
		} else {
			program, err = exec.LookPath(program)
		}
	}
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	digest, err := platform.ExecutableDigest(ctx, program)
	if err != nil || digest != c.Tool {
		result.Reason = "副本工具内容与已成功运行的工具不同"
		return result
	}
	args := append([]string(nil), c.Args...)
	for i := range args {
		args[i] = move(args[i])
	}
	env := os.Environ()
	for i, entry := range env {
		k, v, ok := strings.Cut(entry, "=")
		if ok {
			env[i] = k + "=" + move(v)
		}
	}
	stdout, err := os.Create(filepath.Join(dir, "output"))
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer stdout.Close()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer stdin.Close()
	var pauseErr error
	outputExceeded := false
	pause := func() bool {
		if info, err := stdout.Stat(); err == nil && info.Size() > OutputBytes {
			outputExceeded = true
			cancel()
		}
		if r.input.Pause == nil {
			return false
		}
		var paused bool
		paused, pauseErr = r.input.Pause(ctx)
		if pauseErr != nil {
			cancel()
		}
		return paused
	}
	// 等待并发槽位、暂停和准备副本消耗同一阶段的预算，启动前再次核对。
	if reason := commandBudget(ctx, c); reason != "" {
		result.Reason = reason
		return result
	}
	code, evidence, runErr := platform.RunProcess(ctx, platform.ProcessCommand{Path: program, Args: args, Dir: cwd, Env: env, Stdin: stdin, Stdout: stdout, Stderr: stdout, LowPriority: true, Pause: pause, Poll: PollInterval})
	result.Started, result.Duration = evidence.Started, evidence.Duration
	result.Resources = evidence.Resources
	_, _ = stdout.Seek(0, io.SeekStart)
	output, readErr := io.ReadAll(io.LimitReader(stdout, OutputBytes))
	result.Output = string(output)
	if runErr != nil || readErr != nil || pauseErr != nil || outputExceeded {
		result.Reason = fmt.Sprintf("运行或输出未完成：%v %v %v；输出预算=%t", runErr, readErr, pauseErr, outputExceeded)
		return result
	}
	result.ExitCode = &code
	if code == 0 {
		result.State = "passed"
	} else {
		result.State = "failed"
	}
	return result
}

func commandBudget(ctx context.Context, c Command) string {
	end, ok := ctx.Deadline()
	if !ok {
		return "动态检查没有可核对的截止时间"
	}
	remaining := max(time.Duration(0), time.Until(end))
	if c.Duration <= 0 {
		return fmt.Sprintf("命令实测耗时未知，未启动；剩余预算=%s", remaining)
	}
	if ctx.Err() != nil || remaining < c.Duration {
		return fmt.Sprintf("剩余预算不足，未启动；剩余预算=%s，真实工作区实测耗时=%s", remaining, c.Duration)
	}
	return ""
}

func relocate(value, source, target string) string { return strings.ReplaceAll(value, source, target) }

func apply(root string, patches []Patch) error {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	for _, p := range patches {
		if !filepath.IsLocal(filepath.FromSlash(p.Path)) {
			return fmt.Errorf("补丁路径超出副本：%s", p.Path)
		}
		old, err := dir.ReadFile(p.Path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if string(old) != p.Before {
			return fmt.Errorf("补丁前置内容不对应：%s", p.Path)
		}
		if p.Remove {
			if err := dir.Remove(p.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := dir.MkdirAll(filepath.Dir(p.Path), 0700); err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if p.Mode == "100755" {
			mode = 0700
		}
		if err := dir.WriteFile(p.Path, []byte(p.After), mode); err != nil {
			return err
		}
	}
	return nil
}

func sorted[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
