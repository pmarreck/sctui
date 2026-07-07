package audio_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	soundcloudapi "github.com/zackradisic/soundcloud-api"

	"soundcloud-tui/internal/audio"
)

// trackWithProgressive builds a mock track carrying a single progressive
// transcoding and a positive duration.
func trackWithProgressive(id int64) []soundcloudapi.Track {
	return []soundcloudapi.Track{
		{
			ID:         id,
			Title:      "Test Track",
			DurationMS: 240000,
			Media: soundcloudapi.Media{
				Transcodings: []soundcloudapi.Transcoding{
					{
						Format: soundcloudapi.TranscodingFormat{
							Protocol: "progressive",
							MimeType: "audio/mpeg",
						},
					},
				},
			},
		},
	}
}

func TestSoundCloudStreamExtractor_ExtractStreamURL(t *testing.T) {
	tests := []struct {
		name        string
		trackID     int64
		api         audio.SoundCloudAPI
		wantErr     bool
		wantFormat  string
		wantQuality string
	}{
		{
			name:    "valid track ID returns stream info",
			trackID: 123456789,
			api: &MockSoundCloudAPI{
				GetTrackInfoFunc: func(soundcloudapi.GetTrackInfoOptions) ([]soundcloudapi.Track, error) {
					return trackWithProgressive(123456789), nil
				},
			},
			wantErr:     false,
			wantFormat:  "mp3",
			wantQuality: "sq",
		},
		{
			name:    "invalid track ID returns error",
			trackID: -1,
			api:     &MockSoundCloudAPI{},
			wantErr: true,
		},
		{
			name:    "nil API returns error",
			trackID: 123456789,
			api:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := audio.NewSoundCloudStreamExtractorWithAPI(tt.api)

			streamInfo, err := extractor.ExtractStreamURL(context.Background(), tt.trackID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, streamInfo)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, streamInfo)

			assert.NotEmpty(t, streamInfo.URL, "Stream URL should not be empty")
			assert.Equal(t, tt.wantFormat, streamInfo.Format)
			assert.Equal(t, tt.wantQuality, streamInfo.Quality)
			assert.Greater(t, streamInfo.Duration, int64(0), "Duration should be positive")
			assert.Contains(t, streamInfo.URL, "http", "Stream URL should be a valid HTTP URL")
		})
	}
}

func TestSoundCloudStreamExtractor_GetAvailableQualities(t *testing.T) {
	tests := []struct {
		name          string
		trackID       int64
		api           audio.SoundCloudAPI
		wantQualities []string
		wantErr       bool
	}{
		{
			name:    "valid track returns available qualities",
			trackID: 123456789,
			api: &MockSoundCloudAPI{
				GetTrackInfoFunc: func(soundcloudapi.GetTrackInfoOptions) ([]soundcloudapi.Track, error) {
					return []soundcloudapi.Track{
						{
							ID:         123456789,
							DurationMS: 240000,
							Media: soundcloudapi.Media{
								Transcodings: []soundcloudapi.Transcoding{
									{Format: soundcloudapi.TranscodingFormat{Protocol: "progressive"}},
									{Format: soundcloudapi.TranscodingFormat{Protocol: "hls"}},
								},
							},
						},
					}, nil
				},
			},
			wantQualities: []string{"sq", "hq"},
			wantErr:       false,
		},
		{
			name:    "invalid track ID returns error",
			trackID: -1,
			api:     &MockSoundCloudAPI{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := audio.NewSoundCloudStreamExtractorWithAPI(tt.api)

			qualities, err := extractor.GetAvailableQualities(context.Background(), tt.trackID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, qualities)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantQualities, qualities)
			assert.NotEmpty(t, qualities, "Should return at least one quality option")
		})
	}
}

func TestSoundCloudStreamExtractor_ValidateStreamURL(t *testing.T) {
	tests := []struct {
		name      string
		streamURL string
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "reachable SoundCloud URL returns true",
			streamURL: "https://cf-media.sndcdn.com/test.mp3",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "unreachable URL returns false",
			streamURL: "https://invalid-url.com/test.mp3",
			wantValid: false,
			wantErr:   false,
		},
		{
			name:      "empty URL returns error",
			streamURL: "",
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "malformed URL returns error",
			streamURL: "not-a-url",
			wantValid: false,
			wantErr:   true,
		},
	}

	// Injected HTTP client: 200 for sndcdn.com hosts, 404 for everything else.
	extractor := audio.NewSoundCloudStreamExtractorWithAPI(
		&MockSoundCloudAPI{},
		audio.WithExtractorHTTPClient(newStatusResponder("sndcdn.com")),
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := extractor.ValidateStreamURL(context.Background(), tt.streamURL)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValid, valid)
		})
	}
}

func TestSoundCloudStreamExtractor_ContextCancellation(t *testing.T) {
	extractor := audio.NewSoundCloudStreamExtractorWithAPI(&MockSoundCloudAPI{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := extractor.ExtractStreamURL(ctx, 123456789)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSoundCloudStreamExtractor_Timeout(t *testing.T) {
	extractor := audio.NewSoundCloudStreamExtractorWithAPI(&MockSoundCloudAPI{})

	// A zero timeout yields an already-expired context — deterministic, no sleep.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := extractor.ExtractStreamURL(ctx, 123456789)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}
