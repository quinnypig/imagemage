package metadata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	filepathpkg "path/filepath"
)

const maxTextChunkSize = 1 << 20

// AddPromptToPNG adds a prompt as a tEXt chunk to a PNG file
// If the file is JPEG, it will be converted to PNG first
func AddPromptToPNG(filepath string, prompt string) error {
	// Read the entire file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read image file: %w", err)
	}

	// Check if it's a PNG file (first 8 bytes are PNG signature)
	isPNG := len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})

	// If it's not PNG, try to convert from JPEG
	if !isPNG {
		// Check if it's JPEG
		isJPEG := len(data) >= 2 && bytes.Equal(data[:2], []byte{0xFF, 0xD8})
		if !isJPEG {
			return fmt.Errorf("file is neither PNG nor JPEG format")
		}

		// Convert JPEG to PNG
		if err := convertJPEGToPNG(filepath); err != nil {
			return fmt.Errorf("failed to convert JPEG to PNG: %w", err)
		}

		// Re-read the now-PNG file
		data, err = os.ReadFile(filepath)
		if err != nil {
			return fmt.Errorf("failed to read converted PNG file: %w", err)
		}
	}

	// Create tEXt chunk with prompt
	textChunk := createTextChunk("Prompt", prompt)

	// Find the position to insert (before IEND chunk)
	// IEND is the last chunk and is always 12 bytes: 4(length) + 4(type) + 0(data) + 4(CRC)
	if len(data) < 12 {
		return fmt.Errorf("PNG file too short")
	}

	// Insert the tEXt chunk before IEND
	insertPos := len(data) - 12
	newData := make([]byte, 0, len(data)+len(textChunk))
	newData = append(newData, data[:insertPos]...)
	newData = append(newData, textChunk...)
	newData = append(newData, data[insertPos:]...)

	// Write the modified PNG
	if err := os.WriteFile(filepath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write PNG file: %w", err)
	}

	return nil
}

// createTextChunk creates a PNG tEXt chunk
// Format: Length (4 bytes) + Type "tEXt" (4 bytes) + Data (keyword\0text) + CRC (4 bytes)
func createTextChunk(keyword, text string) []byte {
	// Build chunk data: keyword + null byte + text
	chunkData := []byte(keyword)
	chunkData = append(chunkData, 0) // null separator
	chunkData = append(chunkData, []byte(text)...)

	// Create chunk
	buf := new(bytes.Buffer)

	// Length (4 bytes, big-endian)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(chunkData)))

	// Type (4 bytes: "tEXt")
	buf.WriteString("tEXt")

	// Data
	buf.Write(chunkData)

	// CRC (4 bytes) - calculated over type + data
	crcData := append([]byte("tEXt"), chunkData...)
	crc := crc32.ChecksumIEEE(crcData)
	_ = binary.Write(buf, binary.BigEndian, crc)

	return buf.Bytes()
}

// ReadPromptFromPNG reads the prompt from a PNG file's tEXt chunks
func ReadPromptFromPNG(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to open PNG file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Verify PNG signature
	sig := make([]byte, 8)
	if _, err := io.ReadFull(file, sig); err != nil {
		return "", fmt.Errorf("failed to read PNG signature: %w", err)
	}
	if !bytes.Equal(sig, []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return "", fmt.Errorf("not a valid PNG file")
	}

	// Read chunks until we find tEXt with "Prompt" keyword
	for {
		// Read chunk length
		var length uint32
		if err := binary.Read(file, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("no Prompt metadata found in PNG")
			}
			return "", err
		}

		// Read chunk type
		chunkType := make([]byte, 4)
		if _, err := io.ReadFull(file, chunkType); err != nil {
			return "", err
		}

		// Check if this is a tEXt chunk with "Prompt" keyword
		if string(chunkType) == "tEXt" {
			if length > maxTextChunkSize {
				return "", fmt.Errorf("PNG tEXt chunk too large")
			}
			chunkData := make([]byte, length)
			if _, err := io.ReadFull(file, chunkData); err != nil {
				return "", err
			}
			if _, err := file.Seek(4, io.SeekCurrent); err != nil {
				return "", err
			}

			// Find null separator
			nullPos := bytes.IndexByte(chunkData, 0)
			if nullPos > 0 {
				keyword := string(chunkData[:nullPos])
				if keyword == "Prompt" {
					text := string(chunkData[nullPos+1:])
					return text, nil
				}
			}
		} else {
			if _, err := file.Seek(int64(length)+4, io.SeekCurrent); err != nil {
				return "", err
			}
		}

		// IEND chunk means we've reached the end
		if string(chunkType) == "IEND" {
			return "", fmt.Errorf("no Prompt metadata found in PNG")
		}
	}
}

// convertJPEGToPNG converts a JPEG file to PNG format in-place
func convertJPEGToPNG(filepath string) error {
	// Open and decode JPEG
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}

	img, err := jpeg.Decode(file)
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	if err != nil {
		return fmt.Errorf("failed to decode JPEG: %w", err)
	}

	dir := filepathpkg.Dir(filepath)
	tmpFile, err := os.CreateTemp(dir, ".imagemage-*.png")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := png.Encode(tmpFile, img); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath)
}
