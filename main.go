// RenameSongs renames audio files to "Artist - Title.ext" using the
// metadata embedded in each file.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	appName = "rename_songs"
	version = "1.5.0"
)

// validExtensions lists the audio formats we attempt to process.
var validExtensions = map[string]bool{
	".mp3": true, ".flac": true, ".m4a": true, ".mp4": true,
	".ogg": true, ".oga": true, ".opus": true, ".wav": true,
	".aac": true, ".wma": true, ".aiff": true, ".ape": true,
}

// replacements maps characters that are invalid in file names (Windows is
// the most restrictive) to visually similar substitutes.
var replacements = map[string]string{
	"\\": "-",
	"/":  "-",
	":":  " -",
	"*":  "x",
	"?":  "",
	"\"": "'",
	"<":  "(",
	">":  ")",
	"|":  "-",
}

var (
	controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	spaces       = regexp.MustCompile(`\s+`)
	dashes       = regexp.MustCompile(`\s+-\s+`)

	// Matches names such as "Name (2)" or "Name (12)".
	numberedSuffix      = regexp.MustCompile(`^.+?\s+\(\d+\)$`)
	numberedSuffixStrip = regexp.MustCompile(`\s+\(\d+\)$`)

	// Matches a leading track number such as "01. ", "01 - " or "01)".
	trackPrefix = regexp.MustCompile(`^(\d{1,3})\s*[.)\-]\s*`)

	// Matches a trailing release year such as "Song (2019)" or "Song [2019]".
	yearSuffix = regexp.MustCompile(`[\(\[]((?:19|20)\d{2})[\)\]]$`)
)

// reservedNames are device names Windows refuses to use for files.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func main() {
	dryRun := flag.Bool("dry-run", false, "show the changes without writing anything (alias: -n)")
	dryRunShort := flag.Bool("n", false, "alias of --dry-run")
	recursive := flag.Bool("recursive", false, "also process subfolders (alias: -r)")
	recursiveShort := flag.Bool("r", false, "alias of --recursive")
	audit := flag.Bool("audit", false, "list metadata problems without changing anything")
	noTags := flag.Bool("no-tags", false, "only rename; do not repair metadata")
	forceTags := flag.Bool("force-tags", true, "make tags agree with the file name (use --force-tags=false to keep existing tags)")
	backup := flag.Bool("backup", false, "keep a .bak copy before writing tags")
	overwrite := flag.Bool("overwrite", false, "replace a file that already has the target name instead of keeping both")
	albumFromFolder := flag.Bool("album-from-folder", true, "fill an empty album with the folder name (use =false to disable)")
	genreFromFolder := flag.Bool("genre-from-folder", true, "fill an empty genre with the folder name (use =false to disable)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [folder] [options]\n", appName)
		fmt.Fprintln(flag.CommandLine.Output(),
			"Without a folder, it fixes and renames the audio files in the folder\n"+
				"that contains the executable. Drop rename_songs.exe in a music folder\n"+
				"and double-click it, or pass another folder as the argument.")
		flag.PrintDefaults()
	}

	// Allow options before or after the folder: the flag package stops
	// parsing at the first positional argument.
	if err := flag.CommandLine.Parse(reorderArgs(os.Args[1:])); err != nil {
		os.Exit(2)
	}

	if *showVersion {
		fmt.Printf("%s %s\n", appName, version)
		return
	}

	folder := flag.Arg(0)
	noFolder := folder == ""
	if noFolder {
		folder = executableFolder()
	}

	fmt.Printf("Folder: %s\n\n", folder)

	if *noTags {
		*forceTags = false
	}

	opts := options{
		dryRun:          *dryRun || *dryRunShort,
		recursive:       *recursive || *recursiveShort,
		audit:           *audit,
		fixTags:         !*noTags,
		forceTags:       *forceTags,
		backup:          *backup,
		overwrite:       *overwrite,
		albumFromFolder: *albumFromFolder,
		genreFromFolder: *genreFromFolder,
	}

	stats := process(folder, opts)
	printSummary(stats, opts)

	// Pause when launched by double click (no folder argument) so the console
	// does not close before the result can be read. Skipped when stdin is not
	// interactive.
	if noFolder {
		pause()
	}

	if stats.errors > 0 {
		os.Exit(1)
	}
}

// reorderArgs moves options to the front so flag recognizes them even when
// they are written after the folder.
func reorderArgs(args []string) []string {
	options := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			options = append(options, a)
			continue
		}
		positionals = append(positionals, a)
	}
	return append(options, positionals...)
}

// executableFolder returns the folder that contains the running binary, so a
// double click fixes the files sitting next to rename_songs.exe.
func executableFolder() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// cleanName replaces characters that are invalid in file names and
// normalizes whitespace.
func cleanName(text string) string {
	text = strings.TrimSpace(text)
	for original, substitute := range replacements {
		text = strings.ReplaceAll(text, original, substitute)
	}
	text = controlChars.ReplaceAllString(text, "")
	text = spaces.ReplaceAllString(text, " ")
	text = strings.Trim(text, " .")
	return text
}

