package audio

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	soundcloudapi "github.com/zackradisic/soundcloud-api"
)

// StreamInfo represents information about an audio stream
type StreamInfo struct {
	URL      string
	Format   string
	Quality  string
	Duration int64
}

// TrackStreamRequest carries the SoundCloud track ID plus optional private
// playlist context needed when refetching stream metadata for library tracks.
type TrackStreamRequest struct {
	TrackID             int64
	PermalinkURL        string
	PlaylistID          int64
	PlaylistSecretToken string
	SecretToken         string
}

// StreamExtractor defines the interface for extracting streaming URLs
type StreamExtractor interface {
	// ExtractStreamURL extracts the actual streaming URL from a track
	ExtractStreamURL(ctx context.Context, trackID int64) (*StreamInfo, error)

	// GetAvailableQualities returns available quality options for a track
	GetAvailableQualities(ctx context.Context, trackID int64) ([]string, error)

	// ValidateStreamURL checks if a streaming URL is still valid
	ValidateStreamURL(ctx context.Context, streamURL string) (bool, error)
}

// TrackContextStreamExtractor is implemented by extractors that can use private
// playlist context instead of refetching stream metadata by a naked public ID.
type TrackContextStreamExtractor interface {
	ExtractTrackStreamURL(ctx context.Context, req TrackStreamRequest) (*StreamInfo, error)
}

// SoundCloudAPI interface for dependency injection and testing
type SoundCloudAPI interface {
	GetTrackInfo(options soundcloudapi.GetTrackInfoOptions) ([]soundcloudapi.Track, error)
}

// RealSoundCloudAPI interface includes methods for actual streaming URL extraction
type RealSoundCloudAPI interface {
	GetTrackInfoWithOptions(options soundcloudapi.GetTrackInfoOptions) ([]soundcloudapi.Track, error)
	GetDownloadURL(trackURL string, format string) (string, error)
}

// TranscodingURLResolver resolves a specific api-v2 media transcoding URL to a
// signed CDN URL, avoiding a second permalink resolve during playback.
type TranscodingURLResolver interface {
	GetTranscodingURL(ctx context.Context, transcodingURL string) (string, error)
}

// SoundCloudStreamExtractor implements StreamExtractor for SoundCloud
type SoundCloudStreamExtractor struct {
	api        SoundCloudAPI
	httpClient *http.Client
}

// ExtractorOption configures a SoundCloudStreamExtractor (dependency-injection seam).
type ExtractorOption func(*SoundCloudStreamExtractor)

// WithExtractorHTTPClient injects the HTTP client used by ValidateStreamURL's
// reachability check, so tests can supply a RoundTripper instead of hitting the
// network.
func WithExtractorHTTPClient(c *http.Client) ExtractorOption {
	return func(e *SoundCloudStreamExtractor) {
		if c != nil {
			e.httpClient = c
		}
	}
}

// NewSoundCloudStreamExtractor creates a new SoundCloud stream extractor
func NewSoundCloudStreamExtractor(clientID string) *SoundCloudStreamExtractor {
	var api *soundcloudapi.API
	var err error

	if clientID == "" {
		// Use default client (auto-fetches client ID)
		api, err = soundcloudapi.New(soundcloudapi.APIOptions{})
	} else {
		// Use provided client ID
		api, err = soundcloudapi.New(soundcloudapi.APIOptions{
			ClientID: clientID,
		})
	}

	if err != nil {
		// Return nil extractor if API creation fails
		return nil
	}

	return &SoundCloudStreamExtractor{
		api:        api,
		httpClient: &http.Client{},
	}
}

