package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/imagegen"
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
	editPromptFile  string
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
  imagemage edit office.png "add this person and this laptop" -i person.png -i laptop.png

  # Read instruction from a file
  imagemage edit photo.png --prompt-file ./instruction.txt`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)

	editCmd.Flags().StringVarP(&editOutput, "output", "o", "", "Output path for edited image (default: base-image-edited.png)")
	editCmd.Flags().StringArrayVarP(&editInputs, "input", "i", []string{}, "Additional input images for composition (can be used multiple times)")
	editCmd.Flags().StringVarP(&editAspectRatio, "aspect-ratio", "a", "", "Aspect ratio for output (auto-detected from input if not specified)")
	editCmd.Flags().StringVarP(&editResolution, "resolution", "r", "", "Image resolution (512px, 1K, 2K, 4K). Defaults to 4K")
	editCmd.Flags().BoolVarP(&editFrugal, "frugal", "f", false, "Deprecated alias for low-cost generation; with Gemini selects Nano Banana 2")
	editCmd.Flags().BoolVar(&editForce, "force", false, "Overwrite output file if it exists")
	editCmd.Flags().BoolVar(&editStorePrompt, "store-prompt", false, "Store instruction in PNG metadata")
	addPromptFileFlag(editCmd, &editPromptFile)
}

func runEdit(cmd *cobra.Command, args []string) error {
	baseImagePath := args[0]
	instruction, err := resolvePrompt(optionalArg(args, 1), editPromptFile)
	if err != nil {
		return err
	}

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
		baseName = filehandler.SanitizeFilenameStem(baseName)
		outputPath = filepath.Join(filepath.Dir(baseImagePath), baseName+"-edited"+imageFileExtension())
	}

	// Check if output exists
	if !editForce {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("output file already exists: %s (use --force to overwrite)", outputPath)
		}
	}

	// Auto-detect aspect ratio from base image if not specified
	detectedAspectRatio := ""
	if editAspectRatio == "" {
		width, height, err := filehandler.GetImageDimensions(baseImagePath)
		if err != nil {
			fmt.Printf("⚠️  Could not detect image dimensions: %v\n", err)
		} else {
			detectedAspectRatio = imagegen.FindClosestAspectRatio(width, height)
			editAspectRatio = detectedAspectRatio
		}
	}

	// Validate aspect ratio if provided
	if editAspectRatio != "" {
		if err := imagegen.ValidateAspectRatio(editAspectRatio); err != nil {
			return err
		}
	}

	fmt.Printf("Loading base image: %s\n", filepath.Base(baseImagePath))

	// Load and encode base image
	baseImage, err := loadImageInput(baseImagePath)
	if err != nil {
		return fmt.Errorf("failed to load base image: %w", err)
	}

	// Load and encode additional images
	allImages := []imagegen.ImageInput{baseImage}

	for i, inputPath := range editInputs {
		fmt.Printf("Loading input %d: %s\n", i+1, filepath.Base(inputPath))
		inputImage, err := loadImageInput(inputPath)
		if err != nil {
			return fmt.Errorf("failed to load input image %s: %w", inputPath, err)
		}
		allImages = append(allImages, inputImage)
	}

	client, provider, model, err := newImageClient(editFrugal)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	// Display edit info
	fmt.Printf("\nEditing with %d image(s)\n", totalImages)
	fmt.Printf("Instruction: %s\n", instruction)
	if editAspectRatio != "" {
		if detectedAspectRatio != "" {
			fmt.Printf("Aspect Ratio: %s (auto-detected from input)\n", editAspectRatio)
		} else {
			fmt.Printf("Aspect Ratio: %s\n", editAspectRatio)
		}
	}
	// Display resolution info
	resolution := editResolution
	if resolution == "" {
		resolution = "4K"
	}
	fmt.Printf("Resolution: %s\n", resolution)
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Quality: %s\n", generationRequest("", "", "", nil, editFrugal).Quality)
	fmt.Println("\nGenerating edited image...")

	// Generate with all images
	req := generationRequest(instruction, editResolution, editAspectRatio, allImages, editFrugal)
	result, err := client.Edit(context.Background(), req)

	if err != nil {
		return fmt.Errorf("failed to edit image: %w", err)
	}

	// Save edited image
	if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
		return fmt.Errorf("failed to save edited image: %w", err)
	}

	// Store instruction in metadata if requested
	if editStorePrompt {
		if !canStorePNGMetadata() {
			fmt.Printf("⚠️  Warning: prompt metadata is only supported for PNG output\n")
		} else if err := metadata.AddPromptToPNG(outputPath, instruction); err != nil {
			fmt.Printf("⚠️  Warning: failed to store prompt in metadata: %v\n", err)
		}
	}

	fmt.Printf("✓ Saved to: %s\n", outputPath)
	if editStorePrompt {
		fmt.Printf("  (instruction stored in metadata)\n")
	}

	return nil
}
