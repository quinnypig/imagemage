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
	"net/textproto"
	"os"
	"strings"
	"time"

	"imagemage/pkg/imagegen"
	"imagemage/pkg/metadata"
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
	results, err := c.generate(ctx, req, 1)
	if err != nil {
		return imagegen.Result{}, err
	}
	return results[0], nil
}

func (c *Client) Edit(ctx context.Context, req imagegen.Request) (imagegen.Result, error) {
	if len(req.Images) == 0 {
		return c.Generate(ctx, req)
	}
	results, err := c.edit(ctx, req, 1)
	if err != nil {
		return imagegen.Result{}, err
	}
	return results[0], nil
}

// GenerateBatch implements imagegen.BatchGenerator, requesting up to req.Count
// images in a single API call via the `n` parameter. It routes to the edits
// endpoint when reference images are present, otherwise to generations.
func (c *Client) GenerateBatch(ctx context.Context, req imagegen.Request) ([]imagegen.Result, error) {
	n := req.Count
	if n < 1 {
		n = 1
	}
	if len(req.Images) > 0 {
		return c.edit(ctx, req, n)
	}
	return c.generate(ctx, req, n)
}

func (c *Client) generate(ctx context.Context, req imagegen.Request, n int) ([]imagegen.Result, error) {
	size, err := normalizeSize(req)
	if err != nil {
		return nil, err
	}
	if err := validateBackground(req); err != nil {
		return nil, err
	}
	body := generationRequest{
		Model:             c.model,
		Prompt:            req.Prompt,
		Size:              size,
		Quality:           normalizeQuality(req.Quality),
		OutputFormat:      normalizeOutputFormat(req.OutputFormat),
		Background:        normalizeBackground(req.Background),
		Moderation:        normalizeModeration(req.Moderation),
		OutputCompression: normalizeCompression(req),
		N:                 n,
	}
	return c.postJSON(ctx, "/generations", body)
}

func (c *Client) edit(ctx context.Context, req imagegen.Request, n int) ([]imagegen.Result, error) {
	if err := validateBackground(req); err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	size, err := normalizeSize(req)
	if err != nil {
		return nil, err
	}

	fields := map[string]string{
		"model":          c.model,
		"prompt":         req.Prompt,
		"size":           size,
		"quality":        normalizeQuality(req.Quality),
		"output_format":  normalizeOutputFormat(req.OutputFormat),
		"background":     normalizeBackground(req.Background),
		"input_fidelity": normalizeFidelity(req.InputFidelity),
		"moderation":     normalizeModeration(req.Moderation),
		"n":              fmt.Sprintf("%d", n),
	}
	if comp := normalizeCompression(req); comp != nil {
		fields["output_compression"] = fmt.Sprintf("%d", *comp)
	}
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := writer.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("failed to write form field %s: %w", k, err)
		}
	}

	for i, img := range req.Images {
		mimeType := img.MimeType
		if mimeType == "" {
			mimeType = "image/png"
		}
		ext := extensionForMime(mimeType)
		// OpenAI's /v1/images/edits requires the array form (name="image[]")
		// when more than one image is attached, and accepts it for single
		// images too. The image part also needs its real MIME type — the
		// default application/octet-stream from CreateFormFile causes the
		// endpoint to reject modern parameters like `quality`.
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="input-%d%s"`, i+1, ext))
		header.Set("Content-Type", mimeType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, fmt.Errorf("failed to create image form part: %w", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(img.Base64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode input image %d: %w", i+1, err)
		}
		if _, err := part.Write(decoded); err != nil {
			return nil, fmt.Errorf("failed to write image form part: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart body: %w", err)
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

// normalizeBackground returns "transparent"/"opaque" to be sent verbatim, or
// "" for the default ("auto"), which we omit so the API applies its default.
func normalizeBackground(background string) string {
	switch background {
	case "transparent", "opaque":
		return background
	default:
		return ""
	}
}

// normalizeFidelity returns "high"/"low" or "" to omit (API defaults to low).
func normalizeFidelity(fidelity string) string {
	switch fidelity {
	case "high", "low":
		return fidelity
	default:
		return ""
	}
}

// normalizeModeration returns "low" to relax moderation, or "" to omit so the
// API applies its default ("auto").
func normalizeModeration(moderation string) string {
	if moderation == "low" {
		return "low"
	}
	return ""
}

// normalizeCompression returns the output_compression value to send, or nil to
// omit it. Compression only applies to lossy formats (jpeg, webp).
func normalizeCompression(req imagegen.Request) *int {
	if req.Compression <= 0 || req.Compression > 100 {
		return nil
	}
	switch normalizeOutputFormat(req.OutputFormat) {
	case "jpeg", "webp":
		c := req.Compression
		return &c
	default:
		return nil
	}
}

// validateBackground guards the documented constraint that transparent
// backgrounds require a format that supports transparency (png or webp).
func validateBackground(req imagegen.Request) error {
	if req.Background != "transparent" {
		return nil
	}
	switch normalizeOutputFormat(req.OutputFormat) {
	case "png", "webp":
		return nil
	default:
		return fmt.Errorf("transparent background requires png or webp output, got %q", req.OutputFormat)
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

func (c *Client) postJSON(ctx context.Context, path string, body generationRequest) ([]imagegen.Result, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: OpenAI request body:\n%s\n", string(jsonData))
	}

	return c.do(ctx, http.MethodPost, c.baseURL+path, "application/json", jsonData)
}

func (c *Client) do(ctx context.Context, method, url, contentType string, body []byte) ([]imagegen.Result, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", contentType)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if os.Getenv("DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: OpenAI response status: %d\n", resp.StatusCode)
			fmt.Fprintf(os.Stderr, "DEBUG: OpenAI response body:\n%s\n", string(body))
		}

		if resp.StatusCode == http.StatusOK {
			return c.extractResults(body)
		}

		lastErr = c.handleError(resp.StatusCode, body)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		return nil, lastErr
	}

	return nil, lastErr
}

func (c *Client) extractResults(body []byte) ([]imagegen.Result, error) {
	var result imageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no image data found in response")
	}
	results := make([]imagegen.Result, 0, len(result.Data))
	for _, d := range result.Data {
		if d.B64JSON == "" {
			continue
		}
		// Strip C2PA / XMP / EXIF before handing the image to downstream code so
		// platforms like LinkedIn and Twitter don't auto-label posts as AI-generated
		// from embedded provenance manifests. SynthID is a pixel-domain watermark
		// and is unaffected.
		decoded, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("failed to decode image data: %w", err)
		}
		stripped, err := metadata.StripProvenance(decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to strip provenance metadata: %w", err)
		}
		results = append(results, imagegen.Result{
			ImageData: base64.StdEncoding.EncodeToString(stripped),
			Provider:  imagegen.ProviderOpenAI,
			Model:     c.model,
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no image data found in response")
	}
	return results, nil
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
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Background        string `json:"background,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	N                 int    `json:"n,omitempty"`
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
