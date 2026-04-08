package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ModelName       = "gemini-3-pro-image-preview"
	ModelNameFrugal = "gemini-3.1-flash-image-preview" // Nano Banana 2
	BaseURL         = "https://generativelanguage.googleapis.com/v1beta/models"
	FilenameSuffix  = "\n\nAfter generating the image, respond with a short (2-4 word) evocative filename for it. Just the words, no extension."
)

// Supported aspect ratios for Gemini image models
var SupportedAspectRatios = []string{
	"1:1",  // Square
	"16:9", // Landscape
	"9:16", // Portrait
	"4:3",  // Landscape
	"3:4",  // Portrait
	"3:2",  // Landscape
	"2:3",  // Portrait
	"21:9", // Ultra-wide
	"5:4",  // Flexible
	"4:5",  // Flexible
	"4:1",  // Extreme panorama (Nano Banana 2)
	"1:4",  // Extreme vertical (Nano Banana 2)
	"8:1",  // Ultra panorama (Nano Banana 2)
	"1:8",  // Ultra vertical (Nano Banana 2)
}

// Client represents a Gemini API client
type Client struct {
	apiKey        string
	httpClient    *http.Client
	model         string
	baseURL       string
	useBearerAuth bool
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom http.Client (e.g. one whose Transport
// injects an Authorization: Bearer header for service-account auth).
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) {
		cl.httpClient = c
		cl.useBearerAuth = true
	}
}

// NewClientWithOptions creates a Gemini client for the given model.
// When WithHTTPClient is provided, the API key requirement is skipped
// because the supplied transport handles authentication.
func NewClientWithOptions(model string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		model:      model,
		baseURL:    BaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	if !c.useBearerAuth {
		c.apiKey = getAPIKey()
		if c.apiKey == "" {
			return nil, fmt.Errorf("API key not found. Please set one of: NANOBANANA_GEMINI_API_KEY, NANOBANANA_GOOGLE_API_KEY, GEMINI_API_KEY, or GOOGLE_API_KEY")
		}
	}
	return c, nil
}

// GenerateRequest represents a request to generate content
type GenerateRequest struct {
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
}

// GenerationConfig represents generation configuration
type GenerationConfig struct {
	ResponseModalities []string     `json:"responseModalities,omitempty"`
	ImageConfig        *ImageConfig `json:"imageConfig,omitempty"`
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

// GenerateResult contains both image data and AI-suggested filename
type GenerateResult struct {
	ImageData     string // base64 encoded image
	SuggestedName string // AI-suggested filename (may be empty)
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
		httpClient: &http.Client{Timeout: 5 * time.Minute},
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

// FindClosestAspectRatio finds the closest supported aspect ratio for given dimensions
func FindClosestAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return "1:1" // fallback
	}

	inputRatio := float64(width) / float64(height)
	bestMatch := "1:1"
	smallestDiff := float64(1000)

	for _, ar := range SupportedAspectRatios {
		// Parse aspect ratio string (e.g., "16:9")
		var w, h int
		if _, err := fmt.Sscanf(ar, "%d:%d", &w, &h); err != nil || h == 0 {
			continue
		}

		arRatio := float64(w) / float64(h)
		diff := inputRatio - arRatio
		if diff < 0 {
			diff = -diff
		}

		if diff < smallestDiff {
			smallestDiff = diff
			bestMatch = ar
		}
	}

	return bestMatch
}

// GenerateContent sends a request to generate content
func (c *Client) GenerateContent(prompt string) (GenerateResult, error) {
	return c.GenerateContentWithOptions(prompt, "", "")
}

// GenerateContentWithImage sends a request to generate or edit content with an optional image
func (c *Client) GenerateContentWithImage(prompt string, imageBase64 string) (GenerateResult, error) {
	return c.GenerateContentWithImages(prompt, []string{imageBase64}, "")
}

// GenerateContentWithImages sends a request with multiple input images
func (c *Client) GenerateContentWithImages(prompt string, imagesBase64 []string, aspectRatio string) (GenerateResult, error) {
	return c.GenerateContentWithFullOptions(prompt, imagesBase64, "", aspectRatio)
}

