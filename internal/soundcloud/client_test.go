package soundcloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPlaylistTracksHydratesShallowTracks(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.RequestURI() {
		case "/playlists/99":
			fmt.Fprint(w, `{
				"tracks": [
					{
						"id": 101,
						"title": "Full fixture track",
						"duration": 61000,
						"artwork_url": "https://img.example/full.jpg",
						"permalink_url": "https://soundcloud.com/peter/full",
						"user": {"id": 1, "username": "fullartist"}
					},
					{"id": 202},
					{"id": 303}
				]
			}`)
		case "/tracks?ids=202,303":
			fmt.Fprint(w, `[
				{
					"id": 303,
					"title": "Hydrated second shallow track",
					"duration": 303000,
					"permalink_url": "https://soundcloud.com/peter/second",
					"user": {"id": 3, "username": "secondartist"}
				},
				{
					"id": 202,
					"title": "Hydrated first shallow track",
					"duration": 202000,
					"permalink_url": "https://soundcloud.com/peter/first",
					"user": {"id": 2, "username": "firstartist"}
				}
			]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient:   server.Client(),
		authed:       true,
		apiV2BaseURL: server.URL,
	}

	tracks, err := client.PlaylistTracks(99)
	if err != nil {
		t.Fatalf("PlaylistTracks returned error: %v", err)
	}

	gotTitles := make([]string, 0, len(tracks))
	for _, track := range tracks {
		gotTitles = append(gotTitles, track.Title)
	}
	wantTitles := []string{
		"Full fixture track",
		"Hydrated first shallow track",
		"Hydrated second shallow track",
	}
	if !reflect.DeepEqual(gotTitles, wantTitles) {
		t.Fatalf("tracks were not hydrated in playlist order:\n got: %#v\nwant: %#v", gotTitles, wantTitles)
	}

	if tracks[1].User.Username != "firstartist" || tracks[2].Duration != 303000 {
		t.Fatalf("hydrated metadata missing: %#v", tracks)
	}

	wantRequests := []string{"/playlists/99", "/tracks?ids=202,303"}
	if !reflect.DeepEqual(seen, wantRequests) {
		t.Fatalf("unexpected API requests:\n got: %#v\nwant: %#v", seen, wantRequests)
	}
}

func TestPlaylistTracksHydratesLargeShallowPlaylistsInBatches(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/playlists/499":
			fmt.Fprint(w, `{"tracks":[`)
			for i := 0; i < 205; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"id":%d}`, 1000+i)
			}
			fmt.Fprint(w, `]}`)
		case "/tracks":
			ids := strings.Split(r.URL.Query().Get("ids"), ",")
			if len(ids) > 100 {
				http.Error(w, "too many ids", http.StatusRequestURITooLong)
				return
			}
			fmt.Fprint(w, `[`)
			for i, idText := range ids {
				id, err := strconv.ParseInt(idText, 10, 64)
				if err != nil {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"id":%d,"title":"Track %d","duration":%d,"user":{"username":"artist%d"}}`, id, id, id*100, id)
			}
			fmt.Fprint(w, `]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient:   server.Client(),
		authed:       true,
		apiV2BaseURL: server.URL,
	}

	tracks, err := client.PlaylistTracks(499)
	if err != nil {
		t.Fatalf("PlaylistTracks returned error: %v", err)
	}
	if len(tracks) != 205 {
		t.Fatalf("got %d tracks, want 205", len(tracks))
	}
	if tracks[0].Title != "Track 1000" || tracks[100].Title != "Track 1100" || tracks[204].Title != "Track 1204" {
		t.Fatalf("large playlist was not hydrated in order: first=%q middle=%q last=%q", tracks[0].Title, tracks[100].Title, tracks[204].Title)
	}
	if len(seen) != 6 {
		t.Fatalf("got %d API requests, want playlist + 5 hydration batches: %#v", len(seen), seen)
	}
}

func TestPlaylistTracksHydratesVeryLargePrivatePlaylistsInSoundCloudSizedBatches(t *testing.T) {
	var trackBatchSizes []int
	var sawPlaylistContext bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/playlists/777":
			fmt.Fprint(w, `{"secret_token":"playlist-secret","tracks":[`)
			for i := 0; i < 305; i++ {
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"id":%d}`, 2000+i)
			}
			fmt.Fprint(w, `]}`)
		case "/tracks":
			ids := strings.Split(r.URL.Query().Get("ids"), ",")
			trackBatchSizes = append(trackBatchSizes, len(ids))
			if len(ids) > 50 {
				http.Error(w, "too many ids", http.StatusRequestURITooLong)
				return
			}
			if r.URL.Query().Get("playlistId") != "777" || r.URL.Query().Get("playlistSecretToken") != "playlist-secret" {
				http.Error(w, "missing playlist context", http.StatusForbidden)
				return
			}
			sawPlaylistContext = true
			fmt.Fprint(w, `[`)
			for i, idText := range ids {
				id, err := strconv.ParseInt(idText, 10, 64)
				if err != nil {
					http.Error(w, "bad id", http.StatusBadRequest)
					return
				}
				if i > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"id":%d,"title":"Track %d","duration":%d,"user":{"username":"artist%d"}}`, id, id, id*100, id)
			}
			fmt.Fprint(w, `]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient:   server.Client(),
		authed:       true,
		apiV2BaseURL: server.URL,
	}

	tracks, err := client.PlaylistTracks(777)
	if err != nil {
		t.Fatalf("PlaylistTracks returned error: %v", err)
	}
	if len(tracks) != 305 {
		t.Fatalf("got %d tracks, want 305", len(tracks))
	}
	if tracks[0].Title != "Track 2000" || tracks[304].Title != "Track 2304" {
		t.Fatalf("large private playlist was not hydrated in order: first=%q last=%q", tracks[0].Title, tracks[304].Title)
	}
	if !reflect.DeepEqual(trackBatchSizes, []int{50, 50, 50, 50, 50, 50, 5}) {
		t.Fatalf("unexpected hydration batch sizes: %#v", trackBatchSizes)
	}
	if !sawPlaylistContext {
		t.Fatalf("hydration never included playlist context")
	}
}

func TestGetTranscodingURLAddsClientIDAndDecodesMediaURL(t *testing.T) {
	var gotClientID string
	var gotExisting string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media/track/stream/hls" {
			http.NotFound(w, r)
			return
		}
		gotClientID = r.URL.Query().Get("client_id")
		gotExisting = r.URL.Query().Get("existing")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"url":"https://cf-media.sndcdn.com/playlist.m3u8?Policy=signed"}`)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		clientID:   "client-fixture",
	}

	mediaURL, err := client.GetTranscodingURL(context.Background(), server.URL+"/media/track/stream/hls?existing=1")
	if err != nil {
		t.Fatalf("GetTranscodingURL returned error: %v", err)
	}
	if mediaURL != "https://cf-media.sndcdn.com/playlist.m3u8?Policy=signed" {
		t.Fatalf("media URL mismatch: %q", mediaURL)
	}
	if gotClientID != "client-fixture" || gotExisting != "1" {
		t.Fatalf("unexpected media URL query: client_id=%q existing=%q", gotClientID, gotExisting)
	}
}

