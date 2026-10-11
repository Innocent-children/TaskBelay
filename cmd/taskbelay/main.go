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
  taskbelay call <tool_name>             JSON request on stdin
  taskbelay host-check pre-file-write|workspace-available
  taskbelay config validate
  taskbelay webui start|open|status|stop [--no-open] [--json|--plain]
Set TASKBELAY_DATA_DIR to a local directory. Commands do not modify Git.
`

type mcpServeFunc func(context.Context, *application.Service, string, string, userconfig.Preferences) error

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
	valid := len(args) == 2 && (args[0] == "call" || args[0] == "host-check" && (args[1] == "pre-file-write" || args[1] == "workspace-available") || args[0] == "mcp" && args[1] == "--stdio")
	if !valid {
		fmt.Fprintln(errout, "invalid arguments; use taskbelay help")
		return 2
	}
	s, close, e := service(ctx, getenv)
	if e != nil {
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
		if e = serve(ctx, s, v, getenv("TASKBELAY_CODEX_MCP_INSTRUCTIONS"), preferences); e != nil {
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
	if args[0] == "host-check" {
		complete, e := s.MeasureControl(ctx)
		if e != nil {
			fmt.Fprintln(errout, "control cost reservation failed; check was not performed")
			return 1
		}
		defer complete()
		var q struct {
			Host           string `json:"host"`
			RepositoryPath string `json:"repository_path"`
			Repositories   []struct {
				Key  string `json:"key"`
				Path string `json:"repository_path"`
			} `json:"repositories"`
			Paths    []string `json:"paths"`
			Tool     string   `json:"tool_name"`
			Intent   string   `json:"intent_digest"`
			Complete bool     `json:"path_parse_complete"`
		}
		if e = mcp.Decode(raw, &q); e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		if args[1] == "pre-file-write" {
			result, e := s.PreWrite(ctx, q.RepositoryPath, q.Paths, q.Complete)
			if e != nil {
				fmt.Fprintln(errout, e)
				return 1
			}
			json.NewEncoder(out).Encode(result)
			return 0
		}
		roots := []string{q.RepositoryPath}
		if len(q.Repositories) > 0 {
			roots = nil
			for _, r := range q.Repositories {
				roots = append(roots, r.Path)
			}
		}
		for _, root := range roots {
			canonical, _, e := s.Observer.IdentifyWorkspace(ctx, root)
			if e != nil {
				fmt.Fprintln(errout, e)
				return 1
			}
			id, e := s.Store.Claim(ctx, canonical)
			if e != nil {
				fmt.Fprintln(errout, e)
				return 1
			}
			if id != "" {
				json.NewEncoder(out).Encode(map[string]any{"available": false, "task_id": id})
				return 0
			}
		}
		json.NewEncoder(out).Encode(map[string]any{"available": true, "repository_path": q.RepositoryPath})
		return 0
	}
	server, e := mcp.NewServer(s, v, nil)
	if e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	result := server.Dispatch(ctx, args[1], raw)
	if e = json.NewEncoder(out).Encode(result); e != nil {
		return 1
	}
	if !result.OK {
		return 1
	}
	return 0
}
func serveStandardIO(ctx context.Context, s *application.Service, v, instructions string, p userconfig.Preferences) error {
	server, e := mcp.NewServer(s, v, &mcp.ServerOptions{Instructions: instructions, HostPreferences: p})
	if e != nil {
		return e
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
