package cmd

import (
	"fmt"
	"os"

	"imagemage/pkg/gemini"
	"imagemage/pkg/openai"

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
	Long: fmt.Sprintf(`Imagemage is a focused CLI tool for image generation using OpenAI or Gemini APIs.

Supports multiple providers:
  • OpenAI Images (default) - GPT Image generation and editing (model: %s)
  • Gemini image models (--provider=gemini) - Gemini 3 Pro Image (%s)
    and Nano Banana 2 (%s)

Authentication (required before first use):
  OpenAI:  export OPENAI_API_KEY="..."
  Gemini:  set one of NANOBANANA_GEMINI_API_KEY, NANOBANANA_GOOGLE_API_KEY,
           GEMINI_API_KEY, or GOOGLE_API_KEY (checked in that order)

The default provider can be set with IMAGEMAGE_PROVIDER=openai|gemini instead
of passing --provider on every call.

Features include text-to-image creation, image editing, photo restoration, icon generation,
pattern creation, visual narratives, and technical diagrams. Output filenames are derived
from the prompt (or an AI-suggested name); the final path is printed after each save.

Full documentation: https://github.com/quinnypig/imagemage`,
		openai.ModelName, gemini.ModelName, gemini.ModelNameFrugal),
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
