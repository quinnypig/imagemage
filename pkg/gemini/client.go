package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	ModelName       = "gemini-3-pro-image-preview"
	ModelNameFrugal = "gemini-2.5-flash-image"
	BaseURL         = "https://generativelanguage.googleapis.com/v1beta/models"
)

// Supported aspect ratios for Gemini image models
var SupportedAspectRatios = []string{
	"1:1",   // Square
	"16:9",  // Landscape
	"9:16",  // Portrait
	"4:3",   // Landscape
	"3:4",   // Portrait
	"3:2",   // Landscape
	"2:3",   // Portrait
	"21:9",  // Ultra-wide
	"5:4",   // Flexible
	"4:5",   // Flexible
}

// Client represents a Gemini API client
type Client struct {
	apiKey     string
	httpClient *http.Client
	model      string
	baseURL    string
}

// GenerateRequest represents a request to generate content
type GenerateRequest struct {
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
}

// GenerationConfig represents generation configuration
type GenerationConfig struct {
	ImageConfig *ImageConfig `json:"imageConfig,omitempty"`
}

// ImageConfig represents image-specific configuration
type ImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

// Content represents content in the request
type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

// Part represents a part of the content
type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

// InlineData represents inline data (e.g., images)
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

// GenerateResponse represents the API response
type GenerateResponse struct {
	Candidates []Candidate `json:"candidates"`
	Error      *ErrorInfo  `json:"error,omitempty"`
}

// Candidate represents a response candidate
type Candidate struct {
	Content Content `json:"content"`
}

// ErrorInfo represents error information from the API
type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// NewClient creates a new Gemini API client with the default model
func NewClient() (*Client, error) {
	return NewClientWithModel(ModelName)
}

// NewFrugalClient creates a new Gemini API client with the frugal model
func NewFrugalClient() (*Client, error) {
	return NewClientWithModel(ModelNameFrugal)
}

// NewClientWithModel creates a new Gemini API client with a specific model
func NewClientWithModel(model string) (*Client, error) {
	apiKey := getAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key not found. Please set one of: NANOBANANA_GEMINI_API_KEY, NANOBANANA_GOOGLE_API_KEY, GEMINI_API_KEY, or GOOGLE_API_KEY")
	}

	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		model:      model,
		baseURL:    BaseURL,
	}, nil
}

// getAPIKey retrieves the API key from environment variables
func getAPIKey() string {
	keys := []string{
		"NANOBANANA_GEMINI_API_KEY",
		"NANOBANANA_GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
	}

	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}

	return ""
}

// ValidateAspectRatio checks if the aspect ratio is supported
func ValidateAspectRatio(aspectRatio string) error {
	if aspectRatio == "" {
		return nil // Empty is valid (uses default)
	}

	for _, supported := range SupportedAspectRatios {
		if aspectRatio == supported {
			return nil
		}
	}

	return fmt.Errorf("unsupported aspect ratio: %s. Supported: %v", aspectRatio, SupportedAspectRatios)
}

// GenerateContent sends a request to generate content
func (c *Client) GenerateContent(prompt string) (string, error) {
	return c.GenerateContentWithOptions(prompt, "", "")
}

// GenerateContentWithImage sends a request to generate or edit content with an optional image
func (c *Client) GenerateContentWithImage(prompt string, imageBase64 string) (string, error) {
	return c.GenerateContentWithImages(prompt, []string{imageBase64}, "")
}

// GenerateContentWithImages sends a request with multiple input images
func (c *Client) GenerateContentWithImages(prompt string, imagesBase64 []string, aspectRatio string) (string, error) {
	return c.GenerateContentWithFullOptions(prompt, imagesBase64, "", aspectRatio)
}

// GenerateContentWithResolution sends a request with resolution and aspect ratio
func (c *Client) GenerateContentWithResolution(prompt string, resolution string, aspectRatio string) (string, error) {
	return c.GenerateContentWithFullOptions(prompt, nil, resolution, aspectRatio)
}

// GenerateContentWithOptions sends a request to generate or edit content with full options
func (c *Client) GenerateContentWithOptions(prompt string, imageBase64 string, aspectRatio string) (string, error) {
	var images []string
	if imageBase64 != "" {
		images = []string{imageBase64}
	}
	return c.GenerateContentWithFullOptions(prompt, images, "", aspectRatio)
}

