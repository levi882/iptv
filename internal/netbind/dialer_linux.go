//go:build linux

package netbind

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

const soBindToDevice = 25

func Dialer(interfaceName, sourceIP string, timeout time.Duration) (*net.Dialer, error) {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	if sourceIP != "" {
		ip := net.ParseIP(sourceIP)
		if ip == nil {
			return nil, fmt.Errorf("invalid bind source IP %q", sourceIP)
		}
		dialer.LocalAddr = &net.TCPAddr{IP: ip}
	}
	if interfaceName != "" {
		dialer.Control = func(_, _ string, conn syscall.RawConn) error {
			var controlErr error
			if err := conn.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, soBindToDevice, interfaceName)
			}); err != nil {
				return err
			}
			return controlErr
		}
	}
	return dialer, nil
}
