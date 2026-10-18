package execution

import (
	"github.com/Innocent-children/taskbelay/internal/domain"
	"path/filepath"
	"strings"
)

// Verification is not permission to perform direct Git mutations.
func CheckCommand(argv []string) error {
	if len(argv) == 0 {
		return domain.Invalid("check.argv", "command required")
	}
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(argv[0]), ".exe"))
	if name != "git" {
		return nil
	}
	i := 1
	for i < len(argv) {
		if argv[i] == "-C" {
			i += 2
			continue
		}
		if argv[i] == "--no-pager" || argv[i] == "--literal-pathspecs" {
			i++
			continue
		}
		if strings.HasPrefix(argv[i], "-") {
			return domain.Fail("COMMAND_NOT_ALLOWED", "check.argv", "Git verification only permits the fixed read-only command surface")
		}
		break
	}
	if i >= len(argv) {
		return domain.Fail("COMMAND_NOT_ALLOWED", "check.argv", "Git subcommand required")
	}
	switch argv[i] {
	case "status", "diff", "show", "log", "rev-parse", "ls-files", "ls-tree", "cat-file", "merge-base":
		return nil
	default:
		return domain.Fail("COMMAND_NOT_ALLOWED", "check.argv", "Git mutations are Host operations with separate authority, not Core verification")
	}
}
