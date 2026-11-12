package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// wavDurationMS reads the duration of FFmpeg's 16 kHz mono PCM WAV output.
// Reading the data chunk avoids counting WAV headers as audio samples.
func wavDurationMS(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var header [12]byte
	if _, err = io.ReadFull(f, header[:]); err != nil {
		return 0, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return 0, fmt.Errorf("not a RIFF WAV file")
	}
	var bytesPerSecond uint32
	for {
		var chunk [8]byte
		if _, err = io.ReadFull(f, chunk[:]); err != nil {
			return 0, fmt.Errorf("WAV data chunk not found: %w", err)
		}
		size := binary.LittleEndian.Uint32(chunk[4:])
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 {
				return 0, fmt.Errorf("invalid WAV fmt chunk")
			}
			var fmtData [16]byte
			if _, err = io.ReadFull(f, fmtData[:]); err != nil {
				return 0, err
			}
			if binary.LittleEndian.Uint16(fmtData[:2]) != 1 {
				return 0, fmt.Errorf("WAV must be PCM")
			}
			bytesPerSecond = binary.LittleEndian.Uint32(fmtData[8:12])
			if bytesPerSecond == 0 {
				return 0, fmt.Errorf("invalid WAV byte rate")
			}
			if _, err = f.Seek(int64(size-16)+int64(size%2), io.SeekCurrent); err != nil {
				return 0, err
			}
		case "data":
			if bytesPerSecond == 0 {
				return 0, fmt.Errorf("WAV fmt chunk missing")
			}
			return (int64(size)*1000 + int64(bytesPerSecond)/2) / int64(bytesPerSecond), nil
		default:
			if _, err = f.Seek(int64(size)+int64(size%2), io.SeekCurrent); err != nil {
				return 0, err
			}
		}
	}
}

type textGridInterval struct {
	Start, End int64
	Text       string
}

func textGridEscape(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "�"), `"`, `""`)
}

// makeTextGrid writes a Praat long-text TextGrid with Segments and Words tiers.
// All timestamps are in milliseconds and gaps are explicitly empty intervals.
func makeTextGrid(wordsJSON, wavPath, dst string) error {
	data, err := os.ReadFile(wordsJSON)
	if err != nil {
		return err
	}
	var doc wordDocument
	if err = json.Unmarshal(data, &doc); err != nil {
		return err
	}
	duration, err := wavDurationMS(wavPath)
	if err != nil {
		return err
	}
	segments := make([]textGridInterval, 0, len(doc.Segments))
	words := make([]textGridInterval, 0)
	for _, s := range doc.Segments {
		var labels []string
		for _, w := range s.Words {
			if w.Text != "" {
				labels = append(labels, w.Text)
				words = append(words, textGridInterval{w.Start, w.End, w.Text})
			}
		}
		segments = append(segments, textGridInterval{s.Start, s.End, strings.Join(labels, " ")})
	}
	for _, w := range words {
		if w.End > duration {
			duration = w.End
		}
	}
	if duration <= 0 {
		return fmt.Errorf("TextGrid: invalid audio duration")
	}
	normalizedSegments := fillTextGridGaps(segments, duration)
	normalizedWords := fillTextGridGaps(words, duration)
	return writeAtomic(dst, func(f *os.File) error {
		write := func(format string, args ...any) error { _, e := fmt.Fprintf(f, format, args...); return e }
		if err := write("File type = \"ooTextFile\"\nObject class = \"TextGrid\"\n\nxmin = 0\nxmax = %s\ntiers? <exists>\nsize = 2\nitem []:\n", gridSeconds(duration)); err != nil {
			return err
		}
		for i, tier := range []struct {
			Name      string
			Intervals []textGridInterval
		}{{"Segments", normalizedSegments}, {"Words", normalizedWords}} {
			if err := write("    item [%d]:\n        class = \"IntervalTier\"\n        name = \"%s\"\n        xmin = 0\n        xmax = %s\n        intervals: size = %d\n", i+1, tier.Name, gridSeconds(duration), len(tier.Intervals)); err != nil {
				return err
			}
			for j, v := range tier.Intervals {
				if err := write("        intervals [%d]:\n            xmin = %s\n            xmax = %s\n            text = \"%s\"\n", j+1, gridSeconds(v.Start), gridSeconds(v.End), textGridEscape(v.Text)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func gridSeconds(ms int64) string { return fmt.Sprintf("%.3f", float64(ms)/1000) }

// Whisper DTW occasionally generates overlapping, zero-duration or out-of-order
// word offsets. Praat requires continuous, ordered, non-overlapping intervals.
// Clamp to the preceding boundary; preserve the original timestamps in JSON/CSV.
func fillTextGridGaps(input []textGridInterval, duration int64) []textGridInterval {
	out := make([]textGridInterval, 0, len(input)*2+1)
	cursor := int64(0)
	for _, v := range input {
		start, end := v.Start, v.End
		if start < cursor {
			start = cursor
		}
		if start > duration {
			start = duration
		}
		if end > duration {
			end = duration
		}
		if end <= start {
			continue
		}
		if start > cursor {
			out = append(out, textGridInterval{cursor, start, ""})
		}
		out = append(out, textGridInterval{start, end, v.Text})
		cursor = end
	}
	if cursor < duration {
		out = append(out, textGridInterval{cursor, duration, ""})
	}
	return out
}
