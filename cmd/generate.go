package cmd

import (
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/gemini"
	"imagemage/pkg/metadata"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	generatePromptFile  string
)

var generateCmd = &cobra.Command{
	Use:   "generate [prompt]",
	Short: "Generate images from text descriptions",
	Long: `Generate one or more images from a text prompt using Google's Gemini image models.

By default, uses Gemini 3 Pro Image (gemini-3-pro-image-preview) for high-quality 4K generation.
Use --frugal flag to switch to Nano Banana 2 (gemini-3.1-flash-image-preview) for Pro quality at Flash speed.

Examples:
  imagemage generate "watercolor painting of a fox in snowy forest"
  imagemage generate "mountain landscape" --count=3 --output=./images
  imagemage generate "cyberpunk city" --style="neon, futuristic"
  imagemage generate "wide cinematic shot" --aspect-ratio="21:9"
  imagemage generate "phone wallpaper" --aspect-ratio="9:16"
  imagemage generate "concept art" --frugal
  imagemage generate --prompt-file ./prompt.txt`,
	Args: cobra.MaximumNArgs(1),
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
	generateCmd.Flags().BoolVarP(&generateFrugal, "frugal", "f", false, "Use Nano Banana 2 (faster, cheaper, still supports 4K)")
	generateCmd.Flags().BoolVar(&generateSlide, "slide", false, "Optimize for presentation slides (4K, 16:9, with theme from config)")
	generateCmd.Flags().StringVar(&generateConfig, "config", "", "Path to config file (JSON) with style, colorScheme, additionalContext")
	generateCmd.Flags().BoolVar(&generateForce, "force", false, "Overwrite existing files without confirmation")
	generateCmd.Flags().BoolVar(&generateStorePrompt, "store-prompt", false, "Store prompt in PNG metadata for reproducibility")
	generateCmd.Flags().StringVar(&generatePromptFile, "prompt-file", "", "Read prompt from a file (use '-' for stdin)")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	prompt, err := resolvePrompt(args)
	if err != nil {
		return err
	}

	// Load config if --slide or --config is specified
	var config *gemini.ImageGenConfig
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
		if err := gemini.ValidateAspectRatio(generateAspectRatio); err != nil {
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

	// Create Gemini client (frugal or default)
	var client *gemini.Client
	if generateFrugal {
		client, err = gemini.NewFrugalClient()
		if err != nil {
			return fmt.Errorf("failed to create Gemini client: %w", err)
		}
	} else {
		client, err = gemini.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create Gemini client: %w", err)
		}
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
	if generateFrugal {
		fmt.Printf("Model: %s (Nano Banana 2)\n", gemini.ModelNameFrugal)
	} else {
		fmt.Printf("Model: %s\n", gemini.ModelName)
	}
	fmt.Println()

	successCount := 0
	for i := 1; i <= generateCount; i++ {
		if generateCount > 1 {
			fmt.Printf("[%d/%d] Generating image...\n", i, generateCount)
		} else {
			fmt.Println("Generating image...")
		}

		// Generate image with resolution support
		result, err := client.GenerateContentWithResolution(fullPrompt, generateResolution, generateAspectRatio)
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
			if err := metadata.AddPromptToPNG(outputPath, fullPrompt); err != nil {
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

func resolvePrompt(args []string) (string, error) {
	hasArg := len(args) > 0
	hasFile := generatePromptFile != ""

	if hasArg && hasFile {
		return "", fmt.Errorf("provide the prompt either as a positional argument or via --prompt-file, not both")
	}
	if hasArg {
		return args[0], nil
	}
	if !hasFile {
		return "", fmt.Errorf("a prompt is required: pass it as an argument or via --prompt-file")
	}

	var (
		data []byte
		err  error
	)
	if generatePromptFile == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt from stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(generatePromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file: %w", err)
		}
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}
	return prompt, nil
}
