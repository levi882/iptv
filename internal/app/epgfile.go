package app

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

const maxExpandedEPGBytes = 128 << 20

func epgBytesForPath(raw []byte, path string) ([]byte, error) {
	wantsGzip := strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".gz")
	hasGzip := len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b
	if wantsGzip == hasGzip {
		return raw, nil
	}

	if wantsGzip {
		var output bytes.Buffer
		writer, err := gzip.NewWriterLevel(&output, gzip.BestSpeed)
		if err != nil {
			return nil, fmt.Errorf("create gzip writer: %w", err)
		}
		if _, err := writer.Write(raw); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("compress EPG: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("finish EPG compression: %w", err)
		}
		return output.Bytes(), nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open compressed EPG: %w", err)
	}
	decompressed, readErr := io.ReadAll(io.LimitReader(reader, maxExpandedEPGBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("decompress EPG: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("finish EPG decompression: %w", closeErr)
	}
	if len(decompressed) > maxExpandedEPGBytes {
		return nil, fmt.Errorf("expanded EPG exceeds %d MiB", maxExpandedEPGBytes>>20)
	}
	return decompressed, nil
}
