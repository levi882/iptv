//go:build !linux

package netbind

import (
	"fmt"
	"net"
	"time"
)

func Dialer(interfaceName, sourceIP string, timeout time.Duration) (*net.Dialer, error) {
	if interfaceName != "" {
		return nil, fmt.Errorf("binding to interface %q is only supported on Linux", interfaceName)
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	if sourceIP != "" {
		ip := net.ParseIP(sourceIP)
		if ip == nil {
			return nil, fmt.Errorf("invalid bind source IP %q", sourceIP)
		}
		dialer.LocalAddr = &net.TCPAddr{IP: ip}
	}
	return dialer, nil
}
