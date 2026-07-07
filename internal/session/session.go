// Package session discovers a logged-in SoundCloud web session by reading the
// oauth_token cookie from a locally installed browser, so the app can act as
// the signed-in user (personal/private playlists, etc.). Firefox-family only
// for now — its cookie store keeps values in plaintext, unlike Chrome which
// encrypts them via the OS keyring.
package session

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Token is a discovered SoundCloud OAuth token plus a human-readable source.
type Token struct {
	Value  string
	Source string
}

// cookieQuery reads the SoundCloud web session token. host is the exact
// 'soundcloud.com' for this cookie, but we match subdomains defensively.
const cookieQuery = `SELECT value FROM moz_cookies ` +
	`WHERE host LIKE '%soundcloud.com' AND name = 'oauth_token' ` +
	`ORDER BY LENGTH(value) DESC LIMIT 1;`

// Find searches Firefox-family browsers for a logged-in SoundCloud session and
// returns its token, or nil if none is found (no browser, not logged in, or no
// sqlite3 available). It never fails hard — callers fall back to anonymous.
func Find() *Token {
	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil
	}
	for _, db := range firefoxCookieDBs(runtime.GOOS, os.Getenv) {
		if value := queryToken(sqlite3, db); value != "" {
			return &Token{Value: value, Source: profileLabel(db)}
		}
	}
	return nil
}

// firefoxProfileRoots returns candidate Firefox-family profile roots for the
// given OS using the provided env lookup. Pure and testable.
func firefoxProfileRoots(goos string, getenv func(string) string) []string {
	home := getenv("HOME")
	switch goos {
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles"),
		}
	case "windows":
		if ad := getenv("APPDATA"); ad != "" {
			return []string{filepath.Join(ad, "Mozilla", "Firefox", "Profiles")}
		}
		return nil
	default: // linux and other unixes
		return []string{
			filepath.Join(home, ".mozilla", "firefox"),                                       // native (all editions)
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),          // snap
			filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"), // flatpak
		}
	}
}

// firefoxCookieDBs returns cookies.sqlite paths across all discovered profiles.
func firefoxCookieDBs(goos string, getenv func(string) string) []string {
	var dbs []string
	for _, root := range firefoxProfileRoots(goos, getenv) {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			db := filepath.Join(root, e.Name(), "cookies.sqlite")
			if _, err := os.Stat(db); err == nil {
				dbs = append(dbs, db)
			}
		}
	}
	return dbs
}

// queryToken copies the cookie DB and its WAL/SHM sidecars to a temp dir, then
// runs sqlite3 on the copy. Copying the -wal lets sqlite3 include recent,
// not-yet-checkpointed writes; copying avoids the lock on Firefox's live DB.
// Returns "" on any failure.
func queryToken(sqlite3, db string) string {
	tmp, err := os.MkdirTemp("", "sctui-ck-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tmp)

	dst := filepath.Join(tmp, "cookies.sqlite")
	if err := copyFile(db, dst); err != nil {
		return ""
	}
	_ = copyFile(db+"-wal", dst+"-wal") // best-effort; may not exist
	_ = copyFile(db+"-shm", dst+"-shm")

	out, err := exec.Command(sqlite3, dst, cookieQuery).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// profileLabel produces a friendly source string like "Firefox (b5y7l11v.default)".
func profileLabel(db string) string {
	return "Firefox (" + filepath.Base(filepath.Dir(db)) + ")"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
