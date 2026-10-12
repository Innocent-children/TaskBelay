//go:build darwin

package execution

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

/**
 * The launch pipe closes on Core death. The shell cannot exec the approved argv
 * until its process-group identity is durable, so recovery never misses a running command.
 */
func startProcess(c *exec.Cmd, record func(ProcessIdentity) error) (func(), func(), error) {
	read, write, e := os.Pipe()
	if e != nil {
		return nil, nil, e
	}
	defer read.Close()
	defer write.Close()
	argv := append([]string{c.Path}, c.Args[1:]...)
	c.Path = "/bin/sh"
	c.Args = append([]string{"/bin/sh", "-c", `IFS= read -r ready <&3 || exit 125; exec 3<&-; [ "$ready" = start ] || exit 125; exec "$@"`, "taskbelay-launch"}, argv...)
	c.ExtraFiles = []*os.File{read}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = c.Start(); e != nil {
		return nil, nil, e
	}
	identity, e := processIdentity(c.Process.Pid)
	if e != nil {
		return nil, nil, e
	}
	if e = record(ProcessIdentity{PID: c.Process.Pid, Identity: identity}); e != nil {
		return nil, nil, e
	}
	if _, e = write.WriteString("start\n"); e != nil {
		return nil, nil, e
	}
	stop := func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	return stop, stop, nil
}

/**
 * The leader may have exited while descendants still run. A reused group ID is
 * conservatively treated as active; recovery never signals a potentially unrelated group.
 */
func processActive(ctx context.Context, process ProcessIdentity) (bool, error) {
	if process.PID <= 0 || process.Identity == "" {
		return false, fmt.Errorf("invalid process identity")
	}
	b, e := exec.CommandContext(ctx, "/bin/ps", "-axo", "pgid=,stat=").Output()
	if e != nil {
		return false, e
	}
	group := strconv.Itoa(process.PID)
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == group && !strings.HasPrefix(fields[1], "Z") {
			return true, nil
		}
	}
	return false, nil
}
func processIdentity(pid int) (string, error) {
	c := exec.Command("/bin/ps", "-p", fmt.Sprint(pid), "-o", "lstart=")
	b, e := c.Output()
	if e != nil || strings.TrimSpace(string(b)) == "" {
		return "", fmt.Errorf("process unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}
