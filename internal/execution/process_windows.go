//go:build windows

package execution

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

func configureProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func bindProcess(c *exec.Cmd) (func(), func(), error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		windows.CloseHandle(job)
		return nil, nil, e
	}
	p, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e != nil {
		windows.CloseHandle(job)
		return nil, nil, e
	}
	e = windows.AssignProcessToJobObject(job, p)
	windows.CloseHandle(p)
	if e != nil {
		windows.CloseHandle(job)
		return nil, nil, e
	}
	return func() { windows.TerminateJobObject(job, 1) }, func() { windows.CloseHandle(job) }, nil
}
func processIdentity(pid int) (string, error) {
	p, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return "", e
	}
	defer windows.CloseHandle(p)
	var creation, exit, kernel, user windows.Filetime
	if e = windows.GetProcessTimes(p, &creation, &exit, &kernel, &user); e != nil {
		return "", e
	}
	if exit.HighDateTime != 0 || exit.LowDateTime != 0 {
		return "", fmt.Errorf("process ended")
	}
	return strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), nil
}
