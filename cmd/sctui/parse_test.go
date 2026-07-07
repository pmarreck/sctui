package main

import "testing"

// TestValidateFlagStyle checks the flag-style guard as a classifier over sets:
// every "valid" arg vector must pass and every "invalid" one must be rejected.
func TestValidateFlagStyle(t *testing.T) {
	longNames := map[string]bool{
		"search": true, "track": true, "play": true,
		"test-audio": true, "test-tui": true, "help": true,
	}

	valid := [][]string{
		{"--search", "lofi"},
		{"-s", "lofi"},
		{"--search=lofi"},
		{"--track", "https://x"},
		{"-t", "https://x"},
		{"--help"},
		{"-h"},
		{"--test-audio", "url"},
		{"-lofi"},              // a value that looks flag-ish but isn't a known option
		{"--", "-search"},      // after "--" everything is positional
		{"lofi", "hip", "hop"}, // positionals
		{"-"},                  // lone dash (stdin convention)
		{},
	}
	for _, args := range valid {
		if err := validateFlagStyle(args, longNames); err != nil {
			t.Errorf("expected VALID, got error for %v: %v", args, err)
		}
	}

	invalid := [][]string{
		{"-search", "lofi"},
		{"-track", "url"},
		{"-play", "url"},
		{"-help"},
		{"-test-audio", "url"},
		{"-test-tui", "url"},
		{"-search=lofi"},
		{"--search", "x", "-track", "y"}, // offender is the third token
	}
	for _, args := range invalid {
		if err := validateFlagStyle(args, longNames); err == nil {
			t.Errorf("expected INVALID (error), got nil for %v", args)
		}
	}
}
