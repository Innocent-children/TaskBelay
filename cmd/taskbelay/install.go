package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// runInstall 处理 taskbelay install <agent> 和 taskbelay uninstall <agent>。
func runInstall(env platform.Env, args []string, stdout io.Writer, install bool) error {
	if len(args) != 1 {
		return errUsage
	}
	a, ok := agent.Lookup(args[0])
	if !ok {
		return fmt.Errorf("没有 %s 这个 Agent，可以是 %s", args[0], strings.Join(agent.Names(), "、"))
	}

	var msg string
	var err error
	if install {
		if err := gitstate.CheckVersion(); err != nil {
			return err
		}
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return exeErr
		}
		msg, err = a.Install(env, exe)
	} else {
		msg, err = a.Uninstall(env)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, msg)
	return nil
}
