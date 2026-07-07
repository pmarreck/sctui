package soundcloud

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
