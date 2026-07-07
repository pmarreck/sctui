package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestFirefoxProfileRoots(t *testing.T) {
	env := func(k string) string {
		switch k {
		case "HOME":
			return "/home/me"
		case "APPDATA":
			return `C:\Users\me\AppData\Roaming`
		}
		return ""
	}

	linux := firefoxProfileRoots("linux", env)
	if len(linux) == 0 || linux[0] != "/home/me/.mozilla/firefox" {
		t.Errorf("linux roots = %v, want first /home/me/.mozilla/firefox", linux)
	}
	if !contains(linux, "/home/me/snap/firefox/common/.mozilla/firefox") {
		t.Errorf("linux roots missing snap path: %v", linux)
	}

	mac := firefoxProfileRoots("darwin", env)
	want := "/home/me/Library/Application Support/Firefox/Profiles"
	if len(mac) != 1 || mac[0] != want {
		t.Errorf("darwin roots = %v, want [%q]", mac, want)
	}

	win := firefoxProfileRoots("windows", env)
	if len(win) != 1 || filepath.Base(win[0]) != "Profiles" {
		t.Errorf("windows roots = %v", win)
	}
}

// TestQueryTokenRoundTrip builds a fixture cookies.sqlite with a known
// oauth_token and asserts queryToken reads exactly it back (host- and
// name-filtered), and returns "" when the cookie is absent.
func TestQueryTokenRoundTrip(t *testing.T) {
	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not on PATH")
	}
	dir := t.TempDir()
	db := filepath.Join(dir, "cookies.sqlite")
	const want = "2-abc-1234567890-deadbeefcafe"
	setup := `CREATE TABLE moz_cookies(name TEXT, value TEXT, host TEXT);
INSERT INTO moz_cookies VALUES('oauth_token','` + want + `','soundcloud.com');
INSERT INTO moz_cookies VALUES('sc_anonymous_id','nope','.soundcloud.com');
INSERT INTO moz_cookies VALUES('oauth_token','other-site-token','example.com');`
	if out, err := exec.Command(sqlite3, db, setup).CombinedOutput(); err != nil {
		t.Fatalf("fixture setup failed: %v: %s", err, out)
	}
	if got := queryToken(sqlite3, db); got != want {
		t.Errorf("queryToken = %q, want %q", got, want)
	}

	empty := filepath.Join(dir, "empty.sqlite")
	if out, err := exec.Command(sqlite3, empty, "CREATE TABLE moz_cookies(name TEXT, value TEXT, host TEXT);").CombinedOutput(); err != nil {
		t.Fatalf("empty fixture failed: %v: %s", err, out)
	}
	if got := queryToken(sqlite3, empty); got != "" {
		t.Errorf("queryToken(empty) = %q, want empty", got)
	}
}

func TestFirefoxCookieDBs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("root layout differs on windows; discovery covered on unix")
	}
	home := t.TempDir()
	prof := filepath.Join(home, ".mozilla", "firefox", "abc.default")
	if err := os.MkdirAll(prof, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(prof, "cookies.sqlite")
	if err := os.WriteFile(db, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	if got := firefoxCookieDBs(runtime.GOOS, env); !contains(got, db) {
		t.Errorf("firefoxCookieDBs = %v, want to contain %q", got, db)
	}
}