// GenerateContentWithFullOptions sends a request with all options including multiple images
func (c *Client) GenerateContentWithFullOptions(prompt string, imagesBase64 []string, resolution string, aspectRatio string) (string, error) {
	// Validate aspect ratio
	if err := ValidateAspectRatio(aspectRatio); err != nil {
		return "", err
	}
	parts := []Part{
		{Text: prompt},
	}

	// Add images if provided (for editing/composition)
	for _, imageBase64 := range imagesBase64 {
		if imageBase64 != "" {
			parts = append(parts, Part{
				InlineData: &InlineData{
					MimeType: "image/png",
					Data:     imageBase64,
				},
			})
		}
	}

	reqBody := GenerateRequest{
		Contents: []Content{
			{
				Role:  "user",
				Parts: parts,
			},
		},
	}

	// Configure image generation based on model capabilities
	imageConfig := &ImageConfig{
		AspectRatio: aspectRatio,
	}

	// Frugal model (2.5 Flash) has fixed 1024px output and doesn't accept imageSize parameter
	// Pro model supports 1K, 2K, 4K via imageSize parameter
	if c.model != ModelNameFrugal {
		imageSize := resolution
		if imageSize == "" {
			imageSize = "4K" // Pro model default
		}
		imageConfig.ImageSize = imageSize
	}
	// For frugal model, omit ImageSize entirely (fixed 1024px output)

	reqBody.GenerationConfig = &GenerationConfig{
		ImageConfig: imageConfig,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug: Print request body if DEBUG env var is set
	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: Request body:\n%s\n", string(jsonData))
	}

	// Use client's model and baseURL, falling back to defaults if not set
	model := c.model
	if model == "" {
		model = ModelName
	}
	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = BaseURL
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, model, c.apiKey)

	// Debug: Print URL (without API key) if DEBUG env var is set
	if os.Getenv("DEBUG") != "" {
		debugURL := fmt.Sprintf("%s/%s:generateContent?key=REDACTED", baseURL, model)
		fmt.Fprintf(os.Stderr, "DEBUG: Request URL: %s\n", debugURL)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Debug: Print response if DEBUG env var is set
	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: Response status: %d\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "DEBUG: Response body:\n%s\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return "", c.handleError(resp.StatusCode, body)
	}

	var result GenerateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("API error (%d): %s", result.Error.Code, result.Error.Message)
	}

	// Extract image data from response
	imageData := c.extractImageData(&result)
	if imageData == "" {
		return "", fmt.Errorf("no image data found in response")
	}

	return imageData, nil
}

// extractImageData extracts base64 image data from the response
func (c *Client) extractImageData(result *GenerateResponse) string {
	if len(result.Candidates) == 0 {
		return ""
	}

	for _, part := range result.Candidates[0].Content.Parts {
		// Check for inline data (preferred)
		if part.InlineData != nil && part.InlineData.Data != "" {
			return part.InlineData.Data
		}

		// Fallback to text field (validate it's base64 and long enough)
		if part.Text != "" && len(part.Text) > 1000 {
			// Simple validation that it looks like base64
			if !strings.Contains(part.Text, " ") && !strings.Contains(part.Text, "\n") {
				return part.Text
			}
		}
	}

	return ""
}

// handleError handles API errors and returns user-friendly messages
func (c *Client) handleError(statusCode int, body []byte) error {
	bodyStr := string(body)

	// Try to parse error response
	var errResp GenerateResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		bodyStr = errResp.Error.Message
	}

	switch statusCode {
	case 400:
		if strings.Contains(bodyStr, "safety") {
			return fmt.Errorf("request rejected due to safety concerns")
		}
		return fmt.Errorf("malformed request: %s", bodyStr)
	case 403:
		if strings.Contains(strings.ToLower(bodyStr), "api key not valid") {
			return fmt.Errorf("invalid API key")
		}
		if strings.Contains(strings.ToLower(bodyStr), "quota") {
			return fmt.Errorf("API quota exceeded")
		}
		return fmt.Errorf("authentication failed: %s", bodyStr)
	case 500:
		return fmt.Errorf("service error: %s", bodyStr)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, bodyStr)
	}
}
