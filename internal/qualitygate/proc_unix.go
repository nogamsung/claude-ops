//go:build unix

package qualitygate

import "syscall"

// procAttrNewSession launches the command in its own process group so
// signalProcessGroup can reach descendants (e.g. `go test` forks a child test
// binary which would otherwise survive SIGTERM to the parent).
func procAttrNewSession() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// signalProcessGroup sends sig to the process group whose leader is pid.
// Unix PGID is -pgid when passed to Kill.
func signalProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