// NewSoundCloudStreamExtractorWithAPI creates an extractor with a custom API
// (for testing). Optional ExtractorOptions inject further dependencies such as
// the HTTP client used by ValidateStreamURL.
func NewSoundCloudStreamExtractorWithAPI(api SoundCloudAPI, opts ...ExtractorOption) *SoundCloudStreamExtractor {
	e := &SoundCloudStreamExtractor{
		api:        api,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// RealSoundCloudStreamExtractor implements StreamExtractor with actual API calls
type RealSoundCloudStreamExtractor struct {
	api RealSoundCloudAPI
}

// NewRealSoundCloudStreamExtractor creates a new real SoundCloud stream extractor
func NewRealSoundCloudStreamExtractor(api RealSoundCloudAPI) *RealSoundCloudStreamExtractor {
	return &RealSoundCloudStreamExtractor{
		api: api,
	}
}

// ExtractStreamURL extracts real streaming URLs from SoundCloud using GetDownloadURL
func (e *RealSoundCloudStreamExtractor) ExtractStreamURL(ctx context.Context, trackID int64) (*StreamInfo, error) {
	return e.ExtractTrackStreamURL(ctx, TrackStreamRequest{TrackID: trackID})
}

// ExtractTrackStreamURL extracts a streaming URL while preserving private
// playlist context required by SoundCloud's track metadata endpoint.
func (e *RealSoundCloudStreamExtractor) ExtractTrackStreamURL(ctx context.Context, req TrackStreamRequest) (*StreamInfo, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if e.api == nil {
		return nil, fmt.Errorf("SoundCloud API client not initialized")
	}

	if req.TrackID <= 0 {
		return nil, fmt.Errorf("invalid track ID: %d", req.TrackID)
	}

	// Get track information to obtain permalink URL
	tracks, err := e.api.GetTrackInfoWithOptions(trackInfoOptionsForStreamRequest(req))
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("track not found: %d", req.TrackID)
	}

	track := tracks[0]

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check if transcodings are available
	if len(track.Media.Transcodings) == 0 {
		return nil, fmt.Errorf("no transcodings available for track %d", req.TrackID)
	}

	candidates := chooseSoundCloudTranscodingCandidates(track.Media.Transcodings)
	if len(candidates) == 0 {
		if hasDRMEncryptedTranscoding(track.Media.Transcodings) {
			return nil, encryptedSoundCloudPlusError(req.TrackID)
		}
		return nil, fmt.Errorf("no supported transcoding formats available for track %d", req.TrackID)
	}

	var lastErr error
	for _, candidate := range candidates {
		streamURL, err := e.resolveSelectedTranscoding(ctx, track.PermalinkURL, candidate.quality, candidate.transcoding.URL)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, err
			}
			if isTranscodingNotFound(err) {
				continue
			}
			return nil, err
		}

		streamInfo := &StreamInfo{
			URL:      streamURL,
			Format:   streamFormat(candidate.transcoding),
			Quality:  candidate.quality,
			Duration: track.DurationMS,
		}
		return streamInfo, nil
	}

	if hasDRMEncryptedTranscoding(track.Media.Transcodings) {
		return nil, encryptedSoundCloudPlusError(req.TrackID)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no playable transcoding formats available for track %d", req.TrackID)
}

// trackInfoOptionsForStreamRequest chooses the SoundCloud metadata lookup that
// preserves private access: playlist context first, then secret permalink URL.
func trackInfoOptionsForStreamRequest(req TrackStreamRequest) soundcloudapi.GetTrackInfoOptions {
	if req.PlaylistID != 0 && req.PlaylistSecretToken != "" {
		return soundcloudapi.GetTrackInfoOptions{
			ID:                  []int64{req.TrackID},
			PlaylistID:          req.PlaylistID,
			PlaylistSecretToken: req.PlaylistSecretToken,
		}
	}
	if req.SecretToken != "" && req.PermalinkURL != "" {
		return soundcloudapi.GetTrackInfoOptions{
			URL: withSoundCloudSecretToken(req.PermalinkURL, req.SecretToken),
		}
	}
	return soundcloudapi.GetTrackInfoOptions{
		ID: []int64{req.TrackID},
	}
}

func withSoundCloudSecretToken(permalinkURL, secretToken string) string {
	u, err := url.Parse(permalinkURL)
	if err != nil {
		return permalinkURL
	}
	q := u.Query()
	q.Set("secret_token", secretToken)
	u.RawQuery = q.Encode()
	return u.String()
}

// resolveSelectedTranscoding resolves the exact transcoding selected from
// track metadata, falling back to the legacy permalink helper for old clients.
func (e *RealSoundCloudStreamExtractor) resolveSelectedTranscoding(ctx context.Context, permalinkURL, preferredFormat, transcodingURL string) (string, error) {
	if resolver, ok := e.api.(TranscodingURLResolver); ok && transcodingURL != "" {
		streamURL, err := resolver.GetTranscodingURL(ctx, transcodingURL)
		if err != nil {
			return "", fmt.Errorf("failed to resolve transcoding URL: %w", err)
		}
		return streamURL, nil
	}

	streamURL, err := e.api.GetDownloadURL(permalinkURL, preferredFormat)
	if err != nil {
		return "", fmt.Errorf("failed to get download URL: %w", err)
	}
	return streamURL, nil
}

type soundCloudTranscodingCandidate struct {
	quality     string
	transcoding *soundcloudapi.Transcoding
}

