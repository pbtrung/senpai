package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func scanBool(v *bool) {
	for {
		var s string
		fmt.Scanln(&s)
		if s == "" {
			return
		}
		switch strings.ToLower(s) {
		case "y", "yes":
			*v = true
			return
		case "n", "no":
			*v = false
			return
		}
	}
}

func copyContents(dstPath string, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %q: %v", srcPath, err)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(dstPath), 0700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	// Configuration files can contain passwords. Publish a complete private
	// copy only after both writing and closing it have succeeded.
	dst, err := os.CreateTemp(filepath.Dir(dstPath), ".senpai-config-*")
	if err != nil {
		return fmt.Errorf("create %q: %v", dstPath, err)
	}
	defer os.Remove(dst.Name())
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %q to %q: %w", srcPath, dstPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %q: %w", dstPath, closeErr)
	}
	if err := os.Rename(dst.Name(), dstPath); err != nil {
		return fmt.Errorf("copy %q to %q: %v", srcPath, dstPath, err)
	}
	return nil
}
