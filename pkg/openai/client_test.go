package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"imagemage/pkg/imagegen"
	"imagemage/pkg/openai"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestGenerateSendsImagesRequest(t *testing.T) {
	var gotPath string
	var gotAuth string
	var reqBody map[string]any
	httpClient := roundTripClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		return jsonResponse(http.StatusOK, `{"data":[{"b64_json":"`+tinyPNGBase64+`"}]}`), nil
	})

	client, err := openai.NewClient("test-image-model", openai.WithAPIKey("test-key"), openai.WithBaseURL("https://example.test/v1/images"), openai.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	result, err := client.Generate(context.Background(), imagegen.Request{
		Prompt:       "draw a test image",
		AspectRatio:  "16:9",
		Resolution:   "4K",
		Quality:      "high",
		OutputFormat: "webp",
	})
	if err != nil {
		t.Fatalf("unexpected generate error: %v", err)
	}

	if result.ImageData != tinyPNGBase64 {
		t.Fatalf("unexpected image data")
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected /v1/images/generations, got %s", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	assertField(t, reqBody, "model", "test-image-model")
	assertField(t, reqBody, "prompt", "draw a test image")
	assertField(t, reqBody, "size", "3840x2160")
	assertField(t, reqBody, "quality", "high")
	assertField(t, reqBody, "output_format", "webp")
}

func TestEditSendsMultipartImages(t *testing.T) {
	var gotPath string
	var gotPrompt string
	var gotImageCount int
	var gotImageContentTypes []string
	gotFields := map[string]string{}
	httpClient := roundTripClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("expected multipart request: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("multipart read error: %v", err)
			}
			switch part.FormName() {
			case "prompt":
				data, _ := io.ReadAll(part)
				gotPrompt = string(data)
			case "image":
				gotImageCount++
				gotImageContentTypes = append(gotImageContentTypes, part.Header.Get("Content-Type"))
				if !strings.HasSuffix(part.FileName(), ".png") && !strings.HasSuffix(part.FileName(), ".jpg") {
					t.Fatalf("unexpected filename, got %q", part.FileName())
				}
			default:
				data, _ := io.ReadAll(part)
				gotFields[part.FormName()] = string(data)
			}
		}
		return jsonResponse(http.StatusOK, `{"data":[{"b64_json":"`+tinyPNGBase64+`"}]}`), nil
	})

	client, err := openai.NewClient("test-image-model", openai.WithAPIKey("test-key"), openai.WithBaseURL("https://example.test/v1/images"), openai.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	_, err = client.Edit(context.Background(), imagegen.Request{
		Prompt:       "make it warmer",
		Quality:      "high",
		OutputFormat: "png",
		Images: []imagegen.ImageInput{
			{MimeType: "image/png", Base64: tinyPNGBase64},
			{MimeType: "image/jpeg", Base64: tinyPNGBase64},
		},
	})
	if err != nil {
		t.Fatalf("unexpected edit error: %v", err)
	}

	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected /v1/images/edits, got %s", gotPath)
	}
	if gotPrompt != "make it warmer" {
		t.Fatalf("expected prompt, got %q", gotPrompt)
	}
	if gotImageCount != 2 {
		t.Fatalf("expected 2 images, got %d", gotImageCount)
	}
	// Regression: OpenAI's edits endpoint rejects modern parameters like
	// `quality` when the image part Content-Type is application/octet-stream.
	// Each image part must carry its actual MIME type.
	wantCTs := []string{"image/png", "image/jpeg"}
	for i, ct := range gotImageContentTypes {
		if ct != wantCTs[i] {
			t.Fatalf("image %d: expected Content-Type %q, got %q", i, wantCTs[i], ct)
		}
	}
	if gotFields["quality"] != "high" {
		t.Fatalf("expected quality=high, got %q", gotFields["quality"])
	}
	if gotFields["output_format"] != "png" {
		t.Fatalf("expected output_format=png, got %q", gotFields["output_format"])
	}
	if gotFields["model"] != "test-image-model" {
		t.Fatalf("expected model=test-image-model, got %q", gotFields["model"])
	}
}

func TestGenerateRetriesWithFreshBody(t *testing.T) {
	var attempts int
	var prompts []string
	httpClient := roundTripClient(func(r *http.Request) (*http.Response, error) {
		attempts++
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode body on attempt %d: %v", attempts, err)
		}
		prompts = append(prompts, req["prompt"].(string))
		if attempts == 1 {
			return jsonResponse(http.StatusInternalServerError, `{"error":{"message":"temporary"}}`), nil
		}
		return jsonResponse(http.StatusOK, `{"data":[{"b64_json":"`+tinyPNGBase64+`"}]}`), nil
	})

	client, err := openai.NewClient("test-image-model", openai.WithAPIKey("test-key"), openai.WithBaseURL("https://example.test/v1/images"), openai.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	_, err = client.Generate(context.Background(), imagegen.Request{Prompt: "retry me"})
	if err != nil {
		t.Fatalf("unexpected generate error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(prompts) != 2 || prompts[0] != "retry me" || prompts[1] != "retry me" {
		t.Fatalf("request body was not preserved across retry: %#v", prompts)
	}
}

func assertField(t *testing.T, body map[string]any, field string, want string) {
	t.Helper()
	got, ok := body[field].(string)
	if !ok {
		t.Fatalf("expected string field %s in %#v", field, body)
	}
	if got != want {
		t.Fatalf("expected %s=%q, got %q", field, want, got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func roundTripClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
