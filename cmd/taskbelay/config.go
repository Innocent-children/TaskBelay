package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

// runConfig 处理 taskbelay config get <设置项> 和 taskbelay config set <设置项> <值>。
func runConfig(dataDir string, args []string, stdout io.Writer) error {
	isGet := len(args) == 2 && args[0] == "get"
	isSet := len(args) == 3 && args[0] == "set"
	if !isGet && !isSet {
		return errUsage
	}

	l, err := ledger.Open(filepath.Join(dataDir, "taskbelay.db"))
	if err != nil {
		return err
	}
	defer l.Close()

	ctx := context.Background()
	if isSet {
		return l.SetSetting(ctx, args[1], args[2])
	}
	value, err := l.Setting(ctx, args[1])
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, value)
	return nil
}
