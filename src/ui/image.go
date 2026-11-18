package ui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"github.com/disintegration/imaging"
)

func exifOrientation(r io.Reader) int {
	// Inspired by: https://github.com/disintegration/imageorient

	const (
		markerSOI      = 0xffd8
		markerAPP1     = 0xffe1
		exifHeader     = 0x45786966
		byteOrderBE    = 0x4d4d
		byteOrderLE    = 0x4949
		orientationTag = 0x0112
	)

	// Check if JPEG SOI marker is present.
	var soi uint16
	if err := binary.Read(r, binary.BigEndian, &soi); err != nil {
		return 0
	}
	if soi != markerSOI {
		return 0 // Missing JPEG SOI marker.
	}

	// Find JPEG APP1 marker.
	for {
		var marker, size uint16
		if err := binary.Read(r, binary.BigEndian, &marker); err != nil {
			return 0
		}
		if err := binary.Read(r, binary.BigEndian, &size); err != nil {
			return 0
		}
		if marker>>8 != 0xff {
			return 0 // Invalid JPEG marker.
		}
		if marker == markerAPP1 {
			break
		}
		if size < 2 {
			return 0 // Invalid block size.
		}
		if _, err := io.CopyN(io.Discard, r, int64(size-2)); err != nil {
			return 0
		}
	}

	// Check if EXIF header is present.
	var header uint32
	if err := binary.Read(r, binary.BigEndian, &header); err != nil {
		return 0
	}
	if header != exifHeader {
		return 0
	}
	if _, err := io.CopyN(io.Discard, r, 2); err != nil {
		return 0
	}

	// Read byte order information.
	var (
		byteOrderTag uint16
		byteOrder    binary.ByteOrder
	)
	if err := binary.Read(r, binary.BigEndian, &byteOrderTag); err != nil {
		return 0
	}
	switch byteOrderTag {
	case byteOrderBE:
		byteOrder = binary.BigEndian
	case byteOrderLE:
		byteOrder = binary.LittleEndian
	default:
		return 0 // Invalid byte order flag.
	}
	if _, err := io.CopyN(io.Discard, r, 2); err != nil {
		return 0
	}

	// Skip the EXIF offset.
	var offset uint32
	if err := binary.Read(r, byteOrder, &offset); err != nil {
		return 0
	}
	if offset < 8 {
		return 0 // Invalid offset value.
	}
	if _, err := io.CopyN(io.Discard, r, int64(offset-8)); err != nil {
		return 0
	}

	// Read the number of tags.
	var numTags uint16
	if err := binary.Read(r, byteOrder, &numTags); err != nil {
		return 0
	}

	// Find the orientation tag.
	for i := 0; i < int(numTags); i++ {
		var tag uint16
		if err := binary.Read(r, byteOrder, &tag); err != nil {
			return 0
		}
		if tag != orientationTag {
			if _, err := io.CopyN(io.Discard, r, 10); err != nil {
				return 0
			}
			continue
		}
		if _, err := io.CopyN(io.Discard, r, 6); err != nil {
			return 0
		}
		var val uint16
		if err := binary.Read(r, byteOrder, &val); err != nil {
			return 0
		}
		if val < 1 || val > 8 {
			return 0 // Invalid tag value.
		}
		return int(val)
	}
	return 0 // Missing orientation tag.
}

func (ui *UI) ImageReady() bool {
	if ui.image == nil {
		return false
	}
	iw, ih := ui.image.CellSize()
	return iw != 0 || ih != 0
}

func (ui *UI) ShowImage(img image.Image) bool {
	if img == nil {
		if ui.image != nil {
			ui.image.Destroy()
			ui.image = nil
		}
		return true
	}

	// Terminal dimensions belong to the UI goroutine; image decoding runs in
	// the background and must not inspect them while a resize is in progress.
	img = ui.maybeResizeImage(img, 0)
	vi, err := ui.vx.NewImage(img)
	if err != nil {
		return false
	}
	w, h := ui.vx.window.Size()
	w = w * 9 / 10
	h = h * 9 / 10
	vi.Resize(w, h)
	ui.image = vi
	return true
}

func (ui *UI) DecodeImage(r io.Reader) (image.Image, string, error) {
	const maxImageBytes = 20 << 20
	const maxImagePixels = 32 << 20
	data, err := io.ReadAll(io.LimitReader(r, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("image exceeds %d byte limit", maxImageBytes)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, format, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImagePixels/cfg.Height {
		return nil, format, fmt.Errorf("image dimensions exceed %d pixel limit", maxImagePixels)
	}
	o := exifOrientation(bytes.NewReader(data))
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return img, format, err
	}

	switch o {
	case 3:
		img = imaging.Rotate180(img)
	case 6:
		img = imaging.Rotate270(img)
	case 8:
		img = imaging.Rotate90(img)
	}
	return img, format, nil
}

func (ui *UI) maybeResizeImage(img image.Image, exifOrientation int) image.Image {
	if ui.vx.xPixel <= 0 || ui.vx.yPixel <= 0 {
		return img
	}
	w, h := ui.vx.window.Size()
	w = w * 9 / 10
	h = h * 9 / 10
	if w <= 0 || h <= 0 {
		return img
	}
	wp := img.Bounds().Dx()
	hp := img.Bounds().Dy()
	switch exifOrientation {
	case 6, 8:
		wp, hp = hp, wp
	}
	cellPixW := max(1, ui.vx.xPixel/w)
	cellPixH := max(1, ui.vx.yPixel/h)
	columns := (wp + cellPixW - 1) / cellPixW
	lines := (hp + cellPixH - 1) / cellPixH
	if columns <= w && lines <= h {
		return img
	}
	sfX := float64(w) / float64(columns)
	sfY := float64(h) / float64(lines)
	sf := min(sfX, sfY)
	nwp := max(1, int(sf*float64(wp)))
	nhp := max(1, int(sf*float64(hp)))
	switch exifOrientation {
	case 6, 8:
		nwp, nhp = nhp, nwp
	}
	return imaging.Resize(img, nwp, nhp, imaging.NearestNeighbor)
}
