package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/imagegen"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	restoreOutput string
)

var restoreCmd = &cobra.Command{
	Use:   "restore [image-path]",
	Short: "Enhance and repair photos",
	Long: `Restore old photos, enhance quality, remove noise, and improve overall image quality.

Examples:
  imagemage restore old_photo.png
  imagemage restore damaged.jpg --output=restored.png`,
	Args: cobra.ExactArgs(1),
	RunE: runRestore,
}

func init() {
	rootCmd.AddCommand(restoreCmd)

	restoreCmd.Flags().StringVarP(&restoreOutput, "output", "o", "", "Output path for restored image")
}

func runRestore(cmd *cobra.Command, args []string) error {
	imagePath := args[0]

	fmt.Printf("Loading image: %s\n", imagePath)

	// Load image as base64
	imageInput, err := loadImageInput(imagePath)
	if err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	client, provider, model, err := newImageClient(false)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	prompt := "Restore and enhance this photo. Remove noise, improve clarity, fix any damage or artifacts, enhance colors naturally, and improve overall quality while preserving the original character of the image."

	fmt.Println("Restoring and enhancing photo...")
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s\n", model)

	// Generate restored image
	req := generationRequest(prompt, "", "", []imagegen.ImageInput{imageInput}, false)
	result, err := client.Edit(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to restore image: %w", err)
	}

	// Determine output path
	outputPath := restoreOutput
	if outputPath == "" {
		ext := filepath.Ext(imagePath)
		base := strings.TrimSuffix(imagePath, ext)
		outputPath = base + "_restored" + imageFileExtension()
	}

	outputPath = filehandler.EnsureUniqueFilename(outputPath)

	// Save restored image
	if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
		return fmt.Errorf("failed to save restored image: %w", err)
	}

	fmt.Printf("✓ Restored image saved to: %s\n", outputPath)

	return nil
}
