package ui_test

import (
	"context"
	"time"

	"github.com/stretchr/testify/assert"

	"soundcloud-tui/internal/audio"
	"soundcloud-tui/internal/soundcloud"
)

// MockAudioPlayer implements audio.Player for testing
type MockAudioPlayer struct {
	state           audio.PlayerState
	volume          float64
	position        time.Duration
	duration        time.Duration
	playCalls       int
	playStreamCalls int
	stopCalls       int
	lastStreamURL   string
	lastStreamInfo  *audio.StreamInfo
	OnStop          func()
}

func (m *MockAudioPlayer) Play(ctx context.Context, streamURL string) error {
	m.playCalls++
	m.lastStreamURL = streamURL
	m.state = audio.StatePlaying
	return nil
}

func (m *MockAudioPlayer) PlayStream(ctx context.Context, streamInfo *audio.StreamInfo) error {
	m.playStreamCalls++
	if streamInfo != nil {
		info := *streamInfo
		m.lastStreamInfo = &info
		m.lastStreamURL = streamInfo.URL
	}
	m.state = audio.StatePlaying
	return nil
}

func (m *MockAudioPlayer) Pause() error {
	m.state = audio.StatePaused
	return nil
}

func (m *MockAudioPlayer) Resume() error {
	if m.state == audio.StatePaused {
		m.state = audio.StatePlaying
	}
	return nil
}

func (m *MockAudioPlayer) Stop() error {
	m.stopCalls++
	if m.OnStop != nil {
		m.OnStop()
	}
	m.state = audio.StateStopped
	m.position = 0
	return nil
}

func (m *MockAudioPlayer) GetState() audio.PlayerState {
	return m.state
}

func (m *MockAudioPlayer) SetVolume(volume float64) error {
	if volume < 0 || volume > 1 {
		return assert.AnError
	}
	m.volume = volume
	return nil
}

func (m *MockAudioPlayer) GetVolume() float64 {
	return m.volume
}

func (m *MockAudioPlayer) Seek(position time.Duration) error {
	if position < 0 || position > m.duration {
		return assert.AnError
	}
	m.position = position
	return nil
}

func (m *MockAudioPlayer) GetPosition() time.Duration {
	return m.position
}

func (m *MockAudioPlayer) GetDuration() time.Duration {
	return m.duration
}

func (m *MockAudioPlayer) Close() error {
	m.state = audio.StateStopped
	return nil
}

// MockStreamExtractor implements audio.StreamExtractor for testing
type MockStreamExtractor struct {
	ExtractFunc      func(ctx context.Context, trackID int64) (*audio.StreamInfo, error)
	ExtractTrackFunc func(ctx context.Context, req audio.TrackStreamRequest) (*audio.StreamInfo, error)
	TrackRequests    []audio.TrackStreamRequest
}

func (m *MockStreamExtractor) ExtractStreamURL(ctx context.Context, trackID int64) (*audio.StreamInfo, error) {
	if m.ExtractFunc != nil {
		return m.ExtractFunc(ctx, trackID)
	}
	return &audio.StreamInfo{
		URL:      "https://example.com/stream.mp3",
		Format:   "mp3",
		Quality:  "sq",
		Duration: 240000,
	}, nil
}

func (m *MockStreamExtractor) ExtractTrackStreamURL(ctx context.Context, req audio.TrackStreamRequest) (*audio.StreamInfo, error) {
	m.TrackRequests = append(m.TrackRequests, req)
	if m.ExtractTrackFunc != nil {
		return m.ExtractTrackFunc(ctx, req)
	}
	return m.ExtractStreamURL(ctx, req.TrackID)
}

func (m *MockStreamExtractor) GetAvailableQualities(ctx context.Context, trackID int64) ([]string, error) {
	return []string{"sq", "hq"}, nil
}

func (m *MockStreamExtractor) ValidateStreamURL(ctx context.Context, streamURL string) (bool, error) {
	return true, nil
}

// MockSoundCloudClient implements soundcloud.ClientInterface for testing
type MockSoundCloudClient struct {
	SearchFunc      func(query string) ([]soundcloud.Track, error)
	LibraryFunc     func() ([]soundcloud.Playlist, error)
	PlaylistFunc    func(playlistID int64) ([]soundcloud.Track, error)
	FavoritesFunc   func() ([]soundcloud.Track, error)
	Authenticated   bool
	AuthSourceValue string
}

func (m *MockSoundCloudClient) Search(query string) ([]soundcloud.Track, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(query)
	}
	return []soundcloud.Track{}, nil
}

func (m *MockSoundCloudClient) GetTrackInfo(url string) (*soundcloud.Track, error) {
	return &soundcloud.Track{
		ID:    123,
		Title: "Test Track",
		User:  soundcloud.User{Username: "Test Artist"},
	}, nil
}

func (m *MockSoundCloudClient) GetDownloadURL(trackURL string, format string) (string, error) {
	return "https://example.com/download.mp3", nil
}

func (m *MockSoundCloudClient) IsAuthenticated() bool {
	return m.Authenticated
}

func (m *MockSoundCloudClient) AuthSource() string {
	return m.AuthSourceValue
}

func (m *MockSoundCloudClient) Library() ([]soundcloud.Playlist, error) {
	if m.LibraryFunc != nil {
		return m.LibraryFunc()
	}
	return []soundcloud.Playlist{}, nil
}

func (m *MockSoundCloudClient) PlaylistTracks(playlistID int64) ([]soundcloud.Track, error) {
	if m.PlaylistFunc != nil {
		return m.PlaylistFunc(playlistID)
	}
	return []soundcloud.Track{}, nil
}

func (m *MockSoundCloudClient) FavoriteTracks() ([]soundcloud.Track, error) {
	if m.FavoritesFunc != nil {
		return m.FavoritesFunc()
	}
	return []soundcloud.Track{}, nil
}
