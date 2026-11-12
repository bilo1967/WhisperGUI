package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type timedWord struct {
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Duration int64  `json:"duration"`
	Text     string `json:"text"`
}
type wordSegment struct {
	Start int64       `json:"start"`
	End   int64       `json:"end"`
	Words []timedWord `json:"words"`
}
type wordDocument struct {
	Language string        `json:"language"`
	TimeUnit string        `json:"time_unit"`
	Segments []wordSegment `json:"segments"`
}

type tokenJSON struct {
	Text    string `json:"text"`
	Offsets struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"offsets"`
	ID int `json:"id"`
}

// makeWordOutputs converts the full token JSON into a per-word JSON and CSV.
// Text is reconstructed from token fragments, preserving punctuation and apostrophes.
// The millisecond interval is the span from the first to last token in each word.
func makeWordOutputs(src, jsonDst, csvDst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var original struct {
		Result struct {
			Language string `json:"language"`
		} `json:"result"`
		Transcription []struct {
			Offsets struct {
				From int64 `json:"from"`
				To   int64 `json:"to"`
			} `json:"offsets"`
			Tokens []tokenJSON `json:"tokens"`
		} `json:"transcription"`
	}
	if err = json.Unmarshal(data, &original); err != nil {
		return fmt.Errorf("parse token JSON: %w", err)
	}
	if len(original.Transcription) == 0 {
		return fmt.Errorf("token JSON contains no transcription segments")
	}
	doc := wordDocument{Language: original.Result.Language, TimeUnit: "milliseconds", Segments: make([]wordSegment, 0, len(original.Transcription))}
	var records [][]string
	records = append(records, []string{"start", "end", "duration", "word"})
	total := 0
	for _, segment := range original.Transcription {
		ws := aggregateWords(segment.Tokens)
		doc.Segments = append(doc.Segments, wordSegment{Start: segment.Offsets.From, End: segment.Offsets.To, Words: ws})
		for _, w := range ws {
			records = append(records, []string{csvSeconds(w.Start), csvSeconds(w.End), csvSeconds(w.Duration), w.Text})
			total++
		}
	}
	if total == 0 {
		return fmt.Errorf("token JSON contains no words")
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	// Write temporary files and rename only after their contents are complete.
	if err := writeAtomic(jsonDst, func(f *os.File) error { _, err := f.Write(encoded); return err }); err != nil {
		return err
	}
	if err := writeAtomic(csvDst, func(f *os.File) error {
		// UTF-8 BOM helps Excel on Windows recognise non-ASCII languages.
		if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return err
		}
		c := csv.NewWriter(f)
		if err := c.WriteAll(records); err != nil {
			return err
		}
		return c.Error()
	}); err != nil {
		return err
	}
	return nil
}

func csvSeconds(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }

func writeAtomic(path string, write func(*os.File) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".whispergui-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = write(f); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func aggregateWords(tokens []tokenJSON) []timedWord {
	words := make([]timedWord, 0)
	var pending *timedWord
	flush := func() {
		if pending != nil && pending.Text != "" {
			pending.Duration = pending.End - pending.Start
			words = append(words, *pending)
		}
		pending = nil
	}
	for _, t := range tokens {
		// Whisper special tokens are not speech; skip by token ID rather than
		// looking for a bracketed string in user-visible text.
		if t.ID >= 50257 || t.Text == "" {
			continue
		}
		// A leading space marks a new word in Whisper's BPE vocabulary.
		// Handle unusual embedded whitespace defensively as well.
		parts := strings.FieldsFunc(t.Text, unicode.IsSpace)
		if len(parts) == 0 {
			continue
		}
		leadingSpace := unicode.IsSpace([]rune(t.Text)[0])
		for i, part := range parts {
			if leadingSpace || i > 0 {
				flush()
			}
			// Punctuation with no whitespace belongs to the preceding word;
			// a leading apostrophe is treated as part of the same word too.
			if pending == nil {
				pending = &timedWord{Start: t.Offsets.From, End: t.Offsets.To, Text: part}
			} else {
				pending.Text += part
				if t.Offsets.To > pending.End {
					pending.End = t.Offsets.To
				}
			}
		}
	}
	flush()
	return words
}
