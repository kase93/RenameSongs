# RenameSongs

Reads the metadata of audio files, repairs MP3 tags, and renames every file to `Artist - Title`.

## Use

Drop `rename_songs.exe` in a music folder and double-click it: it works on the folder that holds the executable.

Tag repair only works on `.mp3`. Every other format is renamed, and its tags
are read but not written; the summary counts those as "cannot be written".

## What it repairs

- `artist` and `title` from the file name (`Artist - Title`).
- `track` from a leading `01. `, `01 - ` or `01)`.
- `album artist` from the artist, only when empty.
- `album` and `genre` from the folder name, only when empty and the folder name
  means something.
- `year` from a trailing `(1980)` or `[1980]` in the title, only when empty.
- Damaged text: a file extension inside the title, repeated spaces, a title that
  still holds the whole `Artist - Title`.

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
