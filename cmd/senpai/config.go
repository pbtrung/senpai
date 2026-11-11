package main

import (
	"fmt"
	"io"
	"os"
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
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create %q: %v", dstPath, err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy %q to %q: %v", srcPath, dstPath, err)
	}
	return nil
}
