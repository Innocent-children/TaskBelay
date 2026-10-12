//go:build windows

package execution

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

func startProcess(c *exec.Cmd, record func(ProcessIdentity) error) (stop func(), close func(), err error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, nil, e
	}
	defer func() {
		if err != nil {
			windows.CloseHandle(job)
		}
	}()
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		return nil, nil, e
	}
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	if e = c.Start(); e != nil {
		return nil, nil, e
	}
	p, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e != nil {
		return nil, nil, e
	}
	e = windows.AssignProcessToJobObject(job, p)
	windows.CloseHandle(p)
	if e != nil {
		return nil, nil, e
	}
	identity, e := processIdentity(c.Process.Pid)
	if e != nil {
		return nil, nil, e
	}
	if e = record(ProcessIdentity{PID: c.Process.Pid, Identity: identity}); e != nil {
		return nil, nil, e
	}
	if e = resumeProcess(c.Process.Pid); e != nil {
		return nil, nil, e
	}
	return func() { windows.TerminateJobObject(job, 1) }, func() { windows.CloseHandle(job) }, nil
}

/**
 * Go closes the primary thread handle after CreateProcess. The process is still
 * suspended here, so locate that thread only after its identity and Job are saved.
 */
func resumeProcess(pid int) error {
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for e = windows.Thread32First(snapshot, &entry); e == nil; e = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if e != nil {
			return e
		}
		_, e = windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		return e
	}
	return fmt.Errorf("suspended process thread unavailable: %w", e)
}

func processActive(_ context.Context, process ProcessIdentity) (bool, error) {
	if process.PID <= 0 || process.Identity == "" {
		return false, fmt.Errorf("invalid process identity")
	}
	identity, e := processIdentity(process.PID)
	if errors.Is(e, windows.ERROR_INVALID_PARAMETER) || errors.Is(e, errProcessEnded) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return identity == process.Identity, nil
}

var errProcessEnded = errors.New("process ended")

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
		return "", errProcessEnded
	}
	return strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), nil
}
