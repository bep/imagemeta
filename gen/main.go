// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

//go:generate go run main.go
package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	outDir := "testdata_exiftool"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	base := "../testdata"
	keep := map[string]bool{}

	if err := filepath.Walk(filepath.Join(base, "images"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") || strings.HasSuffix(info.Name(), ".txt") {
			return nil
		}

		basePath := strings.TrimPrefix(path, base)

		var buf bytes.Buffer
		cmd := exec.Command("exiftool", path,
			"-json", "-n", "-g", "-e",
			"-x", "FileModifyDate",
			"-x", "FileAccessDate",
			"-x", "FileInodeChangeDate")
		cmd.Stdout = &buf
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return err
		}

		exiftoolOutFilename := filepath.Join(outDir, basePath+".json")
		if err := os.MkdirAll(filepath.Dir(exiftoolOutFilename), 0o755); err != nil {
			return err
		}

		if err := os.WriteFile(exiftoolOutFilename, buf.Bytes(), 0o644); err != nil {
			return err
		}
		keep[exiftoolOutFilename] = true

		imageConfigOutFilename := filepath.Join(outDir, basePath+".config.json")
		buf.Reset()
		cmd = exec.Command("identify", "-format", "{\"width\": %w, \"height\": %h}", path)
		cmd.Stdout = &buf
		var errorBuf bytes.Buffer
		cmd.Stderr = &errorBuf

		if err := cmd.Run(); err != nil {
			// identify fails for corrupt/truncated fixtures and for RAW files unless
			// ImageMagick is built with libraw. The RAW config goldens were produced
			// with libraw and can't be regenerated without it, so keep them.
			// readGoldenInfo handles a missing .config.json via os.IsNotExist.
			reason, _, _ := strings.Cut(strings.TrimSpace(errorBuf.String()), "\n")
			if _, err := os.Stat(imageConfigOutFilename); err == nil {
				keep[imageConfigOutFilename] = true
				log.Printf("warning: identify failed for %s: %s (keeping existing config.json)", path, reason)
			} else {
				log.Printf("warning: identify failed for %s: %s (no config.json)", path, reason)
			}
			return nil
		}

		if err := os.WriteFile(imageConfigOutFilename, buf.Bytes(), 0o644); err != nil {
			return err
		}
		keep[imageConfigOutFilename] = true

		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// Remove outputs for fixtures that no longer exist.
	if err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || keep[path] {
			return err
		}
		log.Printf("removing stale %s", path)
		return os.Remove(path)
	}); err != nil {
		log.Fatal(err)
	}
}
