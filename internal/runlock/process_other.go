//go:build !linux

package runlock

func processAlive(pid int) bool {
	return pid > 0 && pid == processID()
}
