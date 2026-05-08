package imagegen

import "testing"

func TestOpenAISize(t *testing.T) {
	tests := []struct {
		name        string
		resolution  string
		aspectRatio string
		want        string
		wantErr     bool
	}{
		{name: "auto by default", want: "auto"},
		{name: "4K landscape", resolution: "4K", aspectRatio: "16:9", want: "3840x2160"},
		{name: "2K portrait", resolution: "2K", aspectRatio: "9:16", want: "1152x2048"},
		{name: "1K square", resolution: "1K", aspectRatio: "1:1", want: "1024x1024"},
		{name: "custom size passes through", resolution: "1280x720", want: "1280x720"},
		{name: "unsupported extreme OpenAI aspect ratio", resolution: "4K", aspectRatio: "8:1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAISize(tt.resolution, tt.aspectRatio)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFindClosestAspectRatio(t *testing.T) {
	got := FindClosestAspectRatio(3840, 2160)
	if got != "16:9" {
		t.Fatalf("expected 16:9, got %s", got)
	}
}
