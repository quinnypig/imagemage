package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersionInfo sets the version info from main package
func SetVersionInfo(v, _, _ string) {
	version = v
	rootCmd.Version = version
}

var rootCmd = &cobra.Command{
	Use:     "imagemage",
	Version: version,
	Short:   "A CLI tool for generating and manipulating images using OpenAI or Gemini image models",
	Long: `Imagemage is a focused CLI tool for image generation using OpenAI or Gemini APIs.

Supports multiple providers:
  • OpenAI Images (default) - GPT Image generation and editing
  • Gemini image models (--provider=gemini) - Gemini 3 Pro Image and Nano Banana 2

Features include text-to-image creation, image editing, photo restoration, icon generation,
pattern creation, visual narratives, and technical diagrams.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Cobra automatically adds --version flag when Version is set
}
