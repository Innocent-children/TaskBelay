package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func runHits(ctx context.Context, args []string, env platform.Env, dir string, output io.Writer) error {
	days := 14
	if len(args) > 0 {
		if len(args) != 2 || args[0] != "--since" || !strings.HasSuffix(args[1], "d") {
			return fmt.Errorf("用法: taskbelay hits --since 14d")
		}
		var err error
		days, err = strconv.Atoi(strings.TrimSuffix(args[1], "d"))
		if err != nil || days <= 0 || days > 36500 {
			return fmt.Errorf("--since 需要正整数天数，例如 14d")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, event.MCPBudget)
	defer cancel()
	workspace, err := gitstate.Discover(ctx, dir)
	if err != nil {
		return err
	}
	store, err := ledger.Open(ctx, env.DataDir, ledger.NormalWait)
	if err != nil {
		return err
	}
	defer store.Close()
	hits, err := store.Hits(ctx, workspace.ID, time.Now().Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	for _, hit := range hits {
		if err := encoder.Encode(hit); err != nil {
			return err
		}
	}
	return nil
}
