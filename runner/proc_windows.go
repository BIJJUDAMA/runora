//go:build windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func interruptProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	// On Windows, send CTRL_BREAK_EVENT to the process group created with CREATE_NEW_PROCESS_GROUP.
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent := kernel32.NewProc("GenerateConsoleCtrlEvent")
	if procGenerateConsoleCtrlEvent.Find() == nil {
		r1, _, _ := procGenerateConsoleCtrlEvent.Call(syscall.CTRL_BREAK_EVENT, uintptr(proc.Pid))
		if r1 != 0 {
			return nil
		}
	}
	// Fallback to os.Interrupt
	_ = proc.Signal(os.Interrupt)
	return nil
}

func killProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