// GenerateContentWithResolution sends a request with resolution and aspect ratio
func (c *Client) GenerateContentWithResolution(prompt string, resolution string, aspectRatio string) (GenerateResult, error) {
	return c.GenerateContentWithFullOptions(prompt, nil, resolution, aspectRatio)
}

// GenerateContentWithOptions sends a request to generate or edit content with full options
func (c *Client) GenerateContentWithOptions(prompt string, imageBase64 string, aspectRatio string) (GenerateResult, error) {
	var images []string
	if imageBase64 != "" {
		images = []string{imageBase64}
	}
	return c.GenerateContentWithFullOptions(prompt, images, "", aspectRatio)
}

// GenerateContentWithFullOptions sends a request with all options including multiple images
func (c *Client) GenerateContentWithFullOptions(prompt string, imagesBase64 []string, resolution string, aspectRatio string) (GenerateResult, error) {
	// Validate aspect ratio
	if err := ValidateAspectRatio(aspectRatio); err != nil {
		return GenerateResult{}, err
	}
	fullPrompt := prompt + FilenameSuffix
	parts := []Part{
		{Text: fullPrompt},
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
	// Both Pro and Nano Banana 2 (frugal) support 512px, 1K, 2K, 4K
	imageConfig := &ImageConfig{
		AspectRatio: aspectRatio,
	}

	imageSize := resolution
	if imageSize == "" {
		imageSize = "4K" // Default to highest quality
	}
	imageConfig.ImageSize = imageSize

	reqBody.GenerationConfig = &GenerationConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageConfig:        imageConfig,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("failed to marshal request: %w", err)
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

	var url string
	if c.useBearerAuth {
		url = fmt.Sprintf("%s/%s:generateContent", baseURL, model)
	} else {
		url = fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, model, c.apiKey)
	}

	if os.Getenv("DEBUG") != "" {
		debugURL := url
		if !c.useBearerAuth {
			debugURL = fmt.Sprintf("%s/%s:generateContent?key=REDACTED", baseURL, model)
		}
		fmt.Fprintf(os.Stderr, "DEBUG: Request URL: %s\n", debugURL)
	}

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return GenerateResult{}, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return GenerateResult{}, fmt.Errorf("failed to read response: %w", err)
		}

		// Debug: Print response if DEBUG env var is set
		if os.Getenv("DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: Response status: %d\n", resp.StatusCode)
			fmt.Fprintf(os.Stderr, "DEBUG: Response body:\n%s\n", string(body))
		}

		if resp.StatusCode != http.StatusOK {
			return GenerateResult{}, c.handleError(resp.StatusCode, body)
		}

		var result GenerateResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return GenerateResult{}, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if result.Error != nil {
			return GenerateResult{}, fmt.Errorf("API error (%d): %s", result.Error.Code, result.Error.Message)
		}

		// Extract image data and suggested filename from response
		res := c.extractResult(&result)
		if res.ImageData != "" {
			return res, nil
		}

		// No image in response — collect text for diagnostics
		var textParts []string
		if len(result.Candidates) > 0 {
			for _, part := range result.Candidates[0].Content.Parts {
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}
			}
		}
		if len(textParts) > 0 {
			// Model explicitly refused — don't retry
			return GenerateResult{}, fmt.Errorf("no image data found in response (model said: %s)", strings.Join(textParts, " | "))
		}

		// Empty candidates with no explanation — transient, retry
		lastErr = fmt.Errorf("no image data found in response (empty candidates, attempt %d/%d)", attempt, maxRetries)
		if os.Getenv("DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: %v, retrying...\n", lastErr)
		}
	}

	return GenerateResult{}, lastErr
}

// extractResult extracts base64 image data and suggested filename from the response
func (c *Client) extractResult(result *GenerateResponse) GenerateResult {
	var res GenerateResult
	if len(result.Candidates) == 0 {
		return res
	}

	for _, part := range result.Candidates[0].Content.Parts {
		// Check for inline data (image)
		if part.InlineData != nil && part.InlineData.Data != "" {
			res.ImageData = part.InlineData.Data
		}

		// Check for text (suggested filename)
		if part.Text != "" && len(part.Text) < 100 {
			// Only use text if it looks like a filename suggestion (short)
			res.SuggestedName = part.Text
		}
	}

	return res
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
