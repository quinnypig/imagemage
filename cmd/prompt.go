package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func addPromptFileFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "prompt-file", "", "Read prompt from a file (use '-' for stdin)")
}

func resolvePrompt(positional, promptFile string) (string, error) {
	hasArg := positional != ""
	hasFile := promptFile != ""

	if hasArg && hasFile {
		return "", fmt.Errorf("provide the prompt either as a positional argument or via --prompt-file, not both")
	}
	if hasArg {
		return positional, nil
	}
	if !hasFile {
		return "", fmt.Errorf("a prompt is required: pass it as an argument or via --prompt-file")
	}

	var (
		data []byte
		err  error
	)
	if promptFile == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt from stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file: %w", err)
		}
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}
	return prompt, nil
}

func optionalArg(args []string, idx int) string {
	if idx < len(args) {
		return args[idx]
	}
	return ""
}
