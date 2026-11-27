# RenameSongs

Fixes the metadata of every audio file in a folder and renames it to
`Artist - Title.ext`, so tags and file names agree.

Built for Windows 11 (tested there). It runs inside the console; there is no
installer and no GUI.

## Why

Most music libraries have the information the player shows, but not the
information the files actually store: missing artist, missing track number,
no album artist, empty genre, or a title that contains the whole file name.
RenameSongs reads every field, repairs what it can prove from the file name
and the folder, writes it back, and only then renames the file.

## Usage

Put `rename_songs.exe` in the folder you want to fix and double-click it:
with no argument it works on the folder that contains the executable.

```
rename_songs.exe                          # fix + rename the folder holding the exe
rename_songs.exe "C:\Music\Clasicos"      # one folder
rename_songs.exe "C:\Music" -r            # every subfolder
rename_songs.exe "C:\Music" -r --audit    # report only, changes nothing
rename_songs.exe "C:\Music\Clasicos" -n   # show what would happen
rename_songs.exe "C:\Music" -r --backup   # keep a .bak next to each edited file
```

Options can go before or after the folder.

| Option | Alias | Description |
| --- | --- | --- |
| `--dry-run` | `-n` | Prints planned tag changes and renames, writes nothing |
| `--recursive` | `-r` | Also processes subfolders |
| `--audit` | | Lists what would be fixed, per file, and exits |
| `--no-tags` | | Only renames, leaves tags untouched |
| `--force-tags` | | Tags follow the file name (default `true`; use `=false` to keep existing tags) |
| `--album-from-folder` | | Fills an empty album with the folder name (default `true`) |
| `--genre-from-folder` | | Fills an empty genre with the folder name (default `true`) |
| `--backup` | | Keeps `<file>.bak` before writing tags |
| `--version` | | Prints the version and exits |

## What it does

1. Reads every supported field: artist, title, album, album artist, track,
   year and genre.
2. Cleans damaged values: whitespace, a trailing file extension inside the
   title, and a title that still holds the whole `Artist - Title`.
3. Makes the tags agree with the file name: `artist`, `title` and a leading
   track number (`01. `, `01 - `, `01)`).
4. Fills the remaining gaps without guessing:
   - empty `album artist` from the artist;
   - empty `album` from the folder name;
   - empty `genre` from the folder name;
   - empty `year` from a trailing `(2019)` or `[2019]` in the title.
5. Writes the repaired MP3 tag, then renames the file to
   `Artist - Title.ext`.
6. Prints what changed and which fields are still missing.

Folder names that say nothing (`Downloads`, `Music`, `Playlists`, drive roots)
are ignored, so they never become an album or a genre.

Supported extensions: `.mp3`, `.flac`, `.m4a`, `.mp4`, `.ogg`, `.oga`, `.opus`,
`.wav`, `.aac`, `.wma`, `.aiff`, `.ape`.

Tag **writing** is implemented for MP3 (ID3v2.3 and ID3v2.4). Other formats are
still renamed and audited; when they need tag changes the tool reports it
instead of touching them.

## How tags are written

The tag is edited frame by frame, not decoded and rebuilt. Existing frames
such as cover art are copied byte for byte, so pictures and UTF-16 text survive
the edit. The new file is written to a temporary file, read back to verify it,
and only then replaces the original. With `--backup`, a `.bak` copy is left
behind.

## Safety

- Never overwrites: an existing target gets `(2)`, `(3)`, ...
- Idempotent: correctly named files with complete tags are skipped.
- Avoids Windows reserved names (`CON`, `NUL`, `COM1`, ...).
- Truncates long names to the 255-byte limit.
- Exits with a non-zero status if any error occurred.

## Build

```
go build -trimpath -ldflags "-s -w" -o rename_songs.exe .
```

Requires Go 1.22 or newer.

## Download

The Windows 11 executable is attached to the
[latest release](../../releases/latest).

## License

MIT
