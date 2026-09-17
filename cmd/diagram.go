package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	diagramType       string
	diagramOutput     string
	diagramPromptFile string
)

var diagramCmd = &cobra.Command{
	Use:   "diagram [description]",
	Short: "Generate technical diagrams and flowcharts",
	Long: `Create technical diagrams, flowcharts, architecture diagrams, and visualizations.

Examples:
  imagemage diagram "CI/CD pipeline with testing stages"
  imagemage diagram "microservices architecture" --type="architecture"
  imagemage diagram "user authentication flow" --type="flowchart"
  imagemage diagram --prompt-file ./diagram.txt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDiagram,
}

func init() {
	rootCmd.AddCommand(diagramCmd)

	diagramCmd.Flags().StringVar(&diagramType, "type", "diagram", "Diagram type, used verbatim in the prompt: flowchart, architecture, sequence, entity-relationship, or any free-text type")
	diagramCmd.Flags().StringVarP(&diagramOutput, "output", "o", ".", "Output directory")
	addPromptFileFlag(diagramCmd, &diagramPromptFile)
}

func runDiagram(cmd *cobra.Command, args []string) error {
	description, err := resolvePrompt(optionalArg(args, 0), diagramPromptFile)
	if err != nil {
		return err
	}

	// Build prompt
	prompt := fmt.Sprintf("Create a clear, professional %s diagram: %s. ", diagramType, description)
	prompt += "The diagram should be well-organized, easy to read, with clear labels, appropriate shapes/symbols, "
	prompt += "connecting lines/arrows, and good visual hierarchy. Use a clean, technical style."

	client, provider, model, err := newImageClient(false)
	if err != nil {
		return fmt.Errorf("failed to create image client: %w", err)
	}

	fmt.Printf("Generating %s: %s\n", diagramType, description)
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Model: %s\n", model)

	// Generate diagram
	req := generationRequest(prompt, "", "", nil, false)
	if req.Quality == "auto" {
		req.Quality = "high"
	}
	result, err := client.Generate(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to generate diagram: %w", err)
	}

	// Generate filename (prefer AI-suggested name)
	filename := filehandler.GenerateFilename(description, result.SuggestedName, diagramType, 0)
	filename = applyOutputFormatExtension(filename)
	outputPath := filepath.Join(diagramOutput, filename)
	outputPath = filehandler.EnsureUniqueFilename(outputPath)

	// Save diagram
	if err := filehandler.SaveImage(result.ImageData, outputPath); err != nil {
		return fmt.Errorf("failed to save diagram: %w", err)
	}

	fmt.Printf("✓ Diagram saved to: %s\n", outputPath)

	return nil
}
