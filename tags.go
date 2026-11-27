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
	Year   int
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
		// A missing or unreadable tag must not block the repair: the file name
		// still gives us artist and title, and the ID3 writer rebuilds the tag.
		return result, nil
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
	albumFromFolder bool
	genreFromFolder bool
}

// tagEditor applies tag changes to a clean copy of the current tags.
type tagEditor struct {
	tags    tagSet
	changes []change
	force   bool
}

// newTagEditor normalizes the stored values and keeps the cleaning as changes.
func newTagEditor(current tagSet, force bool) *tagEditor {
	cleaned, cleaning := cleanTags(current)
	return &tagEditor{tags: cleaned, changes: cleaning, force: force}
}

func (e *tagEditor) textValue(field string) string {
	switch field {
	case "artist":
		return e.tags.Artist
	case "title":
		return e.tags.Title
	case "album":
		return e.tags.Album
	case "album artist":
		return e.tags.AlbumArtist
	case "genre":
		return e.tags.Genre
	}
	return ""
}

func (e *tagEditor) text(field string, value string) {
	value = cleanTagText(value)
	if value == "" {
		return
	}
	old := e.textValue(field)
	if old == value || (old != "" && !e.force) {
		return
	}
	switch field {
	case "artist":
		e.tags.Artist = value
	case "title":
		e.tags.Title = value
	case "album":
		e.tags.Album = value
	case "album artist":
		e.tags.AlbumArtist = value
	case "genre":
		e.tags.Genre = value
	default:
		return
	}
	e.changes = append(e.changes, change{Field: field, Old: old, New: value})
}

func (e *tagEditor) number(field string, value int) {
	if value <= 0 {
		return
	}
	var old int
	switch field {
	case "track":
		old = e.tags.Track
	case "year":
		old = e.tags.Year
	default:
		return
	}
	if old == value || (old != 0 && !e.force) {
		return
	}
	switch field {
	case "track":
		e.tags.Track = value
	case "year":
		e.tags.Year = value
	}
	e.changes = append(e.changes, change{Field: field, Old: strconv.Itoa(old), New: strconv.Itoa(value)})
}

// cleanTagText normalizes a stored value: no stray spaces, no leftover file
// extension, no doubled whitespace.
func cleanTagText(text string) string {
	text = strings.TrimSpace(text)
	text = spaces.ReplaceAllString(text, " ")
	lower := strings.ToLower(text)
	for ext := range validExtensions {
		if strings.HasSuffix(lower, ext) && len(text) > len(ext) {
			text = strings.TrimSpace(text[:len(text)-len(ext)])
			break
		}
	}
	return text
}

// cleanTags fixes values that are clearly damaged before they are compared
// with the file name, and reports each fix as a change.
func cleanTags(current tagSet) (tagSet, []change) {
	cleaned := current
	cleaned.Artist = cleanTagText(current.Artist)
	cleaned.Title = cleanTagText(current.Title)
	cleaned.Album = cleanTagText(current.Album)
	cleaned.AlbumArtist = cleanTagText(current.AlbumArtist)
	cleaned.Genre = cleanTagText(current.Genre)

	// Some taggers keep the whole "Artist - Title" inside the title field.
	if artist, title := splitArtistTitle(cleaned.Title); artist != "" && title != "" && sameName(artist, cleaned.Artist) {
		cleaned.Title = title
	}

	var changes []change
	record := func(field, before, after string) {
		if before != after {
			changes = append(changes, change{Field: field, Old: before, New: after})
		}
	}
	record("artist", current.Artist, cleaned.Artist)
	record("title", current.Title, cleaned.Title)
	record("album", current.Album, cleaned.Album)
	record("album artist", current.AlbumArtist, cleaned.AlbumArtist)
	record("genre", current.Genre, cleaned.Genre)
	return cleaned, changes
}

// planTags decides which fields can be filled or corrected from local
// evidence: the file name first, then the folder name. It never invents a
// track total.
func planTags(current tagSet, fn fileNameInfo, folder string, opts planOptions) (tagSet, []change) {
	editor := newTagEditor(current, opts.forceTags)

	editor.text("artist", fn.Artist)
	editor.text("title", fn.Title)
	editor.number("track", fn.Track)

	if editor.tags.AlbumArtist == "" {
		editor.text("album artist", editor.tags.Artist)
	}
	if editor.tags.Album == "" && opts.albumFromFolder {
		editor.text("album", albumForFolder(folder))
	}
	if editor.tags.Genre == "" && opts.genreFromFolder {
		editor.text("genre", genreForFolder(folder))
	}
	// A year found in the file name is weak evidence, so it only fills a gap.
	if editor.tags.Year == 0 {
		editor.number("year", fn.Year)
	}

	return editor.tags, editor.changes
}

// genericFolders describe a container, not an album or a genre.
var genericFolders = map[string]bool{
	".": true, "..": true, "downloads": true, "music": true, "musica": true,
	"música": true, "playlists": true, "diferentes": true, "tonazo": true,
	"new folder": true, "temp": true, "tmp": true, "desktop": true,
	"renamesongs": true,
}

// folderLabel returns the folder name when it carries meaning, or "" when it
// is just a container such as Downloads or a drive root.
func folderLabel(folder string) string {
	name := strings.TrimSpace(folder)
	if name == "" {
		return ""
	}
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	if filepath.VolumeName(name) != "" {
		return ""
	}
	if genericFolders[strings.ToLower(name)] {
		return ""
	}
	return name
}

// albumForFolder uses the folder name as the album only when the folder has a
// meaningful name.
func albumForFolder(folder string) string {
	return folderLabel(folder)
}

// genreForFolder maps a folder name to a readable genre, skipping folders that
// are not genres.
func genreForFolder(folder string) string {
	return folderLabel(folder)
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