// chooseSoundCloudTranscodingCandidates orders playable stream candidates while
// excluding SoundCloud's DRM-only cbc/ctr encrypted HLS variants.
func chooseSoundCloudTranscodingCandidates(transcodings []soundcloudapi.Transcoding) []soundCloudTranscodingCandidate {
	var candidates []soundCloudTranscodingCandidate
	for i := range transcodings {
		transcoding := transcodings[i]
		if isSupportedHLSProtocol(transcoding) &&
			!strings.Contains(strings.ToLower(transcoding.Format.MimeType), "ogg") {
			candidates = append(candidates, soundCloudTranscodingCandidate{quality: "hls", transcoding: &transcodings[i]})
		}
	}
	for i := range transcodings {
		transcoding := transcodings[i]
		if isSupportedHLSProtocol(transcoding) &&
			strings.Contains(strings.ToLower(transcoding.Format.MimeType), "ogg") {
			candidates = append(candidates, soundCloudTranscodingCandidate{quality: "hls", transcoding: &transcodings[i]})
		}
	}
	for i := range transcodings {
		transcoding := transcodings[i]
		if strings.EqualFold(transcoding.Format.Protocol, "progressive") {
			candidates = append(candidates, soundCloudTranscodingCandidate{quality: "progressive", transcoding: &transcodings[i]})
		}
	}
	return candidates
}

// chooseSoundCloudTranscoding preserves the old single-candidate test seam while
// delegating to the ordered candidate list used by production extraction.
func chooseSoundCloudTranscoding(transcodings []soundcloudapi.Transcoding) (string, *soundcloudapi.Transcoding) {
	candidates := chooseSoundCloudTranscodingCandidates(transcodings)
	if len(candidates) == 0 {
		return "", nil
	}
	return candidates[0].quality, candidates[0].transcoding
}

func isSupportedHLSProtocol(transcoding soundcloudapi.Transcoding) bool {
	protocol := strings.ToLower(transcoding.Format.Protocol)
	if protocol == "hls" || protocol == "encrypted-hls" {
		return true
	}
	return strings.Contains(strings.ToLower(transcoding.URL), "/encrypted-hls") &&
		!isDRMEncryptedHLSProtocol(protocol)
}

func isDRMEncryptedHLSProtocol(protocol string) bool {
	protocol = strings.ToLower(protocol)
	return strings.HasPrefix(protocol, "cbc-") || strings.HasPrefix(protocol, "ctr-")
}

func hasDRMEncryptedTranscoding(transcodings []soundcloudapi.Transcoding) bool {
	for _, transcoding := range transcodings {
		if isDRMEncryptedHLSProtocol(transcoding.Format.Protocol) {
			return true
		}
	}
	return false
}

func streamFormat(transcoding *soundcloudapi.Transcoding) string {
	if transcoding == nil {
		return "mp3"
	}
	if isSupportedHLSProtocol(*transcoding) {
		return "hls"
	}
	if strings.EqualFold(transcoding.Format.MimeType, "audio/ogg") {
		return "ogg"
	}
	return "mp3"
}

func isTranscodingNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 404")
}

func encryptedSoundCloudPlusError(trackID int64) error {
	return fmt.Errorf("encrypted SoundCloud+ stream unsupported for track %d: SoundCloud returned only DRM-protected cbc/ctr encrypted HLS variants", trackID)
}

