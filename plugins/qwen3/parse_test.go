package main

import (
	"testing"
)

func TestParseXMLOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    parsedOutput
		wantErr bool
	}{
		{
			name:  "full output with title and body",
			input: "<title>Hello</title><body>World</body>",
			want:  parsedOutput{Title: "Hello", Body: "World"},
		},
		{
			name:  "body before title",
			input: "<body>Content</body><title>My Title</title>",
			want:  parsedOutput{Title: "My Title", Body: "Content"},
		},
		{
			name:  "with whitespace around tags",
			input: "  <title>  Hello  </title>  <body>  World  </body>  ",
			want:  parsedOutput{Title: "Hello", Body: "World"},
		},
		{
			name:    "missing title",
			input:   "<body>Content</body>",
			wantErr: true,
		},
		{
			name:    "missing body",
			input:   "<title>Title</title>",
			wantErr: true,
		},
		{
			name:    "empty title",
			input:   "<title></title><body>Content</body>",
			wantErr: true,
		},
		{
			name:    "empty body",
			input:   "<title>Title</title><body></body>",
			wantErr: true,
		},
		{
			name:    "no tags at all",
			input:   "just plain text",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseXMLOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Title != tt.want.Title {
				t.Errorf("Title = %q, want %q", got.Title, tt.want.Title)
			}
			if got.Body != tt.want.Body {
				t.Errorf("Body = %q, want %q", got.Body, tt.want.Body)
			}
		})
	}
}

func TestParseChunkBodyOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "body only",
			input: "<body>Chunk content</body>",
			want:  "Chunk content",
		},
		{
			name:    "missing body",
			input:   "<title>Title</title>",
			wantErr: true,
		},
		{
			name:    "empty body",
			input:   "<body></body>",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChunkBodyOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Body != tt.want {
				t.Errorf("Body = %q, want %q", got.Body, tt.want)
			}
		})
	}
}