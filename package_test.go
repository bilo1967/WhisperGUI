package main

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageResults(t *testing.T) {
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "sample.txt"), filepath.Join(dir, "sample.words.csv")}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("sample transcription"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(dir, "sample.whisper.zip")
	warnings, err := packageResults(context.Background(), output, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != len(files) {
		t.Fatalf("got %d entries", len(zr.File))
	}
	for _, f := range files {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("source file remains: %s", f)
		}
	}
}
func TestPackageResultsRetainsSourcesOnFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "sample.whisper.zip")
	if err := os.WriteFile(output, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := packageResults(context.Background(), output, []string{file}); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("source removed: %v", err)
	}
}
func TestPackageResultsCancelRetainsSources(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := packageResults(ctx, filepath.Join(dir, "sample.whisper.zip"), []string{file}); err == nil {
		t.Fatal("expected cancellation")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("source removed: %v", err)
	}
}
