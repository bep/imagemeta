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

		outFilename := filepath.Join(outDir, basePath+".json")
		if err := os.MkdirAll(filepath.Dir(outFilename), 0o755); err != nil {
			return err
		}

		if err := os.WriteFile(outFilename, buf.Bytes(), 0o644); err != nil {
			return err
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
