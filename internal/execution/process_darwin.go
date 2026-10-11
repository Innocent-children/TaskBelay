//go:build darwin

package execution

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func configureProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func bindProcess(c *exec.Cmd) (func(), func(), error) {
	return func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }, func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }, nil
}
func processIdentity(pid int) (string, error) {
	c := exec.Command("/bin/ps", "-p", fmt.Sprint(pid), "-o", "lstart=")
	b, e := c.Output()
	if e != nil || strings.TrimSpace(string(b)) == "" {
		return "", fmt.Errorf("process unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}
