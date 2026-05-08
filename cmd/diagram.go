package cmd

import (
	"context"
	"fmt"
	"imagemage/pkg/filehandler"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	diagramType   string
	diagramOutput string
)

var diagramCmd = &cobra.Command{
	Use:   "diagram [description]",
	Short: "Generate technical diagrams and flowcharts",
	Long: `Create technical diagrams, flowcharts, architecture diagrams, and visualizations.

Examples:
  imagemage diagram "CI/CD pipeline with testing stages"
  imagemage diagram "microservices architecture" --type="architecture"
  imagemage diagram "user authentication flow" --type="flowchart"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDiagram,
}

func init() {
	rootCmd.AddCommand(diagramCmd)

	diagramCmd.Flags().StringVar(&diagramType, "type", "diagram", "Diagram type: flowchart, architecture, sequence, entity-relationship")
	diagramCmd.Flags().StringVarP(&diagramOutput, "output", "o", ".", "Output directory")
}

func runDiagram(cmd *cobra.Command, args []string) error {
	description := args[0]

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
