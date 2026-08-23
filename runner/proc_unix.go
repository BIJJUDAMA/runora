//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func interruptProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	// Send SIGINT to the process group
	pgid, err := syscall.Getpgid(proc.Pid)
	if err == nil {
		return syscall.Kill(-pgid, syscall.SIGINT)
	}
	return proc.Signal(os.Interrupt)
}

func killProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	// Force kill the process group
	pgid, err := syscall.Getpgid(proc.Pid)
	if err == nil {
		return syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return proc.Kill()
}
