package main

import "testing"

func TestDTWPreset(t *testing.T) {
	tests := map[string]string{
		"ggml-tiny.bin":                "tiny",
		"ggml-base.en.bin":             "base.en",
		"ggml-small-q5_0.bin":          "small",
		"ggml-medium.en.bin":           "medium.en",
		"ggml-large-v2.bin":            "large.v2",
		"ggml-large-v3-turbo-q5_0.bin": "large.v3.turbo",
		"ggml-small.en-tdrz.bin":       "",
	}
	for filename, want := range tests {
		got, ok := dtwPreset(filename)
		if got != want || ok != (want != "") {
			t.Errorf("dtwPreset(%q) = %q, %v; expected %q", filename, got, ok, want)
		}
	}
}
