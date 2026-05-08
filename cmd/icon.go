package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/imagegen"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	iconSizes  string
	iconType   string
	iconOutput string
	iconInput  string
)

var iconCmd = &cobra.Command{
	Use:   "icon [description]",
	Short: "Generate app icons, favicons, and UI elements",
	Long: `Generate icons in multiple sizes for apps, websites, and UI elements.
Optionally provide an input image to create an icon version of it.

Examples:
  imagemage icon "coffee cup logo"
  imagemage icon "rocket ship" --sizes="64,128,256" --type="app-icon"
  imagemage icon "hamburger menu" --type="ui-element"
  imagemage icon "make this into a flat icon" -i logo.png
  imagemage icon "simplify for app icon" -i photo.png --type="app-icon"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runIcon,
}

func init() {
	rootCmd.AddCommand(iconCmd)

	iconCmd.Flags().StringVar(&iconSizes, "sizes", "64,128,256", "Comma-separated list of icon sizes")
	iconCmd.Flags().StringVar(&iconType, "type", "app-icon", "Icon type: app-icon, favicon, ui-element")
	iconCmd.Flags().StringVarP(&iconOutput, "output", "o", ".", "Output directory for icons")
	iconCmd.Flags().StringVarP(&iconInput, "input", "i", "", "Input image to convert to icon")
}

func runIcon(cmd *cobra.Command, args []string) error {
	description := args[0]

	// Parse sizes
	sizeStrs := strings.Split(iconSizes, ",")
	sizes := make([]int, 0, len(sizeStrs))
	for _, s := range sizeStrs {
		size, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid size: %s", s)
		}
		sizes = append(sizes, size)
	}

	// Check input image if provided
	var inputImages []imagegen.ImageInput
	if iconInput != "" {
		if _, err := os.Stat(iconInput); os.IsNotExist(err) {
			return fmt.Errorf("input image not found: %s", iconInput)
		}
		inputImage, err := loadImageInput(iconInput)
		if err != nil {
			return fmt.Errorf("failed to load input image: %w", err)
		}
		inputImages = append(inputImages, inputImage)
		fmt.Printf("Input image: %s\n", iconInput)
	}

	// Create enhanced prompt for icon generation
	prompt := fmt.Sprintf("Create a clean, professional %s icon: %s. The icon should be simple, recognizable, and work well at small sizes. Use a square 1:1 aspect ratio. Center the icon on a transparent or solid background.", iconType, description)

	client, provider, model, err := newImageClient(true)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	fmt.Printf("Generating icon: %s\n", description)
	fmt.Printf("Type: %s\n", iconType)
	fmt.Printf("Sizes: %v\n", sizes)
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s (then downscaled)\n", model)
	fmt.Println()

	fmt.Println("Generating base icon...")

	req := generationRequest(prompt, "1K", "1:1", inputImages, true)
	result, err := providerAction(context.Background(), client, req)
	if err != nil {
		return fmt.Errorf("failed to generate icon: %w", err)
	}

	// Resize and save icons at each requested size
	successCount := 0
	for _, size := range sizes {
		filename := filehandler.GenerateFilename(description, result.SuggestedName, fmt.Sprintf("icon_%dx%d", size, size), 0)
		outputPath := filepath.Join(iconOutput, filename)
		outputPath = filehandler.EnsureUniqueFilename(outputPath)

		if err := filehandler.ResizeAndSaveImage(result.ImageData, size, outputPath); err != nil {
			fmt.Printf("Error saving %dx%d icon: %v\n", size, size, err)
			continue
		}

		fmt.Printf("✓ Saved %dx%d icon to: %s\n", size, size, outputPath)
		successCount++
	}

	fmt.Printf("\nSuccessfully generated %d/%d icon sizes\n", successCount, len(sizes))

	return nil
}
