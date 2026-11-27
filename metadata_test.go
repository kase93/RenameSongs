package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhowden/tag"
)

func sampleTag(version byte) []byte {
	frames := []rawFrame{
		{id: "TPE1", body: textFrameBody(version, "AC/DC")},
		{id: "TIT2", body: textFrameBody(version, "Back In Black")},
		{id: "APIC", body: []byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0, 1, 2, 3, 4, 5}},
	}
	return buildID3(version, frames, []byte("AUDIOBYTES"))
}

func TestID3RoundTrip(t *testing.T) {
	for _, version := range []byte{3, 4} {
		data := sampleTag(version)
		gotVersion, frames, audio, err := splitID3(data)
		if err != nil {
			t.Fatalf("v%d split: %v", version, err)
		}
		if gotVersion != version {
			t.Errorf("v%d: version = %d", version, gotVersion)
		}
		if len(frames) != 3 {
			t.Fatalf("v%d: %d frames", version, len(frames))
		}
		if string(audio) != "AUDIOBYTES" {
			t.Errorf("v%d: audio = %q", version, audio)
		}
		if frames[2].id != "APIC" || !bytes.Equal(frames[2].body, []byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0, 1, 2, 3, 4, 5}) {
			t.Errorf("v%d: picture frame was not preserved", version)
		}
	}
}

func TestWriteTagsKeepsPictureAndUpdatesFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ACDC - Back In Black.mp3")
	if err := os.WriteFile(path, sampleTag(3), 0o644); err != nil {
		t.Fatal(err)
	}

	updated := tagSet{Artist: "AC/DC", Title: "Back In Black", AlbumArtist: "AC/DC", Genre: "Rock"}
	changes := []change{
		{Field: "album artist", New: "AC/DC"},
		{Field: "genre", New: "Rock"},
	}
	if err := writeTags(path, updated, changes, false); err != nil {
		t.Fatalf("writeTags: %v", err)
	}

	current, err := readTags(path)
	if err != nil {
		t.Fatalf("readTags: %v", err)
	}
	if current.AlbumArtist != "AC/DC" || current.Genre != "Rock" {
		t.Errorf("tags = %+v", current)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	picture := meta.Picture()
	if picture == nil || !bytes.Equal(picture.Data, []byte{1, 2, 3, 4, 5}) {
		t.Errorf("picture was not preserved: %+v", picture)
	}
}

func TestWriteTagsSplitsExistingUTF16(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artist - title.mp3")
	if err := os.WriteFile(path, sampleTag(3), 0o644); err != nil {
		t.Fatal(err)
	}

	updated := tagSet{Artist: "Nuevo Artista", Title: "Back In Black"}
	if err := writeTags(path, updated, []change{{Field: "artist", New: "Nuevo Artista"}}, true); err != nil {
		t.Fatalf("writeTags: %v", err)
	}

	current, err := readTags(path)
	if err != nil {
		t.Fatalf("readTags: %v", err)
	}
	if current.Artist != "Nuevo Artista" {
		t.Errorf("artist = %q", current.Artist)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("backup not created: %v", err)
	}
}

