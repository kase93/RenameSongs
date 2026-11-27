package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// This file edits ID3v2 tags frame by frame instead of re-encoding the whole
// tag, so existing frames (cover art in particular) are copied byte for byte.
// Libraries that parse and rewrite every frame can corrupt UTF-16 text and
// pictures found in real-world files.

var (
	errID3Unsupported = errors.New("this ID3 layout cannot be edited")
	errID3Malformed   = errors.New("malformed ID3 tag")
)

// rawFrame keeps a frame exactly as it was stored. The two status/format
// bytes are preserved so frames that use unsynchronisation, a data length
// indicator or an encryption scheme survive the rewrite untouched.
type rawFrame struct {
	id    string
	flags [2]byte
	body  []byte
}

// frameIDForField maps a logical field to its ID3v2.3 frame identifier.
func frameIDForField(field string, version byte) string {
	switch field {
	case "artist":
		return "TPE1"
	case "title":
		return "TIT2"
	case "album":
		return "TALB"
	case "album artist":
		return "TPE2"
	case "genre":
		return "TCON"
	case "track":
		return "TRCK"
	case "year":
		if version >= 4 {
			return "TDRC"
		}
		return "TYER"
	}
	return ""
}

// valueForField renders the new frame text for a logical field.
func valueForField(field string, updated tagSet) string {
	switch field {
	case "artist":
		return updated.Artist
	case "title":
		return updated.Title
	case "album":
		return updated.Album
	case "album artist":
		return updated.AlbumArtist
	case "genre":
		return updated.Genre
	case "track":
		if updated.TrackTotal > 0 {
			return fmt.Sprintf("%d/%d", updated.Track, updated.TrackTotal)
		}
		return strconv.Itoa(updated.Track)
	case "year":
		return strconv.Itoa(updated.Year)
	}
	return ""
}

// textFrameBody encodes text using the encoding that version supports.
func textFrameBody(version byte, text string) []byte {
	if version >= 4 {
		body := []byte{3} // UTF-8
		body = append(body, []byte(text)...)
		return append(body, 0)
	}
	// ID3v2.3: UTF-16 with byte order mark.
	body := []byte{1, 0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune(text)) {
		body = binary.LittleEndian.AppendUint16(body, unit)
	}
	return append(body, 0, 0)
}

func synchsafeToInt(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

func intToSynchsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
}

