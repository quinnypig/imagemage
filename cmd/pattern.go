package cmd

import (
	"fmt"
	"imagemage/pkg/filehandler"
	"imagemage/pkg/gemini"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	patternType       string
	patternStyle      string
	patternOutput     string
	patternPromptFile string
)

var patternCmd = &cobra.Command{
	Use:   "pattern [description]",
	Short: "Create seamless patterns and textures",
	Long: `Generate seamless patterns and textures for backgrounds, designs, and textures.

Examples:
  imagemage pattern "geometric triangles"
  imagemage pattern "floral" --type="seamless" --style="watercolor"
  imagemage pattern "hexagons" --style="minimal, modern"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPattern,
}

func init() {
	rootCmd.AddCommand(patternCmd)

	patternCmd.Flags().StringVar(&patternType, "type", "seamless", "Pattern type: seamless, tiled, texture")
	patternCmd.Flags().StringVarP(&patternStyle, "style", "s", "", "Pattern style")
	patternCmd.Flags().StringVarP(&patternOutput, "output", "o", ".", "Output directory")
	addPromptFileFlag(patternCmd, &patternPromptFile)
}

func runPattern(cmd *cobra.Command, args []string) error {
	description, err := resolvePrompt(optionalArg(args, 0), patternPromptFile)
	if err != nil {
		return err
	}

	// Build prompt
	prompt := fmt.Sprintf("Create a %s pattern: %s", patternType, description)
	if patternStyle != "" {
		prompt += fmt.Sprintf(", style: %s", patternStyle)
	}
	prompt += ". The pattern should tile seamlessly and be suitable for use as a background or texture."

	// Create Gemini client
	client, err := gemini.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	fmt.Printf("Generating %s pattern: %s\n", patternType, description)
	if patternStyle != "" {
		fmt.Printf("Style: %s\n", patternStyle)
	}

	// Generate pattern
	result, err := client.GenerateContent(prompt)
	if err != nil {
		return fmt.Errorf("failed to generate pattern: %w", err)
	}

	// Generate filename (prefer AI-suggested name)
	filename := filehandler.GenerateFilename(description, result.SuggestedName, "pattern", 0)
	outputPath := filepath.Join(patternOutput, filename)
	outputPath = filehandler.EnsureUniqueFilename(outputPath)

	// Save pattern
	if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
		return fmt.Errorf("failed to save pattern: %w", err)
	}

	fmt.Printf("✓ Pattern saved to: %s\n", outputPath)

	return nil
}
