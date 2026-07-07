package audio

import "testing"

func TestStreamBufferCompletedShortDownloadIsPreloaded(t *testing.T) {
	buffer := &StreamBuffer{
		data:      make([]byte, 128),
		size:      128,
		minBuffer: 1024,
	}

	buffer.write([]byte("short complete track"))
	buffer.mu.Lock()
	buffer.completed = true
	buffer.mu.Unlock()

	if !buffer.isPreloaded() {
		t.Fatal("completed short download should be considered preloaded")
	}
}
