package soundcloud

import (
	"encoding/json"
	"fmt"
	"net/http"

	soundcloudapi "github.com/zackradisic/soundcloud-api"

	"soundcloud-tui/internal/session"
)

const apiV2 = "https://api-v2.soundcloud.com"

// Client wraps the SoundCloud API client
type Client struct {
	api        *soundcloudapi.API
	httpClient *http.Client
	authed     bool
	authSource string
}

// authTransport injects the web-session OAuth token on every request so the
// SoundCloud v2 API treats us as the signed-in user.
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "OAuth "+t.token)
	return t.base.RoundTrip(r2)
}

// Track represents a SoundCloud track
type Track struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Duration     int64  `json:"duration"` // Duration in milliseconds
	ArtworkURL   string `json:"artwork_url"`
	StreamURL    string `json:"stream_url"`
	PermalinkURL string `json:"permalink_url"`
	User         User   `json:"user"`
}

// Artist returns the artist name for the track
func (t Track) Artist() string {
	return t.User.FullName()
}

// DurationString returns a formatted duration string
func (t Track) DurationString() string {
	if t.Duration <= 0 {
		return "0:00"
	}

	totalSeconds := t.Duration / 1000
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// User represents a SoundCloud user
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// FullName returns the combined first and last name
func (u User) FullName() string {
	if u.FirstName == "" && u.LastName == "" {
		return u.Username
	}
	if u.FirstName == "" {
		return u.LastName
	}
	if u.LastName == "" {
		return u.FirstName
	}
	return u.FirstName + " " + u.LastName
}

// ClientInterface defines the interface for SoundCloud client
type ClientInterface interface {
	Search(query string) ([]Track, error)
	GetTrackInfo(url string) (*Track, error)
	GetDownloadURL(trackURL string, format string) (string, error)
}

// NewClient creates a SoundCloud client. It silently looks for a logged-in
// browser session; if found, requests authenticate as that user, otherwise it
// browses anonymously. Callers can use IsAuthenticated/AuthSource for a notice.
func NewClient() (*Client, error) {
	httpClient := &http.Client{}
	c := &Client{httpClient: httpClient}

	if tok := session.Find(); tok != nil {
		httpClient.Transport = &authTransport{token: tok.Value, base: http.DefaultTransport}
		c.authed = true
		c.authSource = tok.Source
	}

	api, err := soundcloudapi.New(soundcloudapi.APIOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("failed to create SoundCloud API client: %w", err)
	}
	c.api = api
	return c, nil
}

// IsAuthenticated reports whether a logged-in browser session was found.
func (c *Client) IsAuthenticated() bool { return c.authed }

// AuthSource describes where the session came from (e.g. "Firefox (default)").
func (c *Client) AuthSource() string { return c.authSource }

// GetTrackInfo retrieves track information by URL
func (c *Client) GetTrackInfo(url string) (*Track, error) {
	tracks, err := c.api.GetTrackInfo(soundcloudapi.GetTrackInfoOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("no track found for URL: %s", url)
	}

	// Use the first track from the results
	track := tracks[0]

	// Convert to our Track struct
	return &Track{
		ID:           track.ID,
		Title:        track.Title,
		Description:  track.Description,
		Duration:     track.DurationMS, // Use DurationMS field
		ArtworkURL:   track.ArtworkURL,
		PermalinkURL: track.PermalinkURL,
		User: User{
			ID:        track.User.ID,
			Username:  track.User.Username,
			FirstName: track.User.FirstName,
			LastName:  track.User.LastName,
		},
	}, nil
}

// Search searches for tracks on SoundCloud
func (c *Client) Search(query string) ([]Track, error) {
	paginatedQuery, err := c.api.Search(soundcloudapi.SearchOptions{
		Query:  query,
		Kind:   soundcloudapi.KindTrack, // Search only for tracks
		Limit:  50,                      // Increase limit for more results
		Offset: 0,                       // Start from beginning
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	tracks, err := paginatedQuery.GetTracks()
	if err != nil {
		return nil, fmt.Errorf("failed to get tracks from search: %w", err)
	}

	// Convert to our Track structs
	result := make([]Track, len(tracks))
	for i, track := range tracks {
		result[i] = Track{
			ID:           track.ID,
			Title:        track.Title,
			Description:  track.Description,
			Duration:     track.DurationMS, // Use DurationMS field
			ArtworkURL:   track.ArtworkURL,
			PermalinkURL: track.PermalinkURL,
			User: User{
				ID:        track.User.ID,
				Username:  track.User.Username,
				FirstName: track.User.FirstName,
				LastName:  track.User.LastName,
			},
		}
	}

	return result, nil
}

// GetDownloadURL gets a downloadable/streamable URL for a track
func (c *Client) GetDownloadURL(trackURL string, format string) (string, error) {
	// Use the SoundCloud API's GetDownloadURL method
	downloadURL, err := c.api.GetDownloadURL(trackURL, format)
	if err != nil {
		return "", fmt.Errorf("failed to get download URL: %w", err)
	}

	return downloadURL, nil
}

// GetTrackInfoWithOptions gets track info using SoundCloud API options (for RealSoundCloudAPI compatibility)
func (c *Client) GetTrackInfoWithOptions(options soundcloudapi.GetTrackInfoOptions) ([]soundcloudapi.Track, error) {
	tracks, err := c.api.GetTrackInfo(options)
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	return tracks, nil
}

// getV2 performs an authenticated GET against the SoundCloud v2 API and decodes
// the JSON body into v.
func (c *Client) getV2(path string, v any) error {
	resp, err := c.httpClient.Get(apiV2 + path)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api-v2 %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Me represents the authenticated SoundCloud user.
type Me struct {
	ID                    int64  `json:"id"`
	Username              string `json:"username"`
	FullName              string `json:"full_name"`
	FollowersCount        int    `json:"followers_count"`
	PrivatePlaylistsCount int    `json:"private_playlists_count"`
}

// Me returns the signed-in user, or an error if no browser session was found.
func (c *Client) Me() (*Me, error) {
	if !c.authed {
		return nil, fmt.Errorf("not signed in (no browser session found)")
	}
	var m Me
	if err := c.getV2("/me", &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Playlist is a SoundCloud playlist in the user's library.
type Playlist struct {
	ID           int64
	Title        string
	TrackCount   int
	Sharing      string // "public" | "private"
	Owner        string
	PermalinkURL string
	Kind         string // "owned" | "liked" | "system"
}

// IsPrivate reports whether the playlist is private.
func (p Playlist) IsPrivate() bool { return p.Sharing == "private" }

// Library returns the signed-in user's playlists: owned (including private),
// liked, and followed system playlists. It reads /me/library/all, since the
// older /me/playlists endpoint is gone (404).
func (c *Client) Library() ([]Playlist, error) {
	if !c.authed {
		return nil, fmt.Errorf("not signed in (no browser session found)")
	}
	var resp struct {
		Collection []struct {
			Type     string `json:"type"`
			Playlist *struct {
				ID           int64  `json:"id"`
				Title        string `json:"title"`
				Sharing      string `json:"sharing"`
				TrackCount   int    `json:"track_count"`
				PermalinkURL string `json:"permalink_url"`
				User         struct {
					Username string `json:"username"`
				} `json:"user"`
			} `json:"playlist"`
			SystemPlaylist *struct {
				Title        string `json:"title"`
				PermalinkURL string `json:"permalink_url"`
				TrackCount   int    `json:"track_count"`
			} `json:"system_playlist"`
		} `json:"collection"`
	}
	if err := c.getV2("/me/library/all?limit=200&linked_partitioning=1", &resp); err != nil {
		return nil, err
	}

	var out []Playlist
	for _, it := range resp.Collection {
		switch it.Type {
		case "playlist", "playlist-like":
			if it.Playlist == nil {
				continue
			}
			kind := "liked"
			if it.Type == "playlist" {
				kind = "owned"
			}
			out = append(out, Playlist{
				ID:           it.Playlist.ID,
				Title:        it.Playlist.Title,
				TrackCount:   it.Playlist.TrackCount,
				Sharing:      it.Playlist.Sharing,
				Owner:        it.Playlist.User.Username,
				PermalinkURL: it.Playlist.PermalinkURL,
				Kind:         kind,
			})
		case "system-playlist-like":
			if it.SystemPlaylist == nil {
				continue
			}
			out = append(out, Playlist{
				Title:        it.SystemPlaylist.Title,
				TrackCount:   it.SystemPlaylist.TrackCount,
				PermalinkURL: it.SystemPlaylist.PermalinkURL,
				Kind:         "system",
			})
		}
	}
	return out, nil
}
