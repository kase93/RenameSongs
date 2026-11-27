# RenameSongs

Renames audio files to `Artist - Title.ext` and repairs the metadata embedded
in each file, so the tags are right even if you never look at them.

Built for Windows 11 (tested there). It runs inside the console; there is no
installer and no GUI.

## Why

Most music libraries have the information the player shows, but not the
information the files actually store: missing artist, missing track number,
no album artist, empty genre. RenameSongs reads every field first, fills what
it can prove from the file name and folder, then renames the file.

## Usage

```
rename_songs.exe                                  # rename + repair the Downloads folder
rename_songs.exe "C:\Music\Clasicos"              # one folder
rename_songs.exe "C:\Music" -r                    # every subfolder
rename_songs.exe "C:\Music" -r --audit            # report only, changes nothing
rename_songs.exe "C:\Music\Clasicos" -n           # show what would happen
rename_songs.exe "C:\Music" -r --backup           # keep a .bak next to each edited file
```

Options can go before or after the folder.

| Option | Alias | Description |
| --- | --- | --- |
| `--dry-run` | `-n` | Prints planned tag changes and renames, writes nothing |
| `--recursive` | `-r` | Also processes subfolders |
| `--audit` | | Lists missing or mismatched metadata per file and exits |
| `--no-tags` | | Only renames, leaves tags untouched |
| `--force-tags` | | Overwrites tags that differ from the file name |
| `--backup` | | Keeps `<file>.bak` before writing tags |
| `--genre-from-folder` | | Fills an empty genre with the folder name |
| `--version` | | Prints the version and exits |

## What it does

1. Reads every supported field: artist, title, album, album artist, track,
   year and genre.
2. Fills what can be inferred without guessing:
   - empty `artist` and `title` from the file name (`Artist - Title.ext`);
   - leading track numbers such as `01. `, `01 - ` or `01)`;
   - empty `album artist` from the artist;
   - empty `genre` from the folder name, only with `--genre-from-folder`.
3. With `--force-tags`, replaces artist/title/track when the tag disagrees
   with the file name.
4. Writes the repaired MP3 tag, then renames the file.
5. Prints what changed and which fields are still missing.

It never invents album or year: those stay empty when the file does not have
them. Run `--audit` to see how many files still lack them.

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
