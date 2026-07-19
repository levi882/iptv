package loglimit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	DefaultSize = "1M"
	MinBytes    = int64(1 << 10)
	MaxBytes    = int64(100 << 20)
)

func ParseSize(value string) (int64, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "B")
	if len(value) < 2 {
		return 0, fmt.Errorf("log size must include K or M")
	}
	unit := value[len(value)-1]
	number := value[:len(value)-1]
	multiplier := int64(0)
	switch unit {
	case 'K':
		multiplier = 1 << 10
	case 'M':
		multiplier = 1 << 20
	default:
		return 0, fmt.Errorf("log size unit must be K or M")
	}
	amount, err := strconv.ParseInt(number, 10, 64)
	if err != nil || amount <= 0 || amount > MaxBytes/multiplier {
		return 0, fmt.Errorf("log size must be between 1K and 100M")
	}
	result := amount * multiplier
	if result < MinBytes || result > MaxBytes {
		return 0, fmt.Errorf("log size must be between 1K and 100M")
	}
	return result, nil
}

// Writer appends normally and compacts only after the configured byte limit
// is exceeded. Compaction streams the newest portion in place instead of
// loading the whole log into memory.
type Writer struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
}

func New(path string, maxBytes int64) (*Writer, error) {
	if path == "" {
		return nil, fmt.Errorf("log path is required")
	}
	if maxBytes < MinBytes || maxBytes > MaxBytes {
		return nil, fmt.Errorf("log size must be between 1K and 100M")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open bounded log: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close bounded log: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("secure bounded log: %w", err)
	}
	w := &Writer{path: path, maxBytes: maxBytes}
	if err := w.trim(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("append bounded log: %w", err)
	}
	written, writeErr := file.Write(p)
	closeErr := file.Close()
	if writeErr != nil {
		return written, writeErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	if err := w.trim(); err != nil {
		return written, err
	}
	return written, nil
}

func (w *Writer) trim() error {
	file, err := os.OpenFile(w.path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open bounded log for compaction: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat bounded log: %w", err)
	}
	if info.Size() <= w.maxBytes {
		return nil
	}
	keepBytes := w.maxBytes * 3 / 4
	start := info.Size() - keepBytes
	reader := bufio.NewReader(io.NewSectionReader(file, start, info.Size()-start))
	discarded, err := reader.ReadBytes('\n')
	start += int64(len(discarded))
	if err == io.EOF {
		return file.Truncate(0)
	}
	if err != nil {
		return fmt.Errorf("align bounded log compaction: %w", err)
	}
	buffer := make([]byte, 64<<10)
	readOffset := start
	writeOffset := int64(0)
	for readOffset < info.Size() {
		want := min(int64(len(buffer)), info.Size()-readOffset)
		n, readErr := file.ReadAt(buffer[:want], readOffset)
		if n > 0 {
			if _, writeErr := file.WriteAt(buffer[:n], writeOffset); writeErr != nil {
				return fmt.Errorf("compact bounded log: %w", writeErr)
			}
			readOffset += int64(n)
			writeOffset += int64(n)
		}
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("read bounded log during compaction: %w", readErr)
		}
		if n == 0 {
			break
		}
	}
	if err := file.Truncate(writeOffset); err != nil {
		return fmt.Errorf("truncate bounded log: %w", err)
	}
	return nil
}
