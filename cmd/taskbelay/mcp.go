package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/mcpserver"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type outputCloser struct{ io.Writer }

func (outputCloser) Close() error { return nil }

func runMCP(ctx context.Context, env platform.Env, dir string, input io.ReadCloser, output io.Writer) error {
	path, executable := mcpExecutable()
	setup, cancel := context.WithTimeout(ctx, event.MCPBudget)
	workspace, err := gitstate.Discover(setup, dir)
	var service hook.Tasks
	if err == nil {
		var store *ledger.Store
		store, err = ledger.Open(setup, env.DataDir, ledger.NormalWait)
		if err == nil {
			defer store.Close()
			err = store.RegisterGitWorkspace(setup, workspace)
			service = hook.Tasks{Observer: hook.Observer{Store: store, Workspace: workspace}}
		}
	}
	cancel()
	if !errors.Is(err, gitstate.ErrNotWorkspace) {
		service.ReportError = func(cause error) { _ = logWorkspaceError(env, workspace.ID, cause) }
	}
	if err == nil {
		service.Background = check.NewBackground(ctx, service.Store, workspace, service.ReportError)
		defer service.Background.Close()
	}
	// 初始化错误作为可读的工具结果返回；stdio 只写 MCP 协议内容。
	server := mcpserver.New(service, err, agentContext(service, os.Getenv("CLAUDE_CODE_SESSION_ID")))
	transport := &mcp.IOTransport{Reader: input, Writer: outputCloser{output}}
	return serveMCP(ctx, server, transport, path, executable)
}

func serveMCP(ctx context.Context, server *mcp.Server, transport mcp.Transport, path string, executable os.FileInfo) error {
	if executable == nil {
		return server.Run(ctx, transport)
	}
	conn := &retiringMCP{transport: transport}
	session, err := server.Connect(ctx, conn, nil)
	if err != nil {
		return err
	}
	ended := make(chan error, 1)
	go func() { ended <- session.Wait() }()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var drained <-chan struct{}
	for {
		select {
		case err := <-ended:
			return err
		case <-ctx.Done():
			session.Close()
			return ctx.Err()
		case <-drained:
			return session.Close()
		case <-ticker.C:
			current, err := platform.ExecutableFileInfo(path)
			if err == nil && current.Mode().IsRegular() && !os.SameFile(executable, current) {
				ticker.Stop()
				drained = conn.retire()
			}
		}
	}
}

/**
 * 优先跟踪 Agent 启动时使用的入口，安装器可能替换符号链接本身。
 * Linux 的 /proc/self/exe 始终指向本进程加载的文件，即使安装位置已经被替换。
 */
func mcpExecutable() (string, os.FileInfo) {
	path, err := os.Executable()
	if err != nil {
		return "", nil
	}
	image := path
	if runtime.GOOS == "linux" {
		image = "/proc/self/exe"
	}
	running, err := platform.ExecutableFileInfo(image)
	if err != nil {
		return "", nil
	}
	if launch, err := exec.LookPath(os.Args[0]); err == nil {
		if absolute, err := filepath.Abs(launch); err == nil {
			if entry, err := platform.ExecutableFileInfo(absolute); err == nil && os.SameFile(running, entry) {
				return absolute, running
			}
		}
	}
	return path, running
}

func agentContext(service hook.Tasks, claudeSession string) mcpserver.ContextResolver {
	return func(ctx context.Context, request *mcp.CallToolRequest) (ledger.Actor, error) {
		identity, found, err := codex.MCPIdentity(request.Params.Meta)
		if found || err != nil {
			return ledger.Actor{Agent: identity.Agent, SessionID: identity.SessionID, RoundID: identity.RoundID, CallID: identity.ToolCallID}, err
		}
		identity, found, err = claude.MCPIdentity(request.Params.Meta, claudeSession)
		if err != nil {
			return ledger.Actor{}, err
		}
		if found {
			return service.Store.MCPContext(ctx, service.Workspace.ID, ledger.Actor{Agent: identity.Agent, SessionID: identity.SessionID, CallID: identity.ToolCallID}, request.Params.Name)
		}
		return mcpserver.MetadataContext(ctx, request)
	}
}
