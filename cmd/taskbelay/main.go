package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/mcp"
	"github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	"github.com/Innocent-children/taskbelay/internal/version"
	"github.com/Innocent-children/taskbelay/internal/webui"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const helpText = `TaskBelay observes authorized changes and actual verification evidence locally.
Usage:
  taskbelay help|version
  taskbelay mcp --stdio
  taskbelay check run                  Native execution; JSON request on stdin
  taskbelay call <tool_name>             JSON request on stdin
  taskbelay host-check pre-file-write|workspace-available
  taskbelay config validate
  taskbelay webui start|open|status|stop [--no-open] [--json|--plain]
Set TASKBELAY_DATA_DIR to a local directory. Commands do not modify Git.
`

type mcpServeFunc func(context.Context, *application.Service, string, string, userconfig.Preferences, bool) error

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, serveStandardIO)) }
func service(ctx context.Context, getenv func(string) string) (*application.Service, func(), error) {
	dir := getenv("TASKBELAY_DATA_DIR")
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, nil, fmt.Errorf("TASKBELAY_DATA_DIR must be an absolute local directory")
	}
	db, e := store.Open(ctx, filepath.Join(dir, "taskbelay.db"))
	if e != nil {
		return nil, nil, e
	}
	s, e := application.NewService(db, repository.NewGitObserver())
	if e != nil {
		db.Close()
		return nil, nil, e
	}
	return s, func() { db.Close() }, nil
}
func run(args []string, in io.Reader, out, errout io.Writer, getenv func(string) string, serve mcpServeFunc) int {
	if name := auxiliaryHelpName(args); name != "" {
		contract, err := protocol.Describe(name)
		if err != nil {
			json.NewEncoder(out).Encode(protocol.ErrorEnvelope(name, err))
			return 1
		}
		if json.NewEncoder(out).Encode(contract) != nil {
			return 1
		}
		return 0
	}
	if len(args) >= 3 && (args[0] == "call" || args[0] == "check" && args[1] == "run" || args[0] == "host-check") && args[2] == "--help" && (len(args) == 3 || len(args) == 4 && args[3] == "--json") {
		name := args[1]
		if args[0] == "check" {
			name = "check.run"
		}
		if args[0] == "host-check" {
			name = "host-check." + name
		}
		contract, err := protocol.Describe(name)
		if err != nil {
			json.NewEncoder(out).Encode(protocol.ErrorEnvelope(name, err))
			return 1
		}
		if json.NewEncoder(out).Encode(contract) != nil {
			return 1
		}
		return 0
	}
	if len(args) == 0 || len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		io.WriteString(out, helpText)
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		v, e := version.Current()
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		fmt.Fprintln(out, "taskbelay "+v)
		return 0
	}
	if len(args) == 2 && args[0] == "config" && args[1] == "validate" {
		return runConfigValidation(in, out)
	}
	if len(args) == 3 && args[0] == "config" && args[1] == "validate" && args[2] == "--help" {
		io.WriteString(out, configValidationHelp)
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), webUISignals()...)
	defer cancel()
	if len(args) >= 2 && args[0] == "webui" {
		dir := getenv("TASKBELAY_DATA_DIR")
		if dir == "" {
			fmt.Fprintln(errout, "TASKBELAY_DATA_DIR is required")
			return 1
		}
		v, _ := version.Current()
		var result webui.RuntimeState
		var e error
		switch args[1] {
		case "serve":
			e = webui.Serve(ctx, dir, v)
		case "start":
			noOpen := false
			for _, x := range args[2:] {
				noOpen = noOpen || x == "--no-open"
			}
			result, e = webui.Start(ctx, dir, v, noOpen)
		case "status":
			result, e = webui.Status(ctx, dir, v)
		case "open":
			result, e = webui.Status(ctx, dir, v)
			if e == nil {
				e = webui.OpenBrowser(result.URL)
			}
		case "stop":
			result, e = webui.Stop(ctx, dir, v)
		default:
			fmt.Fprintln(errout, "invalid WebUI operation")
			return 2
		}
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		if args[1] != "serve" {
			json.NewEncoder(out).Encode(result)
		}
		return 0
	}
	summaryText := len(args) == 4 && args[0] == "mcp" && args[1] == "--stdio" && args[2] == "--result-text" && args[3] == "summary"
	valid := summaryText || len(args) == 2 && (args[0] == "call" || args[0] == "check" && args[1] == "run" || args[0] == "host-check" && (args[1] == "pre-file-write" || args[1] == "workspace-available") || args[0] == "mcp" && args[1] == "--stdio")
	if !valid {
		fmt.Fprintln(errout, "invalid arguments; use taskbelay help")
		return 2
	}
	s, close, e := service(ctx, getenv)
	if e != nil {
		if args[0] != "mcp" {
			name := args[1]
			if args[0] == "check" {
				name = "check.run"
			}
			if args[0] == "host-check" {
				name = "host-check." + name
			}
			result := protocol.ErrorEnvelope(name, e)
			result.Outcome = "not_applied"
			if args[0] == "check" {
				result.Execution = "not_started"
			}
			json.NewEncoder(out).Encode(result)
			return 1
		}
		fmt.Fprintln(errout, e)
		return 1
	}
	defer close()
	v, e := version.Current()
	if e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	if args[0] == "mcp" {
		preferences, e := userconfig.Load(userHomeDirectory(getenv))
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		if serve == nil {
			serve = serveStandardIO
		}
		if e = serve(ctx, s, v, getenv("TASKBELAY_CODEX_MCP_INSTRUCTIONS"), preferences, summaryText); e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		return 0
	}
	raw, e := io.ReadAll(io.LimitReader(in, 1024*1024+1))
	if e != nil {
		fmt.Fprintln(errout, "request input unavailable")
		return 1
	}

	handler := protocol.NewHandler(s, v, userconfig.Preferences{})
	var result protocol.Envelope
	if args[0] == "host-check" {
		result = handler.RunPreflight(ctx, "host-check."+args[1], raw)
	} else if args[0] == "check" {
		result = handler.RunCheck(ctx, raw)
	} else {
		result = handler.Dispatch(ctx, args[1], raw)
	}
	if e = json.NewEncoder(out).Encode(result); e != nil {
		return 1
	}
	if !result.OK {
		return 1
	}
	return 0
}
func serveStandardIO(ctx context.Context, s *application.Service, v, instructions string, p userconfig.Preferences, summaryText bool) error {
	handler := protocol.NewHandler(s, v, p)
	return mcp.NewServer(handler, v, instructions, summaryText).Run(ctx, &sdk.StdioTransport{})
}

/**
 * Help is resolved before data-directory and service initialization. An optional
 * --json selects the same machine-readable contract as the plain help spelling.
 */
func auxiliaryHelpName(args []string) string {
	if len(args) == 3 && args[0] == "config" && args[1] == "validate" && args[2] == "--help" {
		return ""
	}
	if len(args) > 0 && args[len(args)-1] == "--json" {
		args = args[:len(args)-1]
	}
	if len(args) < 2 || args[len(args)-1] != "--help" {
		return ""
	}
	if len(args) == 2 {
		switch args[0] {
		case "version", "help", "mcp":
			return "core." + args[0]
		}
	}
	if len(args) == 3 {
		if args[0] == "config" && args[1] == "validate" {
			return "core.config.validate"
		}
		if args[0] == "webui" {
			return "core.webui." + args[1]
		}
	}
	return ""
}
