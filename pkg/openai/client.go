package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"imagemage/pkg/imagegen"
)

const (
	ModelName      = "gpt-image-2"
	BaseURL        = "https://api.openai.com/v1/images"
	defaultTimeout = 5 * time.Minute
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	model      string
	baseURL    string
}

type Option func(*Client)

func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		cl.httpClient = c
	}
}

func WithBaseURL(baseURL string) Option {
	return func(cl *Client) {
		cl.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithAPIKey(apiKey string) Option {
	return func(cl *Client) {
		cl.apiKey = apiKey
	}
}

func NewClient(model string, opts ...Option) (*Client, error) {
	if model == "" {
		model = ModelName
	}

	c := &Client{
		apiKey:     os.Getenv("OPENAI_API_KEY"),
		httpClient: &http.Client{Timeout: defaultTimeout},
		model:      model,
		baseURL:    BaseURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("API key not found. Please set OPENAI_API_KEY")
	}
	return c, nil
}

func (c *Client) Generate(ctx context.Context, req imagegen.Request) (imagegen.Result, error) {
	size, err := normalizeSize(req)
	if err != nil {
		return imagegen.Result{}, err
	}
	body := generationRequest{
		Model:        c.model,
		Prompt:       req.Prompt,
		Size:         size,
		Quality:      normalizeQuality(req.Quality),
		OutputFormat: normalizeOutputFormat(req.OutputFormat),
		N:            1,
	}
	return c.postJSON(ctx, "/generations", body)
}

func (c *Client) Edit(ctx context.Context, req imagegen.Request) (imagegen.Result, error) {
	if len(req.Images) == 0 {
		return c.Generate(ctx, req)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	size, err := normalizeSize(req)
	if err != nil {
		return imagegen.Result{}, err
	}

	fields := map[string]string{
		"model":         c.model,
		"prompt":        req.Prompt,
		"size":          size,
		"quality":       normalizeQuality(req.Quality),
		"output_format": normalizeOutputFormat(req.OutputFormat),
		"n":             "1",
	}
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := writer.WriteField(k, v); err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to write form field %s: %w", k, err)
		}
	}

	for i, img := range req.Images {
		mimeType := img.MimeType
		if mimeType == "" {
			mimeType = "image/png"
		}
		ext := extensionForMime(mimeType)
		part, err := writer.CreateFormFile("image", fmt.Sprintf("input-%d%s", i+1, ext))
		if err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to create image form part: %w", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(img.Base64)
		if err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to decode input image %d: %w", i+1, err)
		}
		if _, err := part.Write(decoded); err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to write image form part: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return imagegen.Result{}, fmt.Errorf("failed to close multipart body: %w", err)
	}

	return c.do(ctx, http.MethodPost, c.baseURL+"/edits", writer.FormDataContentType(), body.Bytes())
}

func normalizeSize(req imagegen.Request) (string, error) {
	size, err := imagegen.OpenAISize(req.Resolution, req.AspectRatio)
	if err != nil {
		return "", err
	}
	if size == "" {
		return "auto", nil
	}
	return size, nil
}

func normalizeQuality(quality string) string {
	switch quality {
	case "low", "medium", "high", "auto":
		return quality
	default:
		return "auto"
	}
}

func normalizeOutputFormat(format string) string {
	switch format {
	case "png", "jpeg", "webp":
		return format
	default:
		return "png"
	}
}

func extensionForMime(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func (c *Client) postJSON(ctx context.Context, path string, body generationRequest) (imagegen.Result, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return imagegen.Result{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: OpenAI request body:\n%s\n", string(jsonData))
	}

	return c.do(ctx, http.MethodPost, c.baseURL+path, "application/json", jsonData)
}

func (c *Client) do(ctx context.Context, method, url, contentType string, body []byte) (imagegen.Result, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", contentType)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return imagegen.Result{}, fmt.Errorf("failed to read response: %w", err)
		}

		if os.Getenv("DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: OpenAI response status: %d\n", resp.StatusCode)
			fmt.Fprintf(os.Stderr, "DEBUG: OpenAI response body:\n%s\n", string(body))
		}

		if resp.StatusCode == http.StatusOK {
			return c.extractResult(body)
		}

		lastErr = c.handleError(resp.StatusCode, body)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		return imagegen.Result{}, lastErr
	}

	return imagegen.Result{}, lastErr
}

func (c *Client) extractResult(body []byte) (imagegen.Result, error) {
	var result imageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return imagegen.Result{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(result.Data) == 0 || result.Data[0].B64JSON == "" {
		return imagegen.Result{}, fmt.Errorf("no image data found in response")
	}
	return imagegen.Result{
		ImageData: result.Data[0].B64JSON,
		Provider:  imagegen.ProviderOpenAI,
		Model:     c.model,
	}, nil
}

func (c *Client) handleError(statusCode int, body []byte) error {
	bodyStr := string(body)
	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		bodyStr = errResp.Error.Message
	}

	switch statusCode {
	case 400:
		return fmt.Errorf("malformed request: %s", bodyStr)
	case 401, 403:
		return fmt.Errorf("authentication failed: %s", bodyStr)
	case 429:
		return fmt.Errorf("rate limit exceeded: %s", bodyStr)
	case 500, 502, 503, 504:
		return fmt.Errorf("service error: %s", bodyStr)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, bodyStr)
	}
}

type generationRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	N            int    `json:"n,omitempty"`
}

type imageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}
