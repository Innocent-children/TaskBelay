package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
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
	// 初始化错误作为可读的工具结果返回；stdio 只写 MCP 协议内容。
	return mcpserver.New(service, err, agentContext(service, os.Getenv("CLAUDE_CODE_SESSION_ID"))).Run(ctx, &mcp.IOTransport{Reader: input, Writer: outputCloser{output}})
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