func isFrameID(b []byte) bool {
	for _, c := range b {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// splitID3 separates the tag from the audio payload.
func splitID3(data []byte) (byte, []rawFrame, []byte, error) {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return 3, nil, data, nil
	}

	version := data[3]
	if version < 3 || version > 4 {
		return version, nil, nil, errID3Unsupported
	}
	if data[4] != 0 || data[5]&0x80 != 0 {
		// Unknown revision or unsynchronised tag.
		return version, nil, nil, errID3Unsupported
	}

	size := synchsafeToInt(data[6:10])
	bodyStart, bodyEnd := 10, 10+size
	if bodyEnd > len(data) {
		return version, nil, nil, errID3Malformed
	}
	body := data[bodyStart:bodyEnd]

	// An ID3v2.4 footer sits after the declared tag size and belongs to the
	// tag, not to the audio. Ignoring it would truncate the first 10 audio
	// bytes on every rewrite.
	audioStart := bodyEnd
	if data[5]&0x10 != 0 {
		audioStart += 10
		if audioStart > len(data) {
			return version, nil, nil, errID3Malformed
		}
	}
	audio := data[audioStart:]

	if data[5]&0x40 != 0 {
		// Skip the extended header.
		if len(body) < 4 {
			return version, nil, nil, errID3Malformed
		}
		var extSize int
		if version == 4 {
			extSize = synchsafeToInt(body[:4])
		} else {
			extSize = int(binary.BigEndian.Uint32(body[:4])) + 4
		}
		if extSize < 0 || extSize > len(body) {
			return version, nil, nil, errID3Malformed
		}
		body = body[extSize:]
	}

	var frames []rawFrame
	for len(body) >= 10 {
		id := string(body[:4])
		if !isFrameID(body[:4]) {
			break // padding
		}
		var frameSize int
		if version == 4 {
			frameSize = synchsafeToInt(body[4:8])
		} else {
			frameSize = int(binary.BigEndian.Uint32(body[4:8]))
		}
		if frameSize < 0 || 10+frameSize > len(body) {
			return version, nil, nil, errID3Malformed
		}
		frames = append(frames, rawFrame{
			id:    id,
			flags: [2]byte{body[8], body[9]},
			body:  body[10 : 10+frameSize],
		})
		body = body[10+frameSize:]
	}

	return version, frames, audio, nil
}

// buildID3 serializes a tag with 1 KiB of padding so later edits can grow it.
func buildID3(version byte, frames []rawFrame, audio []byte) []byte {
	payload := bytes.Buffer{}
	for _, f := range frames {
		payload.WriteString(f.id)
		if version == 4 {
			payload.Write(intToSynchsafe(len(f.body)))
		} else {
			var size [4]byte
			binary.BigEndian.PutUint32(size[:], uint32(len(f.body)))
			payload.Write(size[:])
		}
		payload.Write(f.flags[:])
		payload.Write(f.body)
	}
	payload.Write(make([]byte, 1024))

	var out bytes.Buffer
	out.WriteString("ID3")
	out.Write([]byte{version, 0, 0})
	out.Write(intToSynchsafe(payload.Len()))
	out.Write(payload.Bytes())
	out.Write(audio)
	return out.Bytes()
}

// updateID3 replaces (or appends) the frames for the given changes.
func updateID3(version byte, frames []rawFrame, changes []change, updated tagSet) []rawFrame {
	for _, c := range changes {
		id := frameIDForField(c.Field, version)
		if id == "" {
			continue
		}
		kept := frames[:0]
		for _, f := range frames {
			if f.id != id {
				kept = append(kept, f)
			}
		}
		frames = append(kept, rawFrame{id: id, body: textFrameBody(version, valueForField(c.Field, updated))})
	}
	return frames
}

// decodeTextFrame extracts the text of a text frame, honoring the encoding
// byte. It is used to read back what was just written.
func decodeTextFrame(body []byte) (string, bool) {
	if len(body) < 2 {
		return "", false
	}
	text := body[1:]
	switch body[0] {
	case 0: // ISO-8859-1
		runes := make([]rune, 0, len(text))
		for _, b := range text {
			runes = append(runes, rune(b))
		}
		return strings.TrimRight(string(runes), "\x00"), true
	case 3: // UTF-8
		return strings.TrimRight(string(text), "\x00"), true
	case 1, 2: // UTF-16 with or without byte order mark
		bigEndian := false
		if len(text) >= 2 {
			switch {
			case text[0] == 0xFF && text[1] == 0xFE:
				text = text[2:]
			case text[0] == 0xFE && text[1] == 0xFF:
				text = text[2:]
				bigEndian = true
			}
		}
		units := make([]uint16, 0, len(text)/2)
		for i := 0; i+1 < len(text); i += 2 {
			if bigEndian {
				units = append(units, binary.BigEndian.Uint16(text[i:i+2]))
			} else {
				units = append(units, binary.LittleEndian.Uint16(text[i:i+2]))
			}
		}
		for len(units) > 0 && units[len(units)-1] == 0 {
			units = units[:len(units)-1]
		}
		return string(utf16.Decode(units)), true
	}
	return "", false
}

// verifyID3File reads the written file back and confirms that every changed
// field is stored with the expected value and that the audio payload is still
// there. A write that failed silently or damaged the tag is caught before the
// original file is replaced.
func verifyID3File(path string, changes []change, updated tagSet, audioLen int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	version, frames, audio, err := splitID3(data)
	if err != nil {
		return err
	}
	if len(audio) != audioLen {
		return fmt.Errorf("audio payload changed: %d bytes, want %d", len(audio), audioLen)
	}

	bodies := make(map[string][]byte, len(frames))
	for _, f := range frames {
		if _, seen := bodies[f.id]; !seen {
			bodies[f.id] = f.body
		}
	}

	for _, c := range changes {
		id := frameIDForField(c.Field, version)
		if id == "" {
			continue
		}
		body, ok := bodies[id]
		if !ok {
			return fmt.Errorf("frame %s for %s is missing after writing", id, c.Field)
		}
		got, ok := decodeTextFrame(body)
		if !ok {
			return fmt.Errorf("frame %s for %s is not readable text", id, c.Field)
		}
		if want := valueForField(c.Field, updated); got != want {
			return fmt.Errorf("frame %s for %s = %q, want %q", id, c.Field, got, want)
		}
	}
	return nil
}

// writeID3File applies the changes to path through a temporary file.
func writeID3File(path string, changes []change, updated tagSet, backup bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	version, frames, audio, err := splitID3(data)
	if err != nil {
		return err
	}

	frames = updateID3(version, frames, changes, updated)
	output := buildID3(version, frames, audio)

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp := path + ".rename_songs.tmp"
	if err := os.WriteFile(tmp, output, mode); err != nil {
		return err
	}

	if err := verifyID3File(tmp, changes, updated, len(audio)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("verification failed: %w", err)
	}

	if backup {
		if err := copyFile(path, path+".bak"); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, path)
}
