package platform

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processGroup struct {
	job, process windows.Handle
	suspended    []windows.Handle
}

/**
 * 创建时即加入 Job，避免先启动再分组时子进程逃出回收范围。
 * 只继承三个标准句柄；关闭 Job 会终止仍存活的后代进程。
 */
func startProcess(input ProcessCommand) (_ *processGroup, resultErr error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			windows.CloseHandle(job)
		}
	}()
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	const jobListAttribute = 0x0002000d // PROC_THREAD_ATTRIBUTE_JOB_LIST, Windows 10.
	if err := attributes.Update(jobListAttribute, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return nil, err
	}
	parent := windows.CurrentProcess()
	handles := make([]windows.Handle, 3)
	for i, file := range []uintptr{input.Stdin.Fd(), input.Stdout.Fd(), input.Stderr.Fd()} {
		if err := windows.DuplicateHandle(parent, windows.Handle(file), parent, &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		defer windows.CloseHandle(handles[i])
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	app, err := windows.UTF16PtrFromString(input.Path)
	if err != nil {
		return nil, err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{input.Path}, input.Args...)))
	if err != nil {
		return nil, err
	}
	dir, err := windows.UTF16PtrFromString(input.Dir)
	if err != nil {
		return nil, err
	}
	env := append([]string(nil), input.Env...)
	for _, value := range env {
		if strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("进程环境包含 NUL")
		}
	}
	sort.Slice(env, func(i, j int) bool { return strings.ToUpper(env[i]) < strings.ToUpper(env[j]) })
	block := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	var info windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if input.LowPriority {
		flags |= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	err = windows.CreateProcess(app, line, nil, nil, true, flags, &block[0], dir, &startup.StartupInfo, &info)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(job)
	if err != nil {
		return nil, err
	}
	windows.CloseHandle(info.Thread)
	return &processGroup{job: job, process: info.Process}, nil
}

func (p *processGroup) wait() (int, error) {
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		return -1, err
	}
	var code uint32
	err := windows.GetExitCodeProcess(p.process, &code)
	return int(code), err
}

func (p *processGroup) terminate() error { return windows.TerminateJobObject(p.job, 1) }
func (p *processGroup) close() {
	for _, h := range p.suspended {
		windows.CloseHandle(h)
	}
	windows.CloseHandle(p.process)
	windows.CloseHandle(p.job)
}

func (p *processGroup) suspend(paused bool) error {
	nt := windows.NewLazySystemDLL("ntdll.dll")
	if !paused {
		var failure error
		for _, h := range p.suspended {
			status, _, _ := nt.NewProc("NtResumeProcess").Call(uintptr(h))
			windows.CloseHandle(h)
			if status != 0 {
				failure = fmt.Errorf("恢复副本进程失败：%x", status)
			}
		}
		p.suspended = nil
		return failure
	}
	buffer := make([]uintptr, 16)
	for {
		var returned uint32
		err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer))*uint32(unsafe.Sizeof(buffer[0])), &returned)
		if err == windows.ERROR_MORE_DATA {
			buffer = make([]uintptr, len(buffer)*2)
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	count := *(*uint32)(unsafe.Add(unsafe.Pointer(&buffer[0]), 4))
	base := unsafe.Add(unsafe.Pointer(&buffer[0]), 8)
	for i := uint32(0); i < count; i++ {
		pid := *(*uintptr)(unsafe.Add(base, uintptr(i)*unsafe.Sizeof(uintptr(0))))
		h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
		if err != nil {
			return err
		}
		status, _, _ := nt.NewProc("NtSuspendProcess").Call(uintptr(h))
		if status != 0 {
			windows.CloseHandle(h)
			return fmt.Errorf("暂停副本进程失败：%x", status)
		}
		p.suspended = append(p.suspended, h)
	}
	return nil
}

/**
 * Windows 的 Job 与进程资源范围尚未原生核验，不能用零值代替缺失数据。
 */
func (p *processGroup) resources() *ProcessResources { return nil }