func TestFavoriteTracksParsesLikedTracks(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.RequestURI() {
		case "/me":
			fmt.Fprint(w, `{"id": 9867108}`)
		case "/users/9867108/track_likes?limit=200&linked_partitioning=1":
			fmt.Fprint(w, `{
				"collection": [
					{
						"track": {
							"id": 404,
							"title": "Liked fixture track",
							"duration": 184000,
							"permalink_url": "https://soundcloud.com/peter/liked",
							"user": {"id": 4, "username": "likedartist"}
						}
					},
					{"track": null},
					{
						"track": {
							"id": 505,
							"title": "Another favorite",
							"duration": 205000,
							"user": {"id": 5, "username": "anotherartist"}
						}
					}
				]
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient:   server.Client(),
		authed:       true,
		apiV2BaseURL: server.URL,
	}

	tracks, err := client.FavoriteTracks()
	if err != nil {
		t.Fatalf("FavoriteTracks returned error: %v", err)
	}

	gotTitles := make([]string, 0, len(tracks))
	for _, track := range tracks {
		gotTitles = append(gotTitles, track.Title)
	}
	wantTitles := []string{"Liked fixture track", "Another favorite"}
	if !reflect.DeepEqual(gotTitles, wantTitles) {
		t.Fatalf("favorite tracks mismatch:\n got: %#v\nwant: %#v", gotTitles, wantTitles)
	}

	if tracks[0].Artist() != "likedartist" || tracks[1].Duration != 205000 {
		t.Fatalf("favorite track metadata missing: %#v", tracks)
	}

	wantRequests := []string{"/me", "/users/9867108/track_likes?limit=200&linked_partitioning=1"}
	if !reflect.DeepEqual(seen, wantRequests) {
		t.Fatalf("unexpected API requests:\n got: %#v\nwant: %#v", seen, wantRequests)
	}
}
