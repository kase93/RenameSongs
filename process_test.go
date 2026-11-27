package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMP3 writes a file made of an ID3 tag plus fake audio bytes, which is
// enough for every code path that does not decode sound.
func writeMP3(t *testing.T, path string, version byte, frames []rawFrame) {
	t.Helper()
	if err := os.WriteFile(path, buildID3(version, frames, []byte("AUDIOBYTES")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// audioOf returns the payload that follows the ID3 tag.
func audioOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, audio, err := splitID3(data)
	if err != nil {
		t.Fatalf("splitID3(%s): %v", filepath.Base(path), err)
	}
	return string(audio)
}

// TestProcessRepairsTagsAndRenames is the end-to-end check the tool exists
// for: damaged or missing tags are written back, and only then the file is
// renamed from the repaired values.
func TestProcessRepairsTagsAndRenames(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "Rock")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}

	// Empty artist, title polluted with the file extension, no album, no
	// album artist, no track, no genre.
	path := filepath.Join(folder, "03. AC-DC - Back In Black.mp3")
	writeMP3(t, path, 3, []rawFrame{{id: "TIT2", body: textFrameBody(3, "Back In Black.mp3")}})

	result := process(folder, options{fixTags: true, forceTags: true, albumFromFolder: true, genreFromFolder: true})
	if result.errors != 0 {
		t.Fatalf("errors = %d", result.errors)
	}
	if result.tagsWritten != 1 {
		t.Errorf("tagsWritten = %d, want 1", result.tagsWritten)
	}

	expected := filepath.Join(folder, "AC-DC - Back In Black.mp3")
	if result.renamed != 1 || !exists(expected) {
		t.Fatalf("file was not renamed: renamed = %d", result.renamed)
	}

	current, err := readTags(expected)
	if err != nil {
		t.Fatalf("readTags: %v", err)
	}
	want := tagSet{
		Artist:      "AC-DC",
		Title:       "Back In Black",
		AlbumArtist: "AC-DC",
		Album:       "Rock",
		Genre:       "Rock",
		Track:       3,
	}
	if current != want {
		t.Errorf("tags = %+v\nwant  = %+v", current, want)
	}

	if got := audioOf(t, expected); got != "AUDIOBYTES" {
		t.Errorf("audio payload = %q", got)
	}
}

// TestProcessLeavesUnsupportedFormatsUntouched checks that a format without a
// tag writer is reported as such instead of being counted as an error, and
// that the file is still renamed.
func TestProcessLeavesUnsupportedFormatsUntouched(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "03. AC-DC - Back In Black.flac")
	if err := os.WriteFile(path, []byte("not really a flac"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := process(folder, options{fixTags: true, forceTags: true})
	if result.tagsUnsupported != 1 {
		t.Errorf("tagsUnsupported = %d, want 1", result.tagsUnsupported)
	}
	if result.errors != 0 {
		t.Errorf("errors = %d, want 0", result.errors)
	}
	if result.renamed != 1 || !exists(filepath.Join(folder, "AC-DC - Back In Black.flac")) {
		t.Errorf("file was not renamed: renamed = %d", result.renamed)
	}
}

// TestProcessDryRunWritesNothing guards the preview mode.
func TestProcessDryRunWritesNothing(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "03. AC-DC - Back In Black.mp3")
	writeMP3(t, path, 3, nil)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result := process(folder, options{dryRun: true, fixTags: true, forceTags: true})
	if result.renamed != 1 {
		t.Errorf("renamed = %d, want 1", result.renamed)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("original file disappeared: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("dry run modified the file")
	}
}

// TestProcessOverwriteReplacesExistingFile checks --overwrite: the target name
// is taken by another file of the folder, and that file is discarded instead
// of producing a "(2)" copy.
func TestProcessOverwriteReplacesExistingFile(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "AC-DC - Title.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMP3(t, filepath.Join(folder, "03. AC-DC - Title.mp3"), 3, nil)

	result := process(folder, options{overwrite: true})
	if result.errors != 0 {
		t.Fatalf("errors = %d", result.errors)
	}
	if result.replaced != 1 || result.renamed != 1 {
		t.Errorf("replaced = %d, renamed = %d", result.replaced, result.renamed)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "AC-DC - Title.mp3" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("files = %v, want only \"AC-DC - Title.mp3\"", names)
	}
	if got := audioOf(t, filepath.Join(folder, "AC-DC - Title.mp3")); got != "AUDIOBYTES" {
		t.Errorf("audio = %q", got)
	}
}

// TestProcessKeepsBothWithoutOverwrite is the default: nothing is lost.
func TestProcessKeepsBothWithoutOverwrite(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "AC-DC - Title.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMP3(t, filepath.Join(folder, "03. AC-DC - Title.mp3"), 3, nil)

	result := process(folder, options{})
	if result.errors != 0 {
		t.Fatalf("errors = %d", result.errors)
	}
	if result.replaced != 0 {
		t.Errorf("replaced = %d", result.replaced)
	}
	for _, name := range []string{"AC-DC - Title.mp3", "AC-DC - Title (2).mp3"} {
		if !exists(filepath.Join(folder, name)) {
			t.Errorf("%s is missing", name)
		}
	}
}
