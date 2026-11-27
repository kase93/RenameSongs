# RenameSongs

Renames audio files to `Artist - Title.ext` using the metadata embedded in
each file. Works on Windows (tested on Windows 10/11), macOS and Linux.

## Usage

```
renombrar_canciones.exe                       # renames the Downloads folder
renombrar_canciones.exe "C:\Music\EDM"        # renames that folder
renombrar_canciones.exe "C:\Music\EDM" -n     # dry run, touches nothing
renombrar_canciones.exe "C:\Music" -r         # includes subfolders
```

Options can be written before or after the path.

| Option | Alias | Description |
| --- | --- | --- |
| `--dry-run` | `-n` | Shows the changes without renaming anything |
| `--recursive` | `-r` | Also processes subfolders |
| `--version` | | Prints the version and exits |

## What it does

1. Reads the file tags with [dhowden/tag](https://github.com/dhowden/tag).
2. If artist or title are missing, tries to infer them from the current name
   (`Artist - Title.ext`).
3. Cleans characters that are invalid on Windows (`/`, `:`, `*`, `?`, `"`,
   `<`, `>`, `|`).
4. Renames the file and prints a summary at the end.

Supported extensions: `.mp3`, `.flac`, `.m4a`, `.mp4`, `.ogg`, `.oga`, `.opus`,
`.wav`, `.aac`, `.wma`, `.aiff`, `.ape`.

## Safety

- Never overwrites: if the target name already exists it appends a number
  (`(2)`, `(3)`, ...).
- Idempotent: a file that is already correctly named is skipped on later runs.
- Avoids Windows reserved names (`CON`, `NUL`, `COM1`, ...).
- Truncates long names so they do not exceed the 255-byte limit.
- Exits with a non-zero status if any error occurred.

## Build

```
go build -trimpath -ldflags "-s -w" -o renombrar_canciones.exe .
```

Requires Go 1.22 or newer.

## Download

The Windows executable is available in the
[Releases](../../releases/latest) section.

## License

MIT
