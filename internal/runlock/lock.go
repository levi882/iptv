package runlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrAlreadyRunning = errors.New("another refresh is already running")

type Lock struct {
	directory string
}

func Acquire(directory string) (*Lock, error) {
	if err := os.Mkdir(directory, 0o700); err == nil {
		return initialize(directory)
	} else if !os.IsExist(err) {
		return nil, err
	}
	pidPath := filepath.Join(directory, "pid")
	raw, _ := os.ReadFile(pidPath)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	if pid > 0 && processAlive(pid) {
		return nil, fmt.Errorf("%w (pid=%d)", ErrAlreadyRunning, pid)
	}
	_ = os.Remove(pidPath)
	if err := os.Remove(directory); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale refresh lock: %w", err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, err
	}
	return initialize(directory)
}

func initialize(directory string) (*Lock, error) {
	if err := os.WriteFile(filepath.Join(directory, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		_ = os.Remove(directory)
		return nil, err
	}
	return &Lock{directory: directory}, nil
}

func (l *Lock) Release() {
	if l == nil || l.directory == "" {
		return
	}
	_ = os.Remove(filepath.Join(l.directory, "pid"))
	_ = os.Remove(l.directory)
	l.directory = ""
}
