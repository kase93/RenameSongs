# RenameSongs

Repairs audio tags and renames files to `Artist - Title.ext`, so tags and file
names agree. Windows 11, one `.exe`, no installer, no network.

## Use

Drop the executable in a music folder and double-click it: with no argument it
works on the folder that holds the executable.

## What it repairs

- `artist` and `title` from the file name (`Artist - Title`).
- `track` from a leading `01. `, `01 - ` or `01)`.
- `album artist` from the artist, only when empty.
- `album` and `genre` from the folder name, only when empty and the folder name means something.
- `year` from a trailing `(1980)` or `[1980]` in the title, only when empty.
- Damaged text: a file extension inside the title, repeated spaces, a title thatstill holds the whole `Artist - Title`.

Supported extensions: `.mp3`, `.flac`, `.m4a`, `.mp4`, `.ogg`, `.oga`, `.opus`,
`.wav`, `.aac`, `.wma`, `.aiff`, `.ape`.

## Build

Requires Go 1.22 or newer.

```
go test ./...
go build -trimpath -ldflags "-s -w" -o rename_songs.exe .
```

## License

MIT
