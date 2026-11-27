// RenameSongs renames audio files to "Artist - Title.ext" using the
// metadata embedded in each file.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dhowden/tag"
)

const (
	appName = "renombrar_canciones"
	version = "1.1.0"
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
	dryRun := flag.Bool("dry-run", false, "show the changes without renaming (alias: -n)")
	dryRunShort := flag.Bool("n", false, "alias of --dry-run")
	recursive := flag.Bool("recursive", false, "also process subfolders (alias: -r)")
	recursiveShort := flag.Bool("r", false, "alias of --recursive")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [folder] [--dry-run] [--recursive]\n", appName)
		fmt.Fprintln(flag.CommandLine.Output(), "Without a folder, uses Downloads.")
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
		folder = downloadsFolder()
	}

	fmt.Printf("Folder: %s\n\n", folder)

	stats := process(folder, *dryRun || *dryRunShort, *recursive || *recursiveShort)

	fmt.Printf("\nSummary: %d renamed, %d skipped, %d errors\n",
		stats.renamed, stats.skipped, stats.errors)

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

// downloadsFolder returns the current user's Downloads folder.
func downloadsFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
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

type stats struct {
	renamed, skipped, errors int
}

// process walks the folder and renames the audio files it finds.
func process(folder string, dryRun, recursive bool) stats {
	var result stats

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
			if !recursive && path != folder {
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

	for _, path := range files {
		artist, title, err := extract(path)
		if err != nil {
			fmt.Printf("⚠ Could not read %s: %v\n", filepath.Base(path), err)
			result.errors++
			continue
		}
		if artist == "" || title == "" {
			fmt.Printf("✗ Not enough metadata, skipping: %s\n", filepath.Base(path))
			result.skipped++
			continue
		}

		ext := strings.ToLower(filepath.Ext(path))
		name := limitName(fmt.Sprintf("%s - %s%s",
			normalizeName(artist), normalizeName(title), ext))
		destination := filepath.Join(filepath.Dir(path), name)

		if alreadyNamed(path, destination) {
			result.skipped++
			continue
		}

		destination = freeName(destination, used)

		if dryRun {
			fmt.Printf("[Dry run] %s  ->  %s\n", filepath.Base(path), filepath.Base(destination))
			result.renamed++
			continue
		}

		if err := os.Rename(path, destination); err != nil {
			fmt.Printf("⚠ Error renaming %s: %v\n", filepath.Base(path), err)
			result.errors++
			continue
		}
		fmt.Printf("✓ Renamed: %s  ->  %s\n", filepath.Base(path), filepath.Base(destination))
		result.renamed++
	}

	return result
}

// extract returns artist and title from the tags; when they are missing it
// falls back to parsing the current file name ("Artist - Title.ext").
func extract(path string) (string, string, error) {
	var artist, title string

	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	if meta, err := tag.ReadFrom(f); err == nil {
		artist = strings.TrimSpace(meta.Artist())
		title = strings.TrimSpace(meta.Title())
	} else if err != tag.ErrNoTagsFound {
		// Not fatal: we still try the file name.
		fmt.Printf("⚠ No readable tags in %s: %v\n", filepath.Base(path), err)
	}

	if artist == "" || title == "" {
		if a, t := fromFileName(path); a != "" && t != "" {
			if artist == "" {
				artist = a
			}
			if title == "" {
				title = t
			}
		}
	}

	return artist, title, nil
}

// fromFileName tries to split "Artist - Title" from the current file name.
func fromFileName(path string) (string, string) {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

	if parts := strings.SplitN(base, " - ", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if parts := dashes.Split(base, 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}

// freeName avoids overwriting files and collisions within the same run.
func freeName(destination string, used map[string]bool) string {
	key := strings.ToLower(destination)
	if !used[key] && !exists(destination) {
		used[key] = true
		return destination
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
		return candidate
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
