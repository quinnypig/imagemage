package filehandler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// SaveImage saves base64 encoded image data to a file
func SaveImage(imageData, outputPath string) error {
	// Decode base64 image data
	decoded, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return fmt.Errorf("failed to decode image data: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write image to file
	if err := os.WriteFile(outputPath, decoded, 0644); err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	return nil
}

// GenerateFilename creates a descriptive filename from a prompt or suggested name
func GenerateFilename(prompt, suggestedName, prefix string, count int) string {
	// Try to use AI-suggested name first
	cleaned := validateSuggestedName(suggestedName)

	// Fall back to prompt-based name
	if cleaned == "" {
		cleaned = cleanPrompt(prompt)
		// Truncate if too long
		maxLen := 50
		if len(cleaned) > maxLen {
			cleaned = cleaned[:maxLen]
		}
	}

	// Build filename
	var filename string
	if prefix != "" {
		filename = fmt.Sprintf("%s_%s", prefix, cleaned)
	} else {
		filename = cleaned
	}

	// Add counter if specified
	if count > 0 {
		filename = fmt.Sprintf("%s_%d", filename, count)
	}

	return filename + ".png"
}

// cleanPrompt converts a prompt into a filename-safe string
func cleanPrompt(prompt string) string {
	// Convert to lowercase
	s := strings.ToLower(prompt)

	// Remove special characters and replace spaces with underscores
	reg := regexp.MustCompile(`[^a-z0-9\s-]`)
	s = reg.ReplaceAllString(s, "")

	// Replace multiple spaces with single underscore
	reg = regexp.MustCompile(`\s+`)
	s = reg.ReplaceAllString(s, "_")

	return SanitizeFilenameStem(s)
}

// SanitizeFilenameStem collapses runs of separator chars ('-' and '_') to a
// single separator and trims them from the edges. A pure run of hyphens stays
// a single '-' (so "hello-world" survives), but anything containing '_' (or
// mixing both) becomes '_'. This prevents filenames like "foo---bar.png" that
// can break YAML referencing or look like document separators.
func SanitizeFilenameStem(s string) string {
	reg := regexp.MustCompile(`[-_]+`)
	s = reg.ReplaceAllStringFunc(s, func(m string) string {
		if strings.ContainsRune(m, '_') {
			return "_"
		}
		return "-"
	})
	return strings.Trim(s, "-_")
}

// validateSuggestedName checks if a suggested name is usable
func validateSuggestedName(name string) string {
	// Clean the name using same rules as prompts
	cleaned := cleanPrompt(name)

	// Must have at least one letter
	hasLetter := false
	for _, c := range cleaned {
		if c >= 'a' && c <= 'z' {
			hasLetter = true
			break
		}
	}

	// Valid if 1-50 chars and has a letter
	if len(cleaned) >= 1 && len(cleaned) <= 50 && hasLetter {
		return cleaned
	}

	return ""
}

// EnsureUniqueFilename checks if a file exists and adds a counter if needed
func EnsureUniqueFilename(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	// File exists, add counter
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	counter := 1
	for {
		newPath := fmt.Sprintf("%s_%d%s", base, counter, ext)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		counter++
	}
}

// LoadImageAsBase64 loads an image file and returns it as base64
func LoadImageAsBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// ResizeAndSaveImage decodes base64 image data, resizes it to the target size, and saves to outputPath
func ResizeAndSaveImage(imageData string, size int, outputPath string) error {
	// Decode base64 image data
	decoded, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return fmt.Errorf("failed to decode image data: %w", err)
	}

	// Decode image
	src, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Create destination image with target size (square for icons)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	// Use high-quality CatmullRom interpolation for resizing
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	// Encode as PNG
	if err := png.Encode(f, dst); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return f.Close()
}

// GetImageDimensions returns the width and height of an image file
func GetImageDimensions(path string) (width, height int, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode image config: %w", err)
	}

	return config.Width, config.Height, nil
}
