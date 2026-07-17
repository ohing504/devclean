package ui_test

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/ui"
)

// Stop must join the animation goroutine before returning, so no late frame
// reprints over the cleared line and leaves a stale spinner artifact.
func TestSpinner_StopJoinsGoroutine(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	var mu sync.Mutex
	var buf bytes.Buffer
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		b := make([]byte, 256)
		for {
			n, readErr := r.Read(b)
			if n > 0 {
				mu.Lock()
				buf.Write(b[:n])
				mu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()

	s := ui.NewSpinner("scanning")
	time.Sleep(200 * time.Millisecond) // let it tick a few frames
	s.Stop()

	// Stop joined the goroutine, but the pipe→buf copy is async; let it drain
	// before sampling the baseline.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	afterStop := buf.Len()
	mu.Unlock()

	// Well past two tick intervals (80ms) — a leaked goroutine would write here.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	final := buf.Len()
	mu.Unlock()

	if final != afterStop {
		t.Errorf("spinner wrote %d bytes after Stop; goroutine not joined", final-afterStop)
	}

	os.Stdout = orig
	w.Close()
	<-drained
	r.Close()

	// The last write should be the clear sequence, not a frame.
	if !bytes.HasSuffix(buf.Bytes(), []byte("\r\033[K")) {
		t.Errorf("output should end with the clear sequence, got %q", tail(buf.Bytes()))
	}
}

// Stop is idempotent — a second call must not panic on the closed channel.
func TestSpinner_StopIdempotent(t *testing.T) {
	orig := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = orig; w.Close() }()

	s := ui.NewSpinner("x")
	s.Stop()
	s.Stop()
}

func tail(b []byte) []byte {
	const n = 8
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}
