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
	os.RemoveAll(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	base := "../testdata"

	if err := filepath.Walk(filepath.Join(base, "images"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
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

		buf.Reset()
		cmd = exec.Command("identify", "-format", "{\"width\": %w, \"height\": %h}", path)
		cmd.Stdout = &buf
		var errorBuf bytes.Buffer
		cmd.Stderr = &errorBuf

		if err := cmd.Run(); err != nil {
			if strings.Contains(errorBuf.String(), "identify:") {
				// We have some corrupt images.
				return nil
			}
			return err
		}

		imageConfigOutFilename := filepath.Join(outDir, basePath+".config.json")
		if err := os.MkdirAll(filepath.Dir(imageConfigOutFilename), 0o755); err != nil {
			return err
		}

		configBytes := buf.Bytes()

		if err := os.WriteFile(imageConfigOutFilename, configBytes, 0o644); err != nil {
			return err
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
