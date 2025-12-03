package backup

import (
	"bytes"
	"io"
	"testing"
)

// TestCountingReader verifies that countingReader correctly tracks the number
// of bytes read from the underlying stream. This is important for backup
// metrics when streaming snapshots with unknown Content-Length.
func TestCountingReader(t *testing.T) {
	data := []byte("hello, world")
	reader := &countingReader{
		reader: bytes.NewReader(data),
	}

	buf := make([]byte, 4)
	var total int

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			total += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error from Read: %v", err)
		}
	}

	if total != len(data) {
		t.Fatalf("expected total bytes read %d, got %d", len(data), total)
	}
	if reader.bytesRead != int64(len(data)) {
		t.Fatalf("expected countingReader.bytesRead %d, got %d", len(data), reader.bytesRead)
	}
}
