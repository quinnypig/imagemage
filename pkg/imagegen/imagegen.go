package imagegen

import (
	"context"
	"fmt"
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderGemini Provider = "gemini"
)

type ImageInput struct {
	MimeType string
	Base64   string
}

type Request struct {
	Prompt       string
	Images       []ImageInput
	AspectRatio  string
	Resolution   string
	Quality      string
	OutputFormat string
	// Background controls transparency: "transparent", "opaque", or "auto".
	// Transparent output requires a format that supports it (png or webp).
	Background string
	// InputFidelity asks the model to preserve detail from input images
	// ("high" or "low"). Only meaningful when Images are supplied.
	InputFidelity string
	// Moderation selects the content-moderation strictness: "low" or "auto".
	Moderation string
	// Compression is the output compression (0-100) for lossy formats
	// (jpeg, webp). Zero means "leave unset / provider default".
	Compression int
	// Count is the number of images to request in a single batched call.
	// Zero or one means a single image. Only honored by BatchGenerator.
	Count int
}

type Result struct {
	ImageData     string
	SuggestedName string
	Provider      Provider
	Model         string
}

type Client interface {
	Generate(ctx context.Context, req Request) (Result, error)
	Edit(ctx context.Context, req Request) (Result, error)
}

// BatchGenerator is an optional capability for providers that can return
// multiple images from a single API call (e.g. OpenAI's `n` parameter).
// Callers should type-assert against it and fall back to repeated Generate/Edit
// calls for providers that do not implement it (such as Gemini, which returns
// one image per request). It routes to the reference-capable edit path when
// req.Images is non-empty, mirroring providerAction.
type BatchGenerator interface {
	GenerateBatch(ctx context.Context, req Request) ([]Result, error)
}

var SupportedAspectRatios = []string{
	"1:1",
	"16:9",
	"9:16",
	"4:3",
	"3:4",
	"3:2",
	"2:3",
	"21:9",
	"5:4",
	"4:5",
	"4:1",
	"1:4",
	"8:1",
	"1:8",
}

func ValidateAspectRatio(aspectRatio string) error {
	if aspectRatio == "" {
		return nil
	}

	for _, supported := range SupportedAspectRatios {
		if aspectRatio == supported {
			return nil
		}
	}

	return fmt.Errorf("unsupported aspect ratio: %s. Supported: %v", aspectRatio, SupportedAspectRatios)
}

func FindClosestAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return "1:1"
	}

	inputRatio := float64(width) / float64(height)
	bestMatch := "1:1"
	smallestDiff := float64(1000)

	for _, ar := range SupportedAspectRatios {
		var w, h int
		if _, err := fmt.Sscanf(ar, "%d:%d", &w, &h); err != nil || h == 0 {
			continue
		}

		arRatio := float64(w) / float64(h)
		diff := inputRatio - arRatio
		if diff < 0 {
			diff = -diff
		}

		if diff < smallestDiff {
			smallestDiff = diff
			bestMatch = ar
		}
	}

	return bestMatch
}

func OpenAISize(resolution, aspectRatio string) (string, error) {
	if resolution == "" && aspectRatio == "" {
		return "auto", nil
	}

	if aspectRatio == "" {
		aspectRatio = "1:1"
	}
	if err := ValidateAspectRatio(aspectRatio); err != nil {
		return "", err
	}

	switch resolution {
	case "", "4K":
		return openAISizeFor(aspectRatio, map[string]string{
			"1:1":  "2048x2048",
			"16:9": "3840x2160",
			"9:16": "2160x3840",
			"4:3":  "3072x2304",
			"3:4":  "2304x3072",
			"3:2":  "3072x2048",
			"2:3":  "2048x3072",
			"21:9": "3360x1440",
			"5:4":  "2560x2048",
			"4:5":  "2048x2560",
		})
	case "2K":
		return openAISizeFor(aspectRatio, map[string]string{
			"1:1":  "2048x2048",
			"16:9": "2048x1152",
			"9:16": "1152x2048",
			"4:3":  "2048x1536",
			"3:4":  "1536x2048",
			"3:2":  "2016x1344",
			"2:3":  "1344x2016",
			"21:9": "2016x864",
			"5:4":  "1920x1536",
			"4:5":  "1536x1920",
		})
	case "1K", "512px":
		return openAISizeFor(aspectRatio, map[string]string{
			"1:1":  "1024x1024",
			"16:9": "1536x864",
			"9:16": "864x1536",
			"4:3":  "1536x1152",
			"3:4":  "1152x1536",
			"3:2":  "1536x1024",
			"2:3":  "1024x1536",
			"21:9": "1536x672",
			"5:4":  "1280x1024",
			"4:5":  "1024x1280",
		})
	default:
		return resolution, nil
	}
}

func openAISizeFor(aspectRatio string, sizes map[string]string) (string, error) {
	size, ok := sizes[aspectRatio]
	if !ok {
		return "", fmt.Errorf("aspect ratio %s is not supported by the OpenAI size mapper", aspectRatio)
	}
	return size, nil
}
