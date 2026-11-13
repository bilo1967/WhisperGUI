package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveManagedModelsKeepsUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	items := map[string]string{
		"ggml-small.bin": "weights",
		"ggml-small.bin.meta.json": "{}",
		"ggml-silero-v6.2.0.bin": "vad",
		"personal.txt": "keep me",
	}
	for name, contents := range items {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil { t.Fatal(err) }
	}
	n, err := removeManagedModels(dir)
	if err != nil { t.Fatal(err) }
	if n != 3 { t.Fatalf("removed %d, expected 3", n) }
	if _, err := os.Stat(filepath.Join(dir, "personal.txt")); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestRemoveApplicationConfigKeepsNeighbor(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir,"config.yaml")
	other := filepath.Join(dir,"other.txt")
	if err := os.WriteFile(config, []byte("settings"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(other, []byte("keep"), 0600); err != nil { t.Fatal(err) }
	if err := removeApplicationConfig(config); err != nil { t.Fatal(err) }
	if _, err := os.Stat(config); !os.IsNotExist(err) { t.Fatalf("config remains: %v", err) }
	if _, err := os.Stat(other); err != nil { t.Fatal(err) }
}
