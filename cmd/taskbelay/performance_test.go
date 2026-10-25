package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type performanceSample struct {
	Milliseconds float64            `json:"milliseconds"`
	Error        string             `json:"error,omitempty"`
	CheckStatus  ledger.CheckStatus `json:"check_status,omitempty"`
	StaticMillis float64            `json:"static_milliseconds,omitempty"`
	Missing      []ledger.CheckGap  `json:"missing,omitempty"`
	MissingCount int                `json:"missing_count,omitempty"`
	Diagnostics  []string           `json:"diagnostics,omitempty"`
}

type performanceProfile struct {
	Name       string              `json:"name"`
	Samples    []performanceSample `json:"samples"`
	Median     float64             `json:"median_ms"`
	P95        float64             `json:"p95_ms"`
	Budget     float64             `json:"budget_ms"`
	Over       int                 `json:"over_budget"`
	Errors     int                 `json:"errors"`
	Incomplete int                 `json:"incomplete"`
	Degraded   int                 `json:"degraded"`
}

func TestPerformanceProfiles(t *testing.T) {
	text := os.Getenv("TASKBELAY_PERFORMANCE_SAMPLES")
	if text == "" {
		t.Skip("opt-in latency profile; set TASKBELAY_PERFORMANCE_SAMPLES=20")
	}
	count, err := strconv.Atoi(text)
	if err != nil || count < 1 || count > 100 {
		t.Fatal("TASKBELAY_PERFORMANCE_SAMPLES must be between 1 and 100")
	}
	selected := os.Getenv("TASKBELAY_PERFORMANCE_PROFILE")
	profiles := []string{"cold", "warm", "new-files", "large-static", "large-static-unknown", "database-busy"}
	if selected != "" && !slices.Contains(profiles, selected) {
		t.Fatal("unknown performance profile", selected)
	}
	for _, name := range profiles {
		if selected != "" && selected != name {
			continue
		}
		t.Run(name, func(t *testing.T) {
			profile := performanceProfile{Name: name, Budget: float64(event.MessageBudget.Milliseconds())}
			if strings.HasPrefix(name, "large-static") {
				profile.Budget = float64(event.StaticBudget.Milliseconds())
			}
			var dir, data string
			if name == "warm" || name == "new-files" || name == "database-busy" {
				dir, data = performanceWorkspace(t)
				if initial := performanceHook(t, dir, data, "SessionStart", "session-start"); initial.Error != "" {
					t.Fatal("cannot prepare warm profile", initial)
				}
			}
			for sample := 0; sample < count; sample++ {
				var measured performanceSample
				switch name {
				case "cold":
					dir, data = performanceWorkspace(t)
					measured = performanceHook(t, dir, data, "SessionStart", "session-start")
				case "warm":
					measured = performanceHook(t, dir, data, "SessionStart", "session-start")
				case "new-files":
					content := fmt.Sprintf("package main\nvar added%d = %d\n", sample, sample)
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("new-%d.go", sample)), []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
					measured = performanceHook(t, dir, data, "SessionStart", "session-start")
				case "large-static", "large-static-unknown":
					measured = performanceLargeStatic(t, name == "large-static-unknown")
				case "database-busy":
					db, err := sql.Open("sqlite", filepath.Join(data, "taskbelay.db"))
					if err != nil {
						t.Fatal(err)
					}
					conn, err := db.Conn(t.Context())
					if err != nil {
						db.Close()
						t.Fatal(err)
					}
					if _, err := conn.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
						t.Fatal(err)
					}
					measured = performanceHook(t, dir, data, "SessionStart", "session-start")
					_, rollback := conn.ExecContext(t.Context(), "ROLLBACK")
					conn.Close()
					db.Close()
					if rollback != nil {
						t.Fatal(rollback)
					}
					if len(measured.Diagnostics) == 0 {
						measured.Error += "; database contention did not record a diagnostic"
					}
				}
				profile.Samples = append(profile.Samples, measured)
				// 每个样本立即输出，保留进程中断或预算失败前已经完成的证据。
				encoded, _ := json.Marshal(map[string]any{"profile": name, "sample": sample + 1, "measurement": measured})
				t.Logf("TASKBELAY_PERFORMANCE_SAMPLE %s", encoded)
			}
			var durations []float64
			for _, measured := range profile.Samples {
				durations = append(durations, measured.Milliseconds)
				if measured.Milliseconds > profile.Budget {
					profile.Over++
				}
				if measured.Error != "" {
					profile.Errors++
				}
				if len(measured.Diagnostics) > 0 {
					profile.Degraded++
				}
				if measured.CheckStatus != "" && measured.CheckStatus != ledger.CheckComplete {
					profile.Incomplete++
				}
			}
			slices.Sort(durations)
			profile.Median = durations[len(durations)/2]
			if len(durations)%2 == 0 {
				profile.Median = (durations[len(durations)/2-1] + profile.Median) / 2
			}
			profile.P95 = durations[int(math.Ceil(float64(len(durations))*.95))-1]
			encoded, _ := json.Marshal(profile)
			t.Logf("TASKBELAY_PERFORMANCE_PROFILE %s", encoded)
		})
	}
}

