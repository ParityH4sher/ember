package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
)

const (
	inputPath = "/input/photo.png"
	maxBytes  = 4 << 20
	maxPixels = 4 << 20
	side      = 128
)

// Read a bounded snapshot so the header check and decoder see the same bytes.
func thumbnail(r io.Reader, w io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxBytes {
		return errors.New("input exceeds byte limit")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if cfg.Height < 1 || cfg.Width < 1 || cfg.Width > maxPixels/cfg.Height {
		return errors.New("input exceeds pixel limit")
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	// Sample the centered square, preserving aspect ratio.
	n := min(cfg.Width, cfg.Height)
	left, top := (cfg.Width-n)/2, (cfg.Height-n)/2
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			dst.Set(x, y, src.At(left+(2*x+1)*n/(2*side), top+(2*y+1)*n/(2*side)))
		}
	}
	return png.Encode(w, dst)
}

func main() {
	f, err := os.Open(inputPath)
	if err == nil {
		defer f.Close()
		err = thumbnail(f, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "thumbnail:", err)
		os.Exit(1)
	}
}
