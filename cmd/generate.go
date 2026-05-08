package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/gemini"
	"imagemage/pkg/imagegen"
	"imagemage/pkg/metadata"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	generateCount       int
	generateOutput      string
	generateStyle       string
	generatePreview     bool
	generateAspectRatio string
	generateResolution  string
	generateFrugal      bool
	generateSlide       bool
	generateConfig      string
	generateForce       bool
	generateStorePrompt bool
)

var generateCmd = &cobra.Command{
	Use:   "generate [prompt]",
	Short: "Generate images from text descriptions",
	Long: `Generate one or more images from a text prompt using OpenAI Images or Gemini.

By default, uses OpenAI Images via gpt-image-2. Use --provider=gemini to use Gemini
image models. The --frugal flag is a deprecated low-cost alias; with Gemini it
selects Nano Banana 2 (gemini-3.1-flash-image-preview).

Examples:
  imagemage generate "watercolor painting of a fox in snowy forest"
  imagemage generate "mountain landscape" --count=3 --output=./images
  imagemage generate "cyberpunk city" --style="neon, futuristic"
  imagemage generate "wide cinematic shot" --aspect-ratio="21:9"
  imagemage generate "phone wallpaper" --aspect-ratio="9:16"
  imagemage generate "concept art" --frugal`,
	Args: cobra.MinimumNArgs(1),
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().IntVarP(&generateCount, "count", "c", 1, "Number of images to generate")
	generateCmd.Flags().StringVarP(&generateOutput, "output", "o", ".", "Output directory for generated images")
	generateCmd.Flags().StringVarP(&generateStyle, "style", "s", "", "Additional style guidance (e.g., 'watercolor', 'pixel-art')")
	generateCmd.Flags().BoolVarP(&generatePreview, "preview", "p", false, "Show preview information")
	generateCmd.Flags().StringVarP(&generateAspectRatio, "aspect-ratio", "a", "", "Aspect ratio (1:1, 16:9, 9:16, 4:3, 3:4, 3:2, 2:3, 21:9, 5:4, 4:5)")
	generateCmd.Flags().StringVarP(&generateResolution, "resolution", "r", "", "Image resolution (512px, 1K, 2K, 4K). Defaults to 4K")
	generateCmd.Flags().BoolVarP(&generateFrugal, "frugal", "f", false, "Deprecated alias for low-cost generation; with Gemini selects Nano Banana 2")
	generateCmd.Flags().BoolVar(&generateSlide, "slide", false, "Optimize for presentation slides (4K, 16:9, with theme from config)")
	generateCmd.Flags().StringVar(&generateConfig, "config", "", "Path to config file (JSON) with style, colorScheme, additionalContext")
	generateCmd.Flags().BoolVar(&generateForce, "force", false, "Overwrite existing files without confirmation")
	generateCmd.Flags().BoolVar(&generateStorePrompt, "store-prompt", false, "Store prompt in PNG metadata for reproducibility")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	prompt := args[0]

	// Load config if --slide or --config is specified
	var config *gemini.ImageGenConfig
	var err error
	if generateSlide || generateConfig != "" {
		config, err = gemini.FindConfig(generateConfig)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Apply --slide defaults
	if generateSlide {
		if generateAspectRatio == "" {
			generateAspectRatio = "16:9"
		}
		if generateResolution == "" {
			generateResolution = "4K"
		}
	}

	// Override with config defaults if not specified via flags
	if config != nil {
		if generateAspectRatio == "" && config.GetAspectRatio() != "" {
			generateAspectRatio = config.GetAspectRatio()
		}
		if generateResolution == "" && config.GetResolution() != "" {
			generateResolution = config.GetResolution()
		}
	}

	// Validate aspect ratio if provided
	if generateAspectRatio != "" {
		if err := imagegen.ValidateAspectRatio(generateAspectRatio); err != nil {
			return err
		}
	}

	// Build full prompt with style and config
	fullPrompt := prompt
	if generateStyle != "" {
		fullPrompt = fmt.Sprintf("%s, style: %s", prompt, generateStyle)
	}

	// Apply config theme (style, colors, context)
	if config != nil {
		fullPrompt = config.ApplyToPrompt(fullPrompt)
	}

	client, provider, model, err := newImageClient(generateFrugal)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	// Display generation info
	fmt.Printf("Generating %d image(s) for: %s\n", generateCount, prompt)
	if config != nil {
		fmt.Printf("Config: Loaded (theme applied to prompt)\n")
	}
	if generateStyle != "" {
		fmt.Printf("Style: %s\n", generateStyle)
	}
	if generateAspectRatio != "" {
		fmt.Printf("Aspect Ratio: %s\n", generateAspectRatio)
	}
	// Display resolution info
	resolution := generateResolution
	if resolution == "" {
		resolution = "4K"
	}
	fmt.Printf("Resolution: %s\n", resolution)
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Quality: %s\n", generationRequest("", "", "", nil, generateFrugal).Quality)
	fmt.Println()

	successCount := 0
	ctx := context.Background()
	for i := 1; i <= generateCount; i++ {
		if generateCount > 1 {
			fmt.Printf("[%d/%d] Generating image...\n", i, generateCount)
		} else {
			fmt.Println("Generating image...")
		}

		req := generationRequest(fullPrompt, generateResolution, generateAspectRatio, nil, generateFrugal)
		result, err := client.Generate(ctx, req)
		if err != nil {
			fmt.Printf("Error generating image %d: %v\n", i, err)
			continue
		}

		// Generate filename (prefer AI-suggested name)
		var filename string
		if generateCount > 1 {
			filename = filehandler.GenerateFilename(prompt, result.SuggestedName, "", i)
		} else {
			filename = filehandler.GenerateFilename(prompt, result.SuggestedName, "", 0)
		}
		filename = applyOutputFormatExtension(filename)

		// Create output path
		outputPath := filepath.Join(generateOutput, filename)
		outputPath = filehandler.EnsureUniqueFilename(outputPath)

		// Save image
		if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
			fmt.Printf("Error saving image %d: %v\n", i, err)
			continue
		}

		// Store prompt in metadata if requested
		if generateStorePrompt {
			if !canStorePNGMetadata() {
				fmt.Printf("⚠️  Warning: prompt metadata is only supported for PNG output\n")
			} else if err := metadata.AddPromptToPNG(outputPath, fullPrompt); err != nil {
				fmt.Printf("⚠️  Warning: failed to store prompt in metadata: %v\n", err)
				// Don't fail the whole operation just because metadata write failed
			}
		}

		fmt.Printf("✓ Saved to: %s\n", outputPath)
		if generateStorePrompt {
			fmt.Printf("  (prompt stored in metadata)\n")
		}
		successCount++
	}

	fmt.Printf("\nSuccessfully generated %d/%d images\n", successCount, generateCount)

	return nil
}
