package runlock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireExcludesSecondOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refresh.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if _, err := Acquire(path); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error=%v", err)
	}
	first.Release()
	third, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	third.Release()
}
