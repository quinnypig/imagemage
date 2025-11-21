package cmd

import (
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/gemini"
	"imagemage/pkg/metadata"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	editOutput      string
	editInputs      []string
	editAspectRatio string
	editResolution  string
	editFrugal      bool
	editForce       bool
	editStorePrompt bool
)

var editCmd = &cobra.Command{
	Use:   "edit [base-image] [instruction]",
	Short: "Edit an image or compose multiple images",
	Long: `Edit an existing image or compose multiple images using natural language instructions.

Supports multi-image composition: provide a base image and additional images to blend them together.
Works best with up to 3 input images total (base + additional).

Examples:
  # Edit a single image
  imagemage edit photo.png "make it sunset lighting"
  imagemage edit landscape.png "add a rainbow in the sky"

  # Compose multiple images
  imagemage edit background.png "add this person on the left" -i person.png
  imagemage edit scene.png "put these people here" -i person1.png -i person2.png

  # Complex composition
  imagemage edit office.png "add this person and this laptop" -i person.png -i laptop.png`,
	Args: cobra.ExactArgs(2),
	RunE: runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)

	editCmd.Flags().StringVarP(&editOutput, "output", "o", "", "Output path for edited image (default: base-image-edited.png)")
	editCmd.Flags().StringArrayVarP(&editInputs, "input", "i", []string{}, "Additional input images for composition (can be used multiple times)")
	editCmd.Flags().StringVarP(&editAspectRatio, "aspect-ratio", "a", "", "Aspect ratio for output")
	editCmd.Flags().StringVarP(&editResolution, "resolution", "r", "", "Image resolution (1K, 2K, 4K). Defaults to 4K for Pro model, 1K for --frugal")
	editCmd.Flags().BoolVarP(&editFrugal, "frugal", "f", false, "Use the cheaper gemini-2.5-flash-image model")
	editCmd.Flags().BoolVar(&editForce, "force", false, "Overwrite output file if it exists")
	editCmd.Flags().BoolVar(&editStorePrompt, "store-prompt", false, "Store instruction in PNG metadata")
}

func runEdit(cmd *cobra.Command, args []string) error {
	baseImagePath := args[0]
	instruction := args[1]

	// Check if base image exists
	if _, err := os.Stat(baseImagePath); os.IsNotExist(err) {
		return fmt.Errorf("base image not found: %s", baseImagePath)
	}

	// Check additional input images
	for _, inputPath := range editInputs {
		if _, err := os.Stat(inputPath); os.IsNotExist(err) {
			return fmt.Errorf("input image not found: %s", inputPath)
		}
	}

	// Total images check (base + additional)
	totalImages := 1 + len(editInputs)
	if totalImages > 14 {
		return fmt.Errorf("too many input images (%d). Maximum is 14 (base + additional)", totalImages)
	}
	if totalImages > 3 {
		fmt.Printf("⚠️  Using %d images. API works best with 3 or fewer images.\n", totalImages)
	}

	// Determine output path
	outputPath := editOutput
	if outputPath == "" {
		ext := filepath.Ext(baseImagePath)
		baseName := strings.TrimSuffix(filepath.Base(baseImagePath), ext)
		outputPath = filepath.Join(filepath.Dir(baseImagePath), baseName+"-edited"+ext)
	}

	// Check if output exists
	if !editForce {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("output file already exists: %s (use --force to overwrite)", outputPath)
		}
	}

	// Validate aspect ratio if provided
	if editAspectRatio != "" {
		if err := gemini.ValidateAspectRatio(editAspectRatio); err != nil {
			return err
		}
	}

	// Validate frugal mode limitations (Gemini 2.5 Flash has fixed 1024px output)
	if editFrugal {
		if editResolution != "" {
			return fmt.Errorf("--frugal mode has fixed 1024px output and does not accept --resolution parameter. Gemini 2.5 Flash always outputs at 1024px. Remove --resolution or --frugal flag")
		}
		if totalImages > 1 {
			fmt.Printf("⚠️  Warning: Multi-image composition with --frugal mode may have limitations. Gemini 2.5 Flash multi-image capabilities are not well documented. For best results with %d images, consider using Gemini 3 Pro (remove --frugal flag).\n\n", totalImages)
		}
	}

	fmt.Printf("Loading base image: %s\n", filepath.Base(baseImagePath))

	// Load and encode base image
	baseImageBase64, err := filehandler.LoadImageAsBase64(baseImagePath)
	if err != nil {
		return fmt.Errorf("failed to load base image: %w", err)
	}

	// Load and encode additional images
	var allImagesBase64 []string
	allImagesBase64 = append(allImagesBase64, baseImageBase64)

	for i, inputPath := range editInputs {
		fmt.Printf("Loading input %d: %s\n", i+1, filepath.Base(inputPath))
		inputBase64, err := filehandler.LoadImageAsBase64(inputPath)
		if err != nil {
			return fmt.Errorf("failed to load input image %s: %w", inputPath, err)
		}
		allImagesBase64 = append(allImagesBase64, inputBase64)
	}

	// Create Gemini client
	var client *gemini.Client
	if editFrugal {
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

	// Display edit info
	fmt.Printf("\nEditing with %d image(s)\n", totalImages)
	fmt.Printf("Instruction: %s\n", instruction)
	if editAspectRatio != "" {
		fmt.Printf("Aspect Ratio: %s\n", editAspectRatio)
	}
	// Display resolution info
	if editFrugal {
		fmt.Printf("Resolution: 1024px (fixed)\n")
	} else {
		resolution := editResolution
		if resolution == "" {
			resolution = "4K"
		}
		fmt.Printf("Resolution: %s\n", resolution)
	}
	if editFrugal {
		fmt.Printf("Model: %s (frugal)\n", gemini.ModelNameFrugal)
	} else {
		fmt.Printf("Model: %s\n", gemini.ModelName)
	}
	fmt.Println("\nGenerating edited image...")

	// Generate with all images
	var editedImageData string
	if editResolution != "" || editAspectRatio != "" {
		editedImageData, err = client.GenerateContentWithFullOptions(instruction, allImagesBase64, editResolution, editAspectRatio)
	} else {
		editedImageData, err = client.GenerateContentWithImages(instruction, allImagesBase64, "")
	}

	if err != nil {
		return fmt.Errorf("failed to edit image: %w", err)
	}

	// Save edited image
	if err := filehandler.SaveImage(editedImageData, outputPath); err != nil {
		return fmt.Errorf("failed to save edited image: %w", err)
	}

	// Store instruction in metadata if requested
	if editStorePrompt {
		if err := metadata.AddPromptToPNG(outputPath, instruction); err != nil {
			fmt.Printf("⚠️  Warning: failed to store prompt in metadata: %v\n", err)
		}
	}

	fmt.Printf("✓ Saved to: %s\n", outputPath)
	if editStorePrompt {
		fmt.Printf("  (instruction stored in metadata)\n")
	}

	return nil
}
