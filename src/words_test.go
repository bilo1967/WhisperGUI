package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAggregateWords(t *testing.T) {
	tokens := []tokenJSON{{Text: " c", ID: 1}, {Text: "'", ID: 2}, {Text: "erano", ID: 3}, {Text: " bene", ID: 4}, {Text: "!", ID: 5}, {Text: "[_TT_650]", ID: 51014}}
	for i := range tokens {
		tokens[i].Offsets.From = int64(i * 100)
		tokens[i].Offsets.To = int64((i + 1) * 100)
	}
	result := aggregateWords(tokens)
	if len(result) != 2 || result[0].Text != "c'erano" || result[1].Text != "bene!" {
		t.Fatalf("unexpected words: %+v", result)
	}
}
func TestRealTokenSample(t *testing.T) {
	src := "/mnt/data/rs-20261025-220836-3-student@booth.n1.json"
	if _, err := os.Stat(src); err != nil {
		t.Skip("sample unavailable")
	}
	dir := t.TempDir()
	j, c := filepath.Join(dir, "test.words.json"), filepath.Join(dir, "test.words.csv")
	if err := makeWordOutputs(src, j, c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(j)
	if err != nil {
		t.Fatal(err)
	}
	var doc wordDocument
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Segments) == 0 || len(doc.Segments[0].Words) < 4 {
		t.Fatalf("bad document: %+v", doc)
	}
	if doc.Segments[0].Words[0].Text != "Vediamo" {
		t.Fatalf("first word %+v", doc.Segments[0].Words[0])
	}
	csvData, err := os.ReadFile(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvData), "0.270,1.530,1.260,Vediamo") {
		t.Fatalf("unexpected CSV: %.300s", csvData)
	}
	t.Logf("segments=%d first words=%+v", len(doc.Segments), doc.Segments[0].Words[:3])
}
