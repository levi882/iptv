package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesParentAndSetsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.txt")
	if err := Write(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "hello" {
		t.Fatalf("read back %q err=%v", raw, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestWriteReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	if err := Write(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "second" {
		t.Fatalf("read back %q", raw)
	}
}

func TestWriteIfChangedSkipsIdenticalContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	written, err := WriteIfChanged(path, []byte("data"), 0o644)
	if err != nil || !written {
		t.Fatalf("first write: written=%v err=%v", written, err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	written, err = WriteIfChanged(path, []byte("data"), 0o644)
	if err != nil || written {
		t.Fatalf("identical write: written=%v err=%v", written, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("identical content still rewrote the file")
	}
	written, err = WriteIfChanged(path, []byte("changed"), 0o644)
	if err != nil || !written {
		t.Fatalf("changed write: written=%v err=%v", written, err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "changed" {
		t.Fatalf("read back %q", raw)
	}
}
