package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imagemage/pkg/gemini"
	"imagemage/pkg/imagegen"
	"imagemage/pkg/openai"
)

var (
	cliProvider string
	cliModel    string
	cliQuality  string
	cliFormat   string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cliProvider, "provider", "", "Image provider: openai or gemini (default: IMAGEMAGE_PROVIDER or openai)")
	rootCmd.PersistentFlags().StringVar(&cliModel, "model", "", "Provider model override")
	rootCmd.PersistentFlags().StringVar(&cliQuality, "quality", "auto", "Generation quality: low, medium, high, auto")
	rootCmd.PersistentFlags().StringVar(&cliFormat, "format", "png", "Output format for providers that support it: png, jpeg, webp")
}

func newImageClient(frugal bool) (imagegen.Client, imagegen.Provider, string, error) {
	provider := imagegen.Provider(strings.ToLower(cliProvider))
	if provider == "" {
		provider = imagegen.Provider(strings.ToLower(os.Getenv("IMAGEMAGE_PROVIDER")))
	}
	if provider == "" {
		provider = imagegen.ProviderOpenAI
	}

	model := cliModel
	switch provider {
	case imagegen.ProviderOpenAI:
		if model == "" {
			model = openai.ModelName
		}
		client, err := openai.NewClient(model)
		return client, provider, model, err
	case imagegen.ProviderGemini:
		if model == "" && frugal {
			model = gemini.ModelNameFrugal
		}
		if model == "" {
			model = gemini.ModelName
		}
		client, err := gemini.NewClientWithModel(model)
		return client, provider, model, err
	default:
		return nil, "", "", fmt.Errorf("unsupported provider %q (supported: openai, gemini)", provider)
	}
}

func generationRequest(prompt, resolution, aspectRatio string, images []imagegen.ImageInput, frugal bool) imagegen.Request {
	quality := cliQuality
	if frugal && quality == "auto" {
		quality = "low"
	}
	return imagegen.Request{
		Prompt:       prompt,
		Images:       images,
		AspectRatio:  aspectRatio,
		Resolution:   resolution,
		Quality:      quality,
		OutputFormat: cliFormat,
	}
}

func loadImageInput(path string) (imagegen.ImageInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return imagegen.ImageInput{}, fmt.Errorf("failed to read image: %w", err)
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return imagegen.ImageInput{}, fmt.Errorf("input file is not a recognized image: %s", path)
	}
	return imagegen.ImageInput{
		MimeType: mimeType,
		Base64:   base64.StdEncoding.EncodeToString(data),
	}, nil
}

func providerAction(ctx context.Context, client imagegen.Client, req imagegen.Request) (imagegen.Result, error) {
	if len(req.Images) > 0 {
		return client.Edit(ctx, req)
	}
	return client.Generate(ctx, req)
}

func applyOutputFormatExtension(path string) string {
	ext := imageFileExtension()
	if ext == ".png" {
		return path
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext
}

func imageFileExtension() string {
	switch cliFormat {
	case "jpeg":
		return ".jpg"
	case "webp":
		return ".webp"
	default:
		return ".png"
	}
}

func canStorePNGMetadata() bool {
	return imageFileExtension() == ".png"
}