func TestWriteTagsRejectsOtherFormats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.m4a")
	if err := os.WriteFile(path, []byte("whatever"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeTags(path, tagSet{}, []change{{Field: "artist", New: "x"}}, false)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPlanTagsDoesNotUseFolderMetadataWhenDisabled(t *testing.T) {
	inferred := fileNameInfo{Artist: "AC/DC", Title: "Back In Black", Track: 1}
	updated, changes := planTags(tagSet{}, inferred, "Clasicos", planOptions{})
	if updated.Album != "" || updated.Year != 0 {
		t.Errorf("album/year should stay empty: %+v", updated)
	}
	if updated.Artist != "AC/DC" || updated.Title != "Back In Black" || updated.Track != 1 {
		t.Errorf("fields not filled: %+v", updated)
	}
	if len(changes) != 4 { // artist, title, track, album artist
		t.Errorf("changes = %v", changes)
	}
}

func TestPlanTagsRepairsEveryField(t *testing.T) {
	current := tagSet{Artist: "ACDC", Title: "ACDC - Back In Black.mp3"}
	inferred := fileNameInfo{Artist: "AC/DC", Title: "Back In Black", Track: 3, Year: 1980}
	opts := planOptions{forceTags: true, albumFromFolder: true, genreFromFolder: true}

	updated, changes := planTags(current, inferred, filepath.Join("Rock", "Clasicos"), opts)

	want := tagSet{
		Artist: "AC/DC", Title: "Back In Black", Track: 3,
		AlbumArtist: "AC/DC", Album: "Clasicos", Genre: "Clasicos", Year: 1980,
	}
	if updated != want {
		t.Errorf("updated = %+v\nwant    = %+v", updated, want)
	}
	if len(changes) == 0 {
		t.Error("expected changes")
	}
}

func TestPlanTagsKeepsBadAlbumArtistAndYear(t *testing.T) {
	current := tagSet{Artist: "AC/DC", Title: "Back In Black", AlbumArtist: "Various Artists", Year: 1990}
	inferred := fileNameInfo{Artist: "AC/DC", Title: "Back In Black", Year: 1980}
	opts := planOptions{forceTags: true, albumFromFolder: true, genreFromFolder: true}

	updated, _ := planTags(current, inferred, "Clasicos", opts)
	if updated.AlbumArtist != "Various Artists" {
		t.Errorf("album artist was overwritten: %q", updated.AlbumArtist)
	}
	if updated.Year != 1990 {
		t.Errorf("year was overwritten: %d", updated.Year)
	}
}

func TestFolderMetadataSkipsGenericFolders(t *testing.T) {
	cases := map[string]string{
		"Downloads":                    "",
		filepath.Join("C:\\", "Music"): "",
		"Clasicos":                     "Clasicos",
		filepath.Join("Rock", "90s"):   "90s",
	}
	for folder, want := range cases {
		if got := albumForFolder(folder); got != want {
			t.Errorf("albumForFolder(%q) = %q, want %q", folder, got, want)
		}
	}
}

func TestCleanTagTextRemovesExtension(t *testing.T) {
	if got := cleanTagText("  Back   In Black.mp3 "); got != "Back In Black" {
		t.Errorf("cleanTagText = %q", got)
	}
	if got := cleanTagText("Back In Black"); got != "Back In Black" {
		t.Errorf("cleanTagText = %q", got)
	}
}

func TestPlanTagsRespectsExistingValuesWithoutForce(t *testing.T) {
	current := tagSet{Artist: "ACDC", Title: "Back In Black"}
	updated, _ := planTags(current, fileNameInfo{Artist: "AC/DC", Title: "Back In Black"}, "x", planOptions{})
	if updated.Artist != "ACDC" {
		t.Errorf("artist was overwritten without --force-tags: %q", updated.Artist)
	}

	forced, _ := planTags(current, fileNameInfo{Artist: "AC/DC", Title: "Back In Black"}, "x", planOptions{forceTags: true})
	if forced.Artist != "AC/DC" {
		t.Errorf("artist was not forced: %q", forced.Artist)
	}
}

func TestParseFileNameWithTrackPrefix(t *testing.T) {
	info := parseFileName("01. David Guetta - Hey Mama (feat. Nicki Minaj).mp3")
	if info.Track != 1 || info.Artist != "David Guetta" || info.Title != "Hey Mama (feat. Nicki Minaj)" {
		t.Errorf("parseFileName = %+v", info)
	}
}

func TestReadTagsOnFakeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, sampleTag(4), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := readTags(path)
	if err != nil {
		t.Fatalf("readTags: %v", err)
	}
	if current.Artist != "AC/DC" || current.Title != "Back In Black" {
		t.Errorf("tags = %+v", current)
	}
}

func TestParseFileNameYear(t *testing.T) {
	info := parseFileName("ACDC - Back In Black (1980).mp3")
	if info.Artist != "ACDC" || info.Title != "Back In Black (1980)" || info.Year != 1980 {
		t.Errorf("parseFileName = %+v", info)
	}
	info = parseFileName("AC/DC - Back In Black [1980].flac")
	if info.Year != 1980 {
		t.Errorf("bracket year not found: %+v", info)
	}
	if info := parseFileName("ACDC - Back In Black.mp3"); info.Year != 0 {
		t.Errorf("unexpected year: %+v", info)
	}
}
