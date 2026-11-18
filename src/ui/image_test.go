package ui

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestDecodeImageRejectsOversizedDimensions(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := data.Bytes()
	binary.BigEndian.PutUint32(b[16:20], 100000)
	binary.BigEndian.PutUint32(b[20:24], 100000)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	var ui UI
	if _, _, err := ui.DecodeImage(bytes.NewReader(b)); err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("large dimensions not rejected before decode: %v", err)
	}
}

func TestDecodeImageRejectsOversizedBody(t *testing.T) {
	var ui UI
	if _, _, err := ui.DecodeImage(strings.NewReader(strings.Repeat("x", (20<<20)+1))); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("large encoded image accepted: %v", err)
	}
}
