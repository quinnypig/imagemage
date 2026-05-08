package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	storyFrames int
	storyOutput string
	storyStyle  string
)

var storyCmd = &cobra.Command{
	Use:   "story [narrative]",
	Short: "Generate sequential images for visual narratives",
	Long: `Create a sequence of images that tell a story or show progression.

Examples:
  imagemage story "a seed growing into a tree" --frames=4
  imagemage story "day to night transition in a city" --frames=6 --style="cinematic"
  imagemage story "character transformation" --frames=3`,
	Args: cobra.MinimumNArgs(1),
	RunE: runStory,
}

func init() {
	rootCmd.AddCommand(storyCmd)

	storyCmd.Flags().IntVarP(&storyFrames, "frames", "f", 3, "Number of frames/scenes to generate")
	storyCmd.Flags().StringVarP(&storyStyle, "style", "s", "", "Visual style for the story")
	storyCmd.Flags().StringVarP(&storyOutput, "output", "o", ".", "Output directory")
}

func runStory(cmd *cobra.Command, args []string) error {
	narrative := args[0]

	if storyFrames < 2 {
		return fmt.Errorf("frames must be at least 2")
	}
	if storyFrames > 10 {
		return fmt.Errorf("frames cannot exceed 10")
	}

	client, provider, model, err := newImageClient(false)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	fmt.Printf("Generating story: %s\n", narrative)
	fmt.Printf("Frames: %d\n", storyFrames)
	if storyStyle != "" {
		fmt.Printf("Style: %s\n", storyStyle)
	}
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s\n", model)
	fmt.Println()

	successCount := 0
	ctx := context.Background()
	for i := 1; i <= storyFrames; i++ {
		// Create frame-specific prompt
		prompt := fmt.Sprintf("Frame %d of %d in a visual narrative: %s", i, storyFrames, narrative)
		switch i {
		case 1:
			prompt += " (beginning/opening scene)"
		case storyFrames:
			prompt += " (ending/final scene)"
		default:
			prompt += fmt.Sprintf(" (progression, scene %d)", i)
		}

		if storyStyle != "" {
			prompt += fmt.Sprintf(", style: %s", storyStyle)
		}

		fmt.Printf("[%d/%d] Generating frame...\n", i, storyFrames)

		// Generate image
		req := generationRequest(prompt, "", "", nil, false)
		result, err := client.Generate(ctx, req)
		if err != nil {
			fmt.Printf("Error generating frame %d: %v\n", i, err)
			continue
		}

		// Generate filename (prefer AI-suggested name, with frame prefix)
		filename := filehandler.GenerateFilename(narrative, result.SuggestedName, fmt.Sprintf("story_frame_%02d", i), 0)
		filename = applyOutputFormatExtension(filename)
		outputPath := filepath.Join(storyOutput, filename)
		outputPath = filehandler.EnsureUniqueFilename(outputPath)

		// Save image
		if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
			fmt.Printf("Error saving frame %d: %v\n", i, err)
			continue
		}

		fmt.Printf("✓ Saved frame %d to: %s\n", i, outputPath)
		successCount++
	}

	fmt.Printf("\nSuccessfully generated %d/%d story frames\n", successCount, storyFrames)

	return nil
}
