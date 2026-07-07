package audio

import (
	"testing"
	"time"

	"github.com/gopxl/beep"
)

// fakeSeekable is a minimal beep.StreamSeekCloser for exercising Seek() without
// a real decoded stream or audio device.
type fakeSeekable struct {
	pos    int
	length int
}

func (f *fakeSeekable) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (f *fakeSeekable) Err() error                              { return nil }
func (f *fakeSeekable) Len() int                                { return f.length }
func (f *fakeSeekable) Position() int                           { return f.pos }
func (f *fakeSeekable) Seek(p int) error                        { f.pos = p; return nil }
func (f *fakeSeekable) Close() error                            { return nil }

// TestBufferedStreamPlayer_SeekDoesNotDeadlock guards the RWMutex discipline:
// Seek() holds the write lock, so it must NOT call GetDuration() (which takes
// the read lock) — that self-deadlocks and, in the TUI, freezes the whole app
// on the next render while audio keeps playing. Timeout-guarded so a deadlock
// fails fast instead of hanging the suite.
func TestBufferedStreamPlayer_SeekDoesNotDeadlock(t *testing.T) {
	p := NewBufferedStreamPlayer()
	// Simulate a loaded ~10s stream so Seek reaches the duration check.
	p.streamer = &fakeSeekable{length: 441000}
	p.format = beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}

	done := make(chan error, 1)
	go func() { done <- p.Seek(5 * time.Second) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Seek returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Seek deadlocked (no return within 2s) — RWMutex self-deadlock")
	}
}
