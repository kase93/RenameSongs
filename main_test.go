package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"AC/DC":             "AC-DC",
		"Back: In Black":    "Back - In Black",
		`Rock "n" Roll`:     "Rock 'n' Roll",
		"  many   spaces  ": "many spaces",
		"trailing dot.":     "trailing dot",
		"a*b?c":             "axbc",
	}
	for input, want := range cases {
		if got := cleanName(input); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeNameReserved(t *testing.T) {
	if got := normalizeName("CON"); got != "_CON" {
		t.Errorf("normalizeName(\"CON\") = %q, want \"_CON\"", got)
	}
	if got := normalizeName("nul.mp3"); got != "_nul.mp3" {
		t.Errorf("normalizeName(\"nul.mp3\") = %q, want \"_nul.mp3\"", got)
	}
	if got := normalizeName("   "); got != "_" {
		t.Errorf("normalizeName(empty) = %q, want \"_\"", got)
	}
}

func TestFromFileName(t *testing.T) {
	if a, ti := fromFileName("Daft Punk - Get Lucky.mp3"); a != "Daft Punk" || ti != "Get Lucky" {
		t.Errorf("fromFileName = (%q, %q)", a, ti)
	}
	if a, ti := fromFileName("Daft Punk - Get Lucky - Remix.flac"); a != "Daft Punk" || ti != "Get Lucky - Remix" {
		t.Errorf("fromFileName = (%q, %q)", a, ti)
	}
	if a, ti := fromFileName("untitled.mp3"); a != "" || ti != "" {
		t.Errorf("fromFileName without separator = (%q, %q)", a, ti)
	}
}

func TestAlreadyNamed(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		current, target string
		want            bool
	}{
		{"Daft Punk - Get Lucky.mp3", "Daft Punk - Get Lucky.mp3", true},
		{"daft punk - get lucky.MP3", "Daft Punk - Get Lucky.mp3", true},
		{"Daft Punk - Get Lucky (2).mp3", "Daft Punk - Get Lucky.mp3", true},
		{"Daft Punk - Get Lucky (12).mp3", "Daft Punk - Get Lucky.mp3", true},
		{"track01.mp3", "Daft Punk - Get Lucky.mp3", false},
		{"Daft Punk - Get Lucky (2).flac", "Daft Punk - Get Lucky.mp3", false},
	}
	for _, c := range cases {
		current := filepath.Join(dir, c.current)
		target := filepath.Join(dir, c.target)
		if got := alreadyNamed(current, target); got != c.want {
			t.Errorf("alreadyNamed(%q, %q) = %v, want %v", c.current, c.target, got, c.want)
		}
	}
}

func TestFreeNameDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	used := map[string]bool{}
	free := freeName(target, used)
	if filepath.Base(free) != "Artist - Song (2).mp3" {
		t.Errorf("freeName = %q, want \"Artist - Song (2).mp3\"", filepath.Base(free))
	}

	// A second file with the same name must get (3).
	if second := freeName(target, used); filepath.Base(second) != "Artist - Song (3).mp3" {
		t.Errorf("second freeName = %q, want \"Artist - Song (3).mp3\"", filepath.Base(second))
	}
}

func TestReorderArgs(t *testing.T) {
	got := reorderArgs([]string{"folder", "--dry-run", "-r"})
	want := []string{"--dry-run", "-r", "folder"}
	if len(got) != len(want) {
		t.Fatalf("reorderArgs = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("reorderArgs = %v, want %v", got, want)
		}
	}
}
