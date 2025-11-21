package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_UsesSpecifiedModel(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedModel string
	}{
		{
			name:          "uses default model when not specified",
			model:         "",
			expectedModel: "gemini-3-pro-image-preview",
		},
		{
			name:          "uses nano-banana-pro when specified",
			model:         "nano-banana-pro-preview",
			expectedModel: "nano-banana-pro-preview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request URL
			var requestedURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedURL = r.URL.Path
				// Return a minimal valid response
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"candidates": [{
						"content": {
							"parts": [{
								"inlineData": {
									"mimeType": "image/png",
									"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
								}
							}]
						}
					}]
				}`))
			}))
			defer server.Close()

			// Create client with specified model
			client := &Client{
				apiKey:     "test-key",
				httpClient: &http.Client{},
				model:      tt.model,
				baseURL:    server.URL,
			}

			// Make a request
			_, err := client.GenerateContent("test prompt")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the correct model was used in the URL
			expectedPath := "/" + tt.expectedModel + ":generateContent"
			if requestedURL != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, requestedURL)
			}
		})
	}
}

func TestClient_UsesImageResolution(t *testing.T) {
	tests := []struct {
		name               string
		resolution         string
		expectedResolution string
	}{
		{
			name:               "uses 4K by default",
			resolution:         "",
			expectedResolution: "4K",
		},
		{
			name:               "uses 1K when specified",
			resolution:         "1K",
			expectedResolution: "1K",
		},
		{
			name:               "uses 2K when specified",
			resolution:         "2K",
			expectedResolution: "2K",
		},
		{
			name:               "uses 4K when specified",
			resolution:         "4K",
			expectedResolution: "4K",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request body
			var receivedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedBody, _ = io.ReadAll(r.Body)
				// Return a minimal valid response
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"candidates": [{
						"content": {
							"parts": [{
								"inlineData": {
									"mimeType": "image/png",
									"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
								}
							}]
						}
					}]
				}`))
			}))
			defer server.Close()

			// Create client
			client := &Client{
				apiKey:     "test-key",
				httpClient: &http.Client{},
				model:      "gemini-3-pro-image-preview",
				baseURL:    server.URL,
			}

			// Make a request with resolution
			_, err := client.GenerateContentWithResolution("test prompt", tt.resolution, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Parse the request body to verify resolution was set correctly
			var req GenerateRequest
			if err := json.Unmarshal(receivedBody, &req); err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}

			// Verify resolution in the request
			if req.GenerationConfig == nil || req.GenerationConfig.ImageConfig == nil {
				t.Fatal("expected ImageConfig to be set")
			}
			if req.GenerationConfig.ImageConfig.ImageSize != tt.expectedResolution {
				t.Errorf("expected resolution %q, got %q", tt.expectedResolution, req.GenerationConfig.ImageConfig.ImageSize)
			}
		})
	}
}

func TestFrugalClient_OmitsImageSize(t *testing.T) {
	tests := []struct {
		name               string
		model              string
		resolution         string
		expectImageSize    bool
		expectedResolution string
	}{
		{
			name:            "frugal model omits imageSize (fixed 1024px output)",
			model:           ModelNameFrugal,
			resolution:      "",
			expectImageSize: false,
			expectedResolution: "",
		},
		{
			name:               "pro model defaults to 4K when no resolution specified",
			model:              ModelName,
			resolution:         "",
			expectImageSize:    true,
			expectedResolution: "4K",
		},
		{
			name:               "pro model respects explicit 2K",
			model:              ModelName,
			resolution:         "2K",
			expectImageSize:    true,
			expectedResolution: "2K",
		},
		{
			name:               "pro model respects explicit 4K",
			model:              ModelName,
			resolution:         "4K",
			expectImageSize:    true,
			expectedResolution: "4K",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request body
			var receivedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedBody, _ = io.ReadAll(r.Body)
				// Return a minimal valid response
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"candidates": [{
						"content": {
							"parts": [{
								"inlineData": {
									"mimeType": "image/png",
									"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
								}
							}]
						}
					}]
				}`))
			}))
			defer server.Close()

			// Create client with specified model
			client := &Client{
				apiKey:     "test-key",
				httpClient: &http.Client{},
				model:      tt.model,
				baseURL:    server.URL,
			}

			// Make a request with or without resolution
			_, err := client.GenerateContentWithResolution("test prompt", tt.resolution, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Parse the request body to verify resolution was set correctly
			var req GenerateRequest
			if err := json.Unmarshal(receivedBody, &req); err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}

			// Verify imageSize handling in the request
			if req.GenerationConfig == nil || req.GenerationConfig.ImageConfig == nil {
				t.Fatal("expected ImageConfig to be set")
			}

			if tt.expectImageSize {
				if req.GenerationConfig.ImageConfig.ImageSize != tt.expectedResolution {
					t.Errorf("expected resolution %q, got %q", tt.expectedResolution, req.GenerationConfig.ImageConfig.ImageSize)
				}
			} else {
				// Frugal model should not have ImageSize set (fixed 1024px output)
				if req.GenerationConfig.ImageConfig.ImageSize != "" {
					t.Errorf("frugal model should not send imageSize parameter, but got %q", req.GenerationConfig.ImageConfig.ImageSize)
				}
			}
		})
	}
}
