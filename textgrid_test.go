package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextGrid(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "input.wav")
	raw := make([]byte, 44+32000)
	copy(raw[:4], "RIFF")
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	copy(raw[8:12], "WAVE")
	copy(raw[12:16], "fmt ")
	binary.LittleEndian.PutUint32(raw[16:20], 16)
	binary.LittleEndian.PutUint16(raw[20:22], 1)
	binary.LittleEndian.PutUint16(raw[22:24], 1)
	binary.LittleEndian.PutUint32(raw[24:28], 16000)
	binary.LittleEndian.PutUint32(raw[28:32], 32000)
	binary.LittleEndian.PutUint16(raw[32:34], 2)
	binary.LittleEndian.PutUint16(raw[34:36], 16)
	copy(raw[36:40], "data")
	binary.LittleEndian.PutUint32(raw[40:44], 32000)
	if err := os.WriteFile(wav, raw, 0600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "words.json")
	input := `{"language":"it","time_unit":"milliseconds","segments":[{"start":200,"end":750,"words":[{"start":200,"end":400,"text":"Ciao"},{"start":380,"end":750,"text":"\"mondo\""}]}]}`
	if err := os.WriteFile(src, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.TextGrid")
	if err := makeTextGrid(src, wav, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, test := range []string{"Object class = \"TextGrid\"", "name = \"Segments\"", "name = \"Words\"", "xmax = 1.000", "text = \"\"", "text = \"\"\"mondo\"\"\""} {
		if !strings.Contains(s, test) {
			t.Errorf("missing %q in %s", test, s)
		}
	}
	if got := strings.Count(s, "intervals: size = "); got != 2 {
		t.Fatalf("expected two tiers: %d", got)
	}
}
func TestTextGridGaps(t *testing.T) {
	got := fillTextGridGaps([]textGridInterval{{100, 300, "a"}, {250, 600, "b"}, {800, 900, "c"}}, 1000)
	if len(got) != 6 || got[2].Start != 300 || got[2].End != 600 || got[5].Start != 900 {
		t.Fatalf("bad intervals: %+v", got)
	}
}
