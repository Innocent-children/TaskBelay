// Command commands classifies recorded argv without running the recorded commands.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

type input struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"`
	Args      []string `json:"argv"`
	Directory string   `json:"directory"`
}

type result struct {
	ID              string   `json:"id"`
	Verification    *bool    `json:"verification"`
	LiteralCommands []string `json:"literal_commands_sha256"`
	Unknown         []string `json:"unknown"`
}

func classify(item input) result {
	out := result{ID: item.ID, LiteralCommands: []string{}, Unknown: []string{}}
	if len(item.Args) == 0 || item.Role != "native" && item.Role != "shell_invocation" {
		out.Unknown = append(out.Unknown, "missing argv or unsupported trace role")
		return out
	}
	args := item.Args
	if item.Role == "native" && len(args) > 2 && shell.Base(args[0]) == "node" && shell.Base(args[1]) == "npx" {
		// The forwarding trace records Node executing npx's script. Classify
		// its CLI arguments without changing the recorded process identity.
		args = args[1:]
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	parsed := shell.Parse(strings.Join(quoted, " "), item.Directory, event.POSIX,
		map[event.Shell]shell.Parser{event.POSIX: posix.Parser{}, event.PowerShell: pwsh.Parser{}})
	if item.Role == "shell_invocation" {
		// Claude's prefix supplies an assembled shell script. Retain only
		// literal eval inputs for comparison with native Bash tool requests.
		for _, segment := range parsed.Segments {
			if segment.Known && len(segment.Args) == 2 && segment.Args[0] == "eval" {
				digest := sha256.Sum256([]byte(segment.Args[1]))
				out.LiteralCommands = append(out.LiteralCommands, hex.EncodeToString(digest[:]))
			}
		}
		return out
	}
	if len(parsed.Segments) != 1 || !parsed.Segments[0].Known || len(parsed.Missing) != 0 {
		out.Unknown = append(out.Unknown, "recorded argv could not be classified as one known command")
		return out
	}
	verification := parsed.Segments[0].Kind == shell.Verification
	out.Verification = &verification
	return out
}

func main() {
	var inputs []input
	if err := json.NewDecoder(os.Stdin).Decode(&inputs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	outputs := make([]result, 0, len(inputs))
	for _, item := range inputs {
		outputs = append(outputs, classify(item))
	}
	if err := json.NewEncoder(os.Stdout).Encode(outputs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
