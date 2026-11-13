package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// packageResults makes a verified ZIP before removing any source file.
// On failure the source files are left untouched.
func packageResults(ctx context.Context, output string, files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no transcription results to archive")
	}
	dir := filepath.Dir(output)
	temp, err := os.CreateTemp(dir, ".whispergui-results-*.zip.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	archive := zip.NewWriter(temp)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			archive.Close()
			temp.Close()
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			archive.Close()
			temp.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() || filepath.Dir(path) != dir {
			archive.Close()
			temp.Close()
			return nil, fmt.Errorf("unexpected result file: %s", path)
		}
		entry, err := archive.Create(filepath.Base(path))
		if err != nil {
			archive.Close()
			temp.Close()
			return nil, err
		}
		src, err := os.Open(path)
		if err != nil {
			archive.Close()
			temp.Close()
			return nil, err
		}
		_, err = io.Copy(entry, &contextReader{ctx: ctx, src: src})
		closeErr := src.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			archive.Close()
			temp.Close()
			return nil, err
		}
	}
	if err = archive.Close(); err != nil {
		temp.Close()
		return nil, err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return nil, err
	}
	if err = temp.Close(); err != nil {
		return nil, err
	}
	// Verify all entries, including CRCs, before removing the original files.
	zr, err := zip.OpenReader(temp.Name())
	if err != nil {
		return nil, err
	}
	if len(zr.File) != len(files) {
		zr.Close()
		return nil, fmt.Errorf("archive entry count mismatch")
	}
	for _, f := range zr.File {
		r, openErr := f.Open()
		if openErr != nil {
			zr.Close()
			return nil, openErr
		}
		_, readErr := io.Copy(io.Discard, r)
		closeErr := r.Close()
		if readErr != nil {
			zr.Close()
			return nil, readErr
		}
		if closeErr != nil {
			zr.Close()
			return nil, closeErr
		}
	}
	if err = zr.Close(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	// A unique stem is reserved by the job; don't overwrite a pre-existing ZIP.
	if _, err = os.Stat(output); err == nil {
		return nil, fmt.Errorf("archive already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err = os.Rename(temp.Name(), output); err != nil {
		return nil, err
	}
	warnings := []string{}
	for _, path := range files {
		if err := os.Remove(path); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not remove %s: %v", filepath.Base(path), err))
		}
	}
	return warnings, nil
}

type contextReader struct {
	ctx context.Context
	src io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(b)
}
