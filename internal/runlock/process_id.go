//go:build !linux

package runlock

import "os"

func processID() int { return os.Getpid() }
