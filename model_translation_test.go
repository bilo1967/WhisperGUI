package main

import "testing"

func TestModelSupportsTranslation(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"ggml-large-v3.bin", true},
		{"ggml-large-v3-q5_0.bin", true},
		{"ggml-small.bin", true},
		{"ggml-small.en.bin", false},
		{"ggml-medium.en-q5_0.bin", false},
		{"ggml-large-v3-turbo.bin", false},
		{"ggml-large-v3-turbo-q5_0.bin", false},
		{"ggml-large-v3-turbo-q8_0.bin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelSupportsTranslation(tt.name); got != tt.want {
				t.Errorf("modelSupportsTranslation(%q) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