// normalizeName cleans the text and avoids empty or reserved file names.
func normalizeName(text string) string {
	text = cleanName(text)
	if text == "" {
		return "_"
	}
	base := strings.ToUpper(text)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if reservedNames[base] {
		text = "_" + text
	}
	return text
}

// limitName shortens the name so it does not exceed the Windows limit
// (255 bytes).
func limitName(name string) string {
	const limit = 255
	if len(name) <= limit {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	budget := limit - len(ext)
	for len(base) > budget {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	return strings.Trim(base, " .") + ext
}

type options struct {
	dryRun, recursive, audit, fixTags, forceTags, backup, overwrite bool
	albumFromFolder, genreFromFolder                                bool
}

type stats struct {
	renamed, skipped, replaced, errors int
	tagsWritten, tagsUnsupported       int
	remaining                          map[string]int
	remainingFiles                     int
}

// process walks the folder, repairs metadata and renames the audio files.
func process(folder string, opts options) stats {
	result := stats{remaining: map[string]int{}}

	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		fmt.Printf("Folder does not exist or cannot be read: %s\n", folder)
		result.errors++
		return result
	}

	var files []string
	collect := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("⚠ Cannot access %s: %v\n", path, err)
			result.errors++
			return nil
		}
		if info.IsDir() {
			if !opts.recursive && path != folder {
				return filepath.SkipDir
			}
			return nil
		}
		if validExtensions[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	}

	if err := filepath.Walk(folder, collect); err != nil {
		fmt.Printf("Error walking the folder: %v\n", err)
		result.errors++
		return result
	}

	if len(files) == 0 {
		fmt.Println("No audio files found in the folder.")
		return result
	}

	// Target names already claimed during this run (case-insensitive).
	used := map[string]bool{}

	// Files that still have to be processed. With --overwrite a file can be
	// removed because another file takes its name, and it must not be
	// processed afterwards.
	pending := make(map[string]bool, len(files))
	for _, path := range files {
		pending[strings.ToLower(path)] = true
	}

	for _, path := range files {
		pathKey := strings.ToLower(path)
		if !pending[pathKey] {
			continue
		}
		delete(pending, pathKey)

		name := filepath.Base(path)

		current, err := readTags(path)
		if err != nil {
			fmt.Printf("⚠ Could not read tags in %s: %v\n", name, err)
			result.errors++
			continue
		}

		inferred := parseFileName(path)
		planOpts := planOptions{
			forceTags:       opts.forceTags,
			albumFromFolder: opts.albumFromFolder,
			genreFromFolder: opts.genreFromFolder,
		}

		if opts.audit {
			reportAudit(path, current, inferred, filepath.Dir(path), planOpts, &result)
			continue
		}

		if opts.fixTags {
			updated, changes := planTags(current, inferred, filepath.Dir(path), planOpts)
			switch {
			case len(changes) == 0:
				current = updated
			case opts.dryRun:
				fmt.Printf("[Dry run] tags in %s: %s\n", name, describeChanges(changes))
				current = updated
			default:
				err := writeTags(path, updated, changes, opts.backup)
				switch {
				case errors.Is(err, errUnsupportedFormat):
					// Nothing was written: the format has no writer. The file is
					// still renamed from the repaired metadata, and the summary
					// reports it as untouched.
					result.tagsUnsupported++
					current = updated
				case err != nil:
					fmt.Printf("⚠ Could not write tags in %s: %v\n", name, err)
					result.errors++
					// Keep the tags that really are in the file, so the new name
					// never claims metadata the file does not contain.
				default:
					fmt.Printf("✓ Tags updated in %s: %s\n", name, describeChanges(changes))
					result.tagsWritten++
					current = updated
				}
			}
		}

		artist, title := current.Artist, current.Title
		if artist == "" {
			artist = inferred.Artist
		}
		if title == "" {
			title = inferred.Title
		}
		if artist == "" || title == "" {
			fmt.Printf("✗ Not enough metadata, skipping: %s\n", name)
			result.skipped++
			continue
		}

		for _, field := range current.missingFields() {
			result.remaining[field]++
		}
		result.remainingFiles++

		ext := strings.ToLower(filepath.Ext(path))
		newName := limitName(fmt.Sprintf("%s - %s%s",
			normalizeName(artist), normalizeName(title), ext))
		destination := filepath.Join(filepath.Dir(path), newName)

		if alreadyNamed(path, destination) {
			result.skipped++
			continue
		}

		destination, replaced := freeName(destination, used, opts.overwrite)
		if replaced {
			result.replaced++
			delete(pending, strings.ToLower(destination))
		}

		if opts.dryRun {
			fmt.Printf("[Dry run] %s  ->  %s\n", name, filepath.Base(destination))
			result.renamed++
			continue
		}

		if err := os.Rename(path, destination); err != nil {
			fmt.Printf("⚠ Error renaming %s: %v\n", name, err)
			result.errors++
			continue
		}
		fmt.Printf("✓ Renamed: %s  ->  %s\n", name, filepath.Base(destination))
		result.renamed++
	}

	return result
}

// reportAudit prints the metadata problems of one file and counts them. It
// runs the same repair plan used when writing, so the report shows what would
// be fixed and what would still be missing.
func reportAudit(path string, current tagSet, inferred fileNameInfo, folder string, planOpts planOptions, result *stats) {
	planned, changes := planTags(current, inferred, folder, planOpts)
	missing := planned.missingFields()
	mismatch := planned.mismatches(inferred)
	if len(missing) == 0 && len(mismatch) == 0 && len(changes) == 0 {
		return
	}

	var parts []string
	if len(changes) > 0 {
		parts = append(parts, "would fix "+describeChanges(changes))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing "+strings.Join(missing, ", "))
	}
	parts = append(parts, mismatch...)
	fmt.Printf("%s: %s\n", filepath.Base(path), strings.Join(parts, "; "))

	for _, field := range missing {
		result.remaining[field]++
	}
	result.remainingFiles++
}

// describeChanges renders a list of tag changes for the console.
func describeChanges(changes []change) string {
	var parts []string
	for _, c := range changes {
		parts = append(parts, fmt.Sprintf("%s %q -> %q", c.Field, c.Old, c.New))
	}
	return strings.Join(parts, ", ")
}

// printSummary prints the final counters and the fields that could not be
// filled automatically.
func printSummary(result stats, opts options) {
	fmt.Printf("\nSummary: %d renamed, %d skipped, %d replaced, %d errors\n",
		result.renamed, result.skipped, result.replaced, result.errors)

	if opts.fixTags || opts.audit {
		fmt.Printf("Tags: %d updated", result.tagsWritten)
		if result.tagsUnsupported > 0 {
			fmt.Printf(", %d in formats that cannot be written", result.tagsUnsupported)
		}
		fmt.Println()
	}

	if len(result.remaining) > 0 {
		fields := make([]string, 0, len(result.remaining))
		for field := range result.remaining {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		var parts []string
		for _, field := range fields {
			parts = append(parts, fmt.Sprintf("%s %d", field, result.remaining[field]))
		}
		fmt.Printf("Still missing (of %d files): %s\n", result.remainingFiles, strings.Join(parts, ", "))
	}
}

// parseFileName extracts artist, title and track number from "Artist - Title"
// and from names with a leading track number such as "01. Artist - Title".
func parseFileName(path string) fileNameInfo {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	var result fileNameInfo

	if match := trackPrefix.FindStringSubmatch(base); match != nil {
		rest := strings.TrimSpace(base[len(match[0]):])
		if strings.Contains(rest, " - ") {
			if number, err := strconv.Atoi(match[1]); err == nil && number > 0 {
				result.Track = number
				base = rest
			}
		}
	}

	result.Artist, result.Title = splitArtistTitle(base)
	if result.Title != "" {
		if match := yearSuffix.FindStringSubmatch(result.Title); match != nil {
			if year, err := strconv.Atoi(match[1]); err == nil {
				result.Year = year
			}
		}
	}
	return result
}

// fromFileName tries to split "Artist - Title" from the current file name.
func fromFileName(path string) (string, string) {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return splitArtistTitle(base)
}

// splitArtistTitle splits "Artist - Title" without touching the extension, so
// titles containing dots (for example "Song (feat. Someone)") survive.
func splitArtistTitle(base string) (string, string) {
	if parts := strings.SplitN(base, " - ", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if parts := dashes.Split(base, 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}

// freeName avoids collisions. With overwrite an existing file at the target
// name is replaced (and reported); names claimed earlier in the same run are
// still kept apart, so a file this run just produced is never destroyed.
func freeName(destination string, used map[string]bool, overwrite bool) (string, bool) {
	key := strings.ToLower(destination)
	occupied := exists(destination)
	if !used[key] && (!occupied || overwrite) {
		used[key] = true
		if occupied {
			fmt.Printf("⚠ Replacing existing file: %s\n", filepath.Base(destination))
		}
		return destination, occupied
	}

	ext := filepath.Ext(destination)
	base := strings.TrimSuffix(destination, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		candidateKey := strings.ToLower(candidate)
		if used[candidateKey] || exists(candidate) {
			continue
		}
		used[candidateKey] = true
		fmt.Printf("⚠ Target name already exists, using: %s\n", filepath.Base(candidate))
		return candidate, false
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// alreadyNamed reports whether the file is already named like the destination
// or like a numbered variant of it ("Name (2).ext"), so it is not renamed in a
// loop.
func alreadyNamed(path, destination string) bool {
	if strings.EqualFold(filepath.Clean(path), filepath.Clean(destination)) {
		return true
	}
	if !strings.EqualFold(filepath.Ext(path), filepath.Ext(destination)) {
		return false
	}
	destinationBase := strings.TrimSuffix(filepath.Base(destination), filepath.Ext(destination))
	currentBase := strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if strings.EqualFold(currentBase, destinationBase) {
		return true
	}
	return numberedSuffix.MatchString(currentBase) &&
		strings.EqualFold(numberedSuffixStrip.ReplaceAllString(currentBase, ""), destinationBase)
}

func pause() {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("\nPress Enter to exit...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
