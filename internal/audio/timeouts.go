package audio

import "time"

const (
	// DefaultHTTPTimeout is the shared network budget for SoundCloud media
	// requests and metadata calls made during playback startup.
	DefaultHTTPTimeout = 30 * time.Second

	// StreamExtractionTimeout bounds SoundCloud stream metadata/transcoding
	// resolution before playback startup moves on to decoding.
	StreamExtractionTimeout = DefaultHTTPTimeout

	// PlaybackStartTimeout bounds local stream startup, including ffmpeg HLS
	// decode into the in-memory seekable stream.
	PlaybackStartTimeout = DefaultHTTPTimeout

	// LoadingTimeout covers the sequential TUI startup phases plus a small UI
	// cushion so the loading view does not error before legal work can finish.
	LoadingTimeout = StreamExtractionTimeout + PlaybackStartTimeout + 5*time.Second
)
