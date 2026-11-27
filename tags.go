package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
)

// errUnsupportedFormat marks files whose tags cannot be written (everything
// except MP3).
var errUnsupportedFormat = errors.New("tag writing is not supported for this format")

// tagSet holds the fields this tool understands.
type tagSet struct {
	Artist      string
	Title       string
	Album       string
	AlbumArtist string
	Genre       string
	Track       int
	TrackTotal  int
	Year        int
}

// fileNameInfo is what can be inferred from the current file name.
type fileNameInfo struct {
	Artist string
	Title  string
	Track  int
}

type change struct {
	Field string
	Old   string
	New   string
}

// readTags reads the metadata with a format-agnostic parser.
func readTags(path string) (tagSet, error) {
	var result tagSet

	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil {
		if err == tag.ErrNoTagsFound {
			return result, nil
		}
		return result, err
	}

	result.Artist = strings.TrimSpace(meta.Artist())
	result.Title = strings.TrimSpace(meta.Title())
	result.Album = strings.TrimSpace(meta.Album())
	result.Genre = strings.TrimSpace(meta.Genre())
	result.Year = meta.Year()
	result.Track, result.TrackTotal = meta.Track()

	if raw := meta.Raw(); raw != nil {
		for _, key := range []string{"albumartist", "album artist", "TPE2", "aART"} {
			if value, ok := raw[key]; ok {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					result.AlbumArtist = strings.TrimSpace(text)
					break
				}
			}
		}
	}

	return result, nil
}

// missingFields lists the metadata fields that are empty.
func (t tagSet) missingFields() []string {
	var missing []string
	if t.Artist == "" {
		missing = append(missing, "artist")
	}
	if t.Title == "" {
		missing = append(missing, "title")
	}
	if t.Album == "" {
		missing = append(missing, "album")
	}
	if t.AlbumArtist == "" {
		missing = append(missing, "album artist")
	}
	if t.Track == 0 {
		missing = append(missing, "track")
	}
	if t.Year == 0 {
		missing = append(missing, "year")
	}
	if t.Genre == "" {
		missing = append(missing, "genre")
	}
	return missing
}

// mismatches lists fields whose tag value differs from the file name.
func (t tagSet) mismatches(fn fileNameInfo) []string {
	var issues []string
	if fn.Artist != "" && t.Artist != "" && !sameName(t.Artist, fn.Artist) {
		issues = append(issues, fmt.Sprintf("artist tag %q != file name %q", t.Artist, fn.Artist))
	}
	if fn.Title != "" && t.Title != "" && !sameName(t.Title, fn.Title) {
		issues = append(issues, fmt.Sprintf("title tag %q != file name %q", t.Title, fn.Title))
	}
	if fn.Track > 0 && t.Track > 0 && fn.Track != t.Track {
		issues = append(issues, fmt.Sprintf("track tag %d != file name %d", t.Track, fn.Track))
	}
	return issues
}

// sameName compares two names after applying the same file-name sanitizing
// used by renaming, so "AC/DC" matches "AC-DC".
func sameName(a, b string) bool {
	return strings.EqualFold(cleanName(a), cleanName(b))
}

type planOptions struct {
	forceTags       bool
	genreFromFolder bool
}

// planTags decides which fields can be filled or corrected from local
// evidence (file name and folder name). It never invents album or year.
func planTags(current tagSet, fn fileNameInfo, folder string, opts planOptions) (tagSet, []change) {
	updated := current
	var changes []change

	setText := func(field string, old string, value string) {
		if value == "" {
			return
		}
		if old == value || (old != "" && !opts.forceTags) {
			return
		}
		changes = append(changes, change{Field: field, Old: old, New: value})
		switch field {
		case "artist":
			updated.Artist = value
		case "title":
			updated.Title = value
		case "album artist":
			updated.AlbumArtist = value
		case "album":
			updated.Album = value
		case "genre":
			updated.Genre = value
		}
	}

	setNumber := func(field string, old int, value int) {
		if value <= 0 {
			return
		}
		if old == value || (old != 0 && !opts.forceTags) {
			return
		}
		changes = append(changes, change{Field: field, Old: strconv.Itoa(old), New: strconv.Itoa(value)})
		switch field {
		case "track":
			updated.Track = value
		case "year":
			updated.Year = value
		}
	}

	setText("artist", current.Artist, fn.Artist)
	setText("title", current.Title, fn.Title)
	setNumber("track", current.Track, fn.Track)

	if updated.AlbumArtist == "" {
		setText("album artist", "", updated.Artist)
	}

	if updated.Genre == "" && opts.genreFromFolder {
		if genre := genreForFolder(folder); genre != "" {
			setText("genre", "", genre)
		}
	}

	return updated, changes
}

// genreForFolder maps a folder name to a readable genre, skipping folders that
// are not genres.
func genreForFolder(folder string) string {
	name := strings.TrimSpace(folder)
	switch strings.ToLower(name) {
	case "", "playlists", "diferentes", "tonazo", "musica", "music", "downloads":
		return ""
	}
	return name
}

// writeTags writes the given fields back to the file. Only MP3 (ID3v2) is
// supported; other formats are reported as unsupported.
func writeTags(path string, updated tagSet, changes []change, backup bool) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".mp3" {
		return fmt.Errorf("%w: %s", errUnsupportedFormat, ext)
	}
	if len(changes) == 0 {
		return nil
	}
	return writeID3File(path, changes, updated, backup)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