// GetAvailableQualities returns available qualities for track using real API
func (e *RealSoundCloudStreamExtractor) GetAvailableQualities(ctx context.Context, trackID int64) ([]string, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if e.api == nil {
		return nil, fmt.Errorf("SoundCloud API client not initialized")
	}

	if trackID <= 0 {
		return nil, fmt.Errorf("invalid track ID: %d", trackID)
	}

	// Get track information
	tracks, err := e.api.GetTrackInfoWithOptions(soundcloudapi.GetTrackInfoOptions{
		ID: []int64{trackID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("track not found: %d", trackID)
	}

	track := tracks[0]

	// Extract available qualities from transcodings
	qualityMap := make(map[string]bool)

	for _, transcoding := range track.Media.Transcodings {
		protocol := transcoding.Format.Protocol
		if protocol == "progressive" || protocol == "hls" {
			qualityMap[protocol] = true
		}
	}

	// Convert map to slice
	qualities := make([]string, 0, len(qualityMap))
	for quality := range qualityMap {
		qualities = append(qualities, quality)
	}

	// Ensure at least one quality is returned
	if len(qualities) == 0 {
		qualities = []string{"progressive", "hls"} // Default to both qualities
	}

	return qualities, nil
}

// ValidateStreamURL checks if stream URL is valid and not expired
func (e *RealSoundCloudStreamExtractor) ValidateStreamURL(ctx context.Context, streamURL string) (bool, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Validate input
	if streamURL == "" {
		return false, fmt.Errorf("stream URL cannot be empty")
	}

	// Parse URL to check if it's valid
	parsedURL, err := url.Parse(streamURL)
	if err != nil {
		return false, nil // Invalid URL format, but not an error
	}

	// Check if it's a valid HTTP/HTTPS URL
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false, nil // Invalid scheme
	}

	// Check if it's a SoundCloud CloudFront URL
	if !strings.Contains(parsedURL.Host, "sndcdn.com") {
		return false, nil // Not a SoundCloud URL
	}

	// Check for required authentication parameters
	queryParams := parsedURL.Query()
	if !queryParams.Has("Policy") || !queryParams.Has("Signature") || !queryParams.Has("Key-Pair-Id") {
		return false, nil // Missing authentication parameters
	}

	// URL passes basic validation
	return true, nil
}

// ExtractStreamURL extracts streaming URL from SoundCloud track
func (e *SoundCloudStreamExtractor) ExtractStreamURL(ctx context.Context, trackID int64) (*StreamInfo, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if e.api == nil {
		return nil, fmt.Errorf("SoundCloud API client not initialized")
	}

	if trackID <= 0 {
		return nil, fmt.Errorf("invalid track ID: %d", trackID)
	}

	// Get track information
	tracks, err := e.api.GetTrackInfo(soundcloudapi.GetTrackInfoOptions{
		ID: []int64{trackID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("track not found: %d", trackID)
	}

	track := tracks[0]

	// Check for context cancellation again
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check if transcodings are available
	if len(track.Media.Transcodings) == 0 {
		return nil, fmt.Errorf("no transcodings available for track %d", trackID)
	}

	// For now, use a mock streaming URL since we need the full implementation
	// In a real implementation, we'd use the transcoding URL and call GetMediaURL
	streamURL := fmt.Sprintf("https://cf-media.sndcdn.com/track_%d.mp3", trackID)

	// Create StreamInfo
	streamInfo := &StreamInfo{
		URL:      streamURL,
		Format:   "mp3", // Most SoundCloud tracks are MP3
		Quality:  "sq",  // Standard quality
		Duration: track.DurationMS,
	}

	return streamInfo, nil
}

// GetAvailableQualities returns available qualities for track
func (e *SoundCloudStreamExtractor) GetAvailableQualities(ctx context.Context, trackID int64) ([]string, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if e.api == nil {
		return nil, fmt.Errorf("SoundCloud API client not initialized")
	}

	if trackID <= 0 {
		return nil, fmt.Errorf("invalid track ID: %d", trackID)
	}

	// Get track information
	tracks, err := e.api.GetTrackInfo(soundcloudapi.GetTrackInfoOptions{
		ID: []int64{trackID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("track not found: %d", trackID)
	}

	track := tracks[0]

	// Extract available qualities from transcodings
	// SoundCloud typically has different qualities based on transcoding format
	qualityMap := make(map[string]bool)

	for _, transcoding := range track.Media.Transcodings {
		// Check format to determine quality
		if strings.ToLower(transcoding.Format.Protocol) == "progressive" {
			qualityMap["sq"] = true // Standard quality for progressive
		} else {
			qualityMap["hq"] = true // High quality for HLS
		}
	}

	// Convert map to slice
	qualities := make([]string, 0, len(qualityMap))
	for quality := range qualityMap {
		qualities = append(qualities, quality)
	}

	// Ensure at least one quality is returned
	if len(qualities) == 0 {
		qualities = []string{"sq", "hq"} // Default to both qualities
	}

	return qualities, nil
}

// ValidateStreamURL checks if stream URL is valid
func (e *SoundCloudStreamExtractor) ValidateStreamURL(ctx context.Context, streamURL string) (bool, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Validate input
	if streamURL == "" {
		return false, fmt.Errorf("stream URL cannot be empty")
	}

	// Parse URL to check if it's valid
	parsedURL, err := url.Parse(streamURL)
	if err != nil {
		return false, fmt.Errorf("invalid URL format: %w", err)
	}

	// Check if it's a valid HTTP/HTTPS URL
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false, fmt.Errorf("invalid URL scheme: %s", parsedURL.Scheme)
	}

	// Check if it looks like a SoundCloud media URL
	if !strings.Contains(parsedURL.Host, "sndcdn.com") &&
		!strings.Contains(parsedURL.Host, "soundcloud.com") {
		// Not a SoundCloud URL, but might still be valid
	}

	// Perform HEAD request to check if URL is accessible
	req, err := http.NewRequestWithContext(ctx, "HEAD", streamURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	client := e.httpClient
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		// URL is not accessible
		return false, nil
	}
	defer resp.Body.Close()

	// Consider 2xx status codes as valid
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	// URL exists but returned non-2xx status
	return false, nil
}
