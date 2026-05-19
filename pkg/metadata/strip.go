package metadata

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// StripProvenance removes provenance and identifying metadata (C2PA manifests,
// XMP, EXIF, IPTC, free-form text annotations) from a raw image byte stream by
// walking the container format and dropping non-pixel chunks/segments.
//
// Supports PNG, JPEG, and WebP. Unrecognized formats are returned unchanged.
//
// Note: SynthID and similar invisible watermarks live in the pixel data, not
// metadata, and are not affected by this function.
func StripProvenance(data []byte) ([]byte, error) {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return stripPNG(data)
	case len(data) >= 4 && data[0] == 0xFF && data[1] == 0xD8:
		return stripJPEG(data)
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return stripWebP(data)
	default:
		return data, nil
	}
}

// pngKeepChunk reports whether a PNG chunk type carries pixel/display data
// that must be preserved. Everything else (iTXt, tEXt, zTXt, eXIf, custom
// chunks such as caBX used by C2PA) is dropped.
func pngKeepChunk(t string) bool {
	switch t {
	case "IHDR", "IDAT", "IEND",
		"PLTE", "tRNS",
		"gAMA", "cHRM", "sRGB", "iCCP",
		"bKGD", "pHYs", "sBIT", "sPLT", "hIST", "tIME":
		return true
	}
	return false
}

func stripPNG(data []byte) ([]byte, error) {
	const headerSize = 8
	out := make([]byte, headerSize, len(data))
	copy(out, data[:headerSize])

	pos := headerSize
	for pos < len(data) {
		if pos+8 > len(data) {
			return nil, fmt.Errorf("truncated PNG chunk header at %d", pos)
		}
		length := binary.BigEndian.Uint32(data[pos : pos+4])
		chunkType := string(data[pos+4 : pos+8])
		end := pos + 8 + int(length) + 4
		if end > len(data) || end < pos {
			return nil, fmt.Errorf("invalid PNG chunk length for %s at %d", chunkType, pos)
		}

		if pngKeepChunk(chunkType) {
			out = append(out, data[pos:end]...)
		}

		if chunkType == "IEND" {
			return out, nil
		}
		pos = end
	}
	return nil, fmt.Errorf("PNG missing IEND chunk")
}

func stripJPEG(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)

	pos := 2
	for pos < len(data) {
		if data[pos] != 0xFF {
			return nil, fmt.Errorf("expected JPEG marker at %d, got 0x%02x", pos, data[pos])
		}
		// Skip fill bytes (0xFF padding before a marker code is legal).
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			return nil, fmt.Errorf("truncated JPEG marker")
		}
		marker := data[pos]
		pos++

		switch {
		case marker == 0xD9: // EOI
			out = append(out, 0xFF, 0xD9)
			return out, nil
		case marker == 0xD8, marker == 0x01, marker >= 0xD0 && marker <= 0xD7:
			// Standalone markers with no payload.
			out = append(out, 0xFF, marker)
		case marker == 0xDA: // SOS — copy header and entropy stream verbatim
			if pos+2 > len(data) {
				return nil, fmt.Errorf("truncated SOS length")
			}
			segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			if segLen < 2 || pos+segLen > len(data) {
				return nil, fmt.Errorf("invalid SOS length %d", segLen)
			}
			out = append(out, 0xFF, 0xDA)
			out = append(out, data[pos:pos+segLen]...)
			pos += segLen

			start := pos
			for pos < len(data) {
				if data[pos] != 0xFF {
					pos++
					continue
				}
				if pos+1 >= len(data) {
					return nil, fmt.Errorf("truncated entropy stream")
				}
				next := data[pos+1]
				// 0xFF 0x00 is a stuffed literal 0xFF; 0xFFD0-D7 are restart markers.
				if next == 0x00 || (next >= 0xD0 && next <= 0xD7) {
					pos += 2
					continue
				}
				break
			}
			out = append(out, data[start:pos]...)
		default:
			if pos+2 > len(data) {
				return nil, fmt.Errorf("truncated segment length for marker 0x%02x", marker)
			}
			segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			if segLen < 2 || pos+segLen > len(data) {
				return nil, fmt.Errorf("invalid segment length %d for marker 0x%02x", segLen, marker)
			}
			// Drop all APPn segments (0xE0-0xEF) — EXIF/XMP/JUMBF/C2PA live
			// here — plus COM (0xFE) comments. JFIF in APP0 has no provenance
			// value, so we drop it too; standard decoders cope without it.
			drop := (marker >= 0xE0 && marker <= 0xEF) || marker == 0xFE
			if !drop {
				out = append(out, 0xFF, marker)
				out = append(out, data[pos:pos+segLen]...)
			}
			pos += segLen
		}
	}
	return nil, fmt.Errorf("JPEG missing EOI marker")
}

// webpKeepChunk reports whether a WebP RIFF chunk holds pixel/display data
// that must be preserved.
func webpKeepChunk(t string) bool {
	switch t {
	case "VP8 ", "VP8L", "VP8X", "ALPH", "ANIM", "ANMF", "ICCP":
		return true
	}
	return false
}

func stripWebP(data []byte) ([]byte, error) {
	body := make([]byte, 0, len(data)-12)
	pos := 12
	for pos < len(data) {
		if pos+8 > len(data) {
			return nil, fmt.Errorf("truncated WebP chunk header at %d", pos)
		}
		chunkType := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		dataEnd := pos + 8 + chunkSize
		if dataEnd > len(data) || dataEnd < pos {
			return nil, fmt.Errorf("truncated WebP chunk %q at %d", chunkType, pos)
		}
		// RIFF chunks pad to an even byte boundary; the final chunk may
		// legally omit the pad byte if it would land past EOF.
		end := dataEnd
		if chunkSize%2 == 1 && dataEnd < len(data) {
			end++
		}

		if webpKeepChunk(chunkType) {
			body = append(body, data[pos:end]...)
		}
		pos = end
	}

	// Clear the EXIF (bit 3) and XMP (bit 2) flags in any VP8X header so
	// decoders don't expect chunks we dropped.
	clearVP8XFlags(body)

	out := make([]byte, 0, 12+len(body))
	out = append(out, 'R', 'I', 'F', 'F')
	out = binary.LittleEndian.AppendUint32(out, uint32(4+len(body)))
	out = append(out, 'W', 'E', 'B', 'P')
	out = append(out, body...)
	return out, nil
}

func clearVP8XFlags(body []byte) {
	for i := 0; i+9 <= len(body); {
		chunkSize := int(binary.LittleEndian.Uint32(body[i+4 : i+8]))
		if string(body[i:i+4]) == "VP8X" && chunkSize >= 1 {
			body[i+8] &^= 0x0C
			return
		}
		end := i + 8 + chunkSize
		if chunkSize%2 == 1 {
			end++
		}
		if end > len(body) || end <= i {
			return
		}
		i = end
	}
}