func performanceWorkspace(t *testing.T) (string, string) {
	t.Helper()
	dir, data := filepath.Join(t.TempDir(), "项目 space"), t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, data
}

func performanceHook(t *testing.T, dir, data, native, hookName string) performanceSample {
	t.Helper()
	workspace, discoverErr := gitstate.Discover(t.Context(), dir)
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	ctx, cancel := context.WithTimeout(t.Context(), event.StaticBudget+5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHookProcessHelper$")
	cmd.Env = append(os.Environ(), "TASKBELAY_HOOK_TEST_DATA="+data, "TASKBELAY_HOOK_TEST_AGENT=claude", "TASKBELAY_HOOK_TEST_EVENT="+hookName)
	cmd.Stdin = bytes.NewReader(cliHookInput(t, dir, native, nil))
	started := time.Now()
	output, err := cmd.CombinedOutput()
	result := performanceSample{Milliseconds: float64(time.Since(started)) / float64(time.Millisecond)}
	if err != nil {
		result.Error = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	entries, logErr := recentErrors(platform.Env{DataDir: data}, workspace.ID, time.Now())
	if logErr != nil {
		t.Fatal(logErr)
	}
	for _, entry := range entries {
		if !entry.Time.Before(started) {
			result.Diagnostics = append(result.Diagnostics, entry.Error)
		}
	}
	return result
}

func performanceLargeStatic(t *testing.T, unknown bool) performanceSample {
	t.Helper()
	dir, data := performanceWorkspace(t)
	var source strings.Builder
	source.WriteString("package main\n")
	for i := 0; i < 2500; i++ {
		if unknown {
			fmt.Fprintf(&source, "func value%d(n int) int {\n", i)
			for j := 0; j < 18; j++ {
				source.WriteString("n += 1\n")
			}
		} else {
			fmt.Fprintf(&source, "func value%d() int {\nn := 1\n", i)
			for j := 0; j < 17; j++ {
				source.WriteString("n = 1\n")
			}
		}
		source.WriteString("return n }\n")
	}
	file := filepath.Join(dir, "large.go")
	if err := os.WriteFile(file, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ledger.Open(t.Context(), data, ledger.NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterGitWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: workspace}}
	actor := ledger.Actor{Agent: "claude", SessionID: "s", RoundID: "p", CallID: "start"}
	if _, err := service.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, MessageID: "m", Message: "修改第一个函数的一处赋值并交付"}); err != nil {
		t.Fatal(err)
	}
	task, err := service.Start(t.Context(), actor, ledger.StartTaskInput{Title: "50K full static profile"})
	if err != nil {
		t.Fatal(err)
	}
	before, after := "n = 1", "n = 2"
	if unknown {
		before, after = "n += 1", "n += 2"
	}
	if err := os.WriteFile(file, []byte(strings.Replace(source.String(), before, after, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	actor.CallID = "finish"
	if _, err := service.Finish(t.Context(), actor, ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	result := performanceHook(t, dir, data, "Stop", "stop")
	record, err := store.LatestCheck(t.Context(), task.ID, "static", "")
	if err != nil || record == nil {
		result.Error += fmt.Sprintf("; static observation missing: %v", err)
		return result
	}
	result.CheckStatus = record.Status
	result.StaticMillis = float64(record.FinishedAt.Sub(record.StartedAt)) / float64(time.Millisecond)
	result.MissingCount = len(record.Missing)
	result.Missing = record.Missing[:min(8, len(record.Missing))]
	return result
}
