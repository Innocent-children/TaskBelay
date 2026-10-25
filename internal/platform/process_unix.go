//go:build unix

package platform

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
)

type processGroup struct{ command *exec.Cmd }

func startProcess(input ProcessCommand) (*processGroup, error) {
	command := exec.Command(input.Path, input.Args...)
	command.Dir, command.Env = input.Dir, input.Env
	command.Stdin, command.Stdout, command.Stderr = input.Stdin, input.Stdout, input.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &processGroup{command: command}, nil
}

func (p *processGroup) wait() (int, error) {
	err := p.command.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if p.command.ProcessState == nil {
		return -1, err
	}
	return p.command.ProcessState.ExitCode(), err
}

func (p *processGroup) terminate() error {
	err := syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (p *processGroup) close() {}

func (p *processGroup) resources() *ProcessResources {
	state := p.command.ProcessState
	if state == nil {
		return nil
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return nil
	}
	unit := "KiB"
	if runtime.GOOS == "darwin" {
		unit = "bytes"
	} else if runtime.GOOS != "linux" {
		return nil
	}
	return &ProcessResources{UserCPU: state.UserTime(), SystemCPU: state.SystemTime(), MaxRSS: usage.Maxrss, MaxRSSUnit: unit}
}
