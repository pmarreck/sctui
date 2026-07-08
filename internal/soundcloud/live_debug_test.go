package soundcloud

import (
	"context"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	soundcloudapi "github.com/zackradisic/soundcloud-api"
)

const defaultLiveDebugTrackURL = "https://soundcloud.com/justice-official/new-lands"

func TestLiveDebugTrackTranscodings(t *testing.T) {
	if os.Getenv("SCTUI_LIVE_DEBUG") != "1" {
		t.Skip("set SCTUI_LIVE_DEBUG=1")
	}

	trackURL := os.Getenv("SCTUI_LIVE_DEBUG_TRACK_URL")
	if trackURL == "" {
		trackURL = defaultLiveDebugTrackURL
	}

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("auth: signed_in=%v source=%q", client.IsAuthenticated(), client.AuthSource())
	t.Logf("track_url=%s", trackURL)

	tracks, err := client.GetTrackInfoWithOptions(soundcloudapi.GetTrackInfoOptions{URL: trackURL})
	if err != nil {
		t.Fatalf("track metadata: %v", err)
	}
	for _, track := range tracks {
		t.Logf("track: id=%d title=%q policy=%q public=%v streamable=%v monetization=%q secret_present=%v permalink=%q transcodings=%d",
			track.ID,
			track.Title,
			track.Policy,
			track.Public,
			track.Streamable,
			track.MonetizationModel,
			track.SecretToken != "",
			track.PermalinkURL,
			len(track.Media.Transcodings),
		)
		for i, transcoding := range track.Media.Transcodings {
			t.Logf("transcoding[%d]: protocol=%q mime=%q preset=%q snipped=%v url=%s",
				i,
				transcoding.Format.Protocol,
				transcoding.Format.MimeType,
				transcoding.Preset,
				transcoding.Snipped,
				redactDebugURL(transcoding.URL),
			)
			mediaURL, err := client.GetTranscodingURL(context.Background(), transcoding.URL)
			if err != nil {
				t.Logf("transcoding[%d] resolve_error=%v", i, err)
				continue
			}
			t.Logf("transcoding[%d] media=%s", i, redactDebugURL(mediaURL))
			logPlaylistHeader(t, client, i, mediaURL)
		}
	}
}

func logPlaylistHeader(t *testing.T, client *Client, index int, playlistURL string) {
	t.Helper()
	httpClient := client.httpClient
	if httpClient == nil {
		return
	}
	resp, err := httpClient.Get(playlistURL)
	if err != nil {
		t.Logf("transcoding[%d] playlist_error=%v", index, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		t.Logf("transcoding[%d] playlist_status=%d", index, resp.StatusCode)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		t.Logf("transcoding[%d] playlist_read_error=%v", index, err)
		return
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-KEY") ||
			strings.HasPrefix(line, "#EXT-X-SESSION-KEY") ||
			strings.HasPrefix(line, "#EXT-X-MAP") ||
			strings.HasPrefix(line, "#EXT-X-STREAM-INF") ||
			strings.HasPrefix(line, "#EXT-X-MEDIA") ||
			strings.HasPrefix(line, "#EXT-X-VERSION") ||
			strings.HasPrefix(line, "#EXT-X-TARGETDURATION") ||
			strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE") ||
			strings.HasPrefix(line, "#EXT-X-INDEPENDENT-SEGMENTS") {
			t.Logf("transcoding[%d] playlist: %s", index, redactDebugLine(line))
		}
	}
}

func redactDebugLine(line string) string {
	if strings.HasPrefix(line, "#EXT-X-KEY") || strings.HasPrefix(line, "#EXT-X-SESSION-KEY") {
		return redactKeyTag(line)
	}
	parts := strings.Split(line, "\"")
	for i, part := range parts {
		if strings.HasPrefix(part, "http://") || strings.HasPrefix(part, "https://") {
			parts[i] = redactDebugURL(part)
		}
	}
	return strings.Join(parts, "\"")
}

func redactKeyTag(line string) string {
	prefix, rest, ok := strings.Cut(line, ":")
	if !ok {
		return line
	}
	attrs := strings.Split(rest, ",")
	for i, attr := range attrs {
		name, _, ok := strings.Cut(attr, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(name) {
		case "URI", "IV", "KEYID":
			attrs[i] = name + "=[redacted]"
		}
	}
	return prefix + ":" + strings.Join(attrs, ",")
}

func redactDebugURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.RawQuery != "" {
		u.RawQuery = "[redacted]"
	}
	return u.String()
}
