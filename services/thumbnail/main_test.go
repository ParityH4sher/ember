package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestCenterCrop(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	colors := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {A: 255}}
	for x, c := range colors {
		for y := range 2 {
			src.SetNRGBA(x, y, c)
		}
	}
	var in, out bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	if err := thumbnail(&in, &out); err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(&out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 128, 128) ||
		color.NRGBAModel.Convert(got.At(0, 0)) != colors[1] ||
		color.NRGBAModel.Convert(got.At(127, 127)) != colors[2] {
		t.Fatal("expected the middle two columns, scaled to 128x128")
	}
}

func TestRejectInputs(t *testing.T) {
	var oversized bytes.Buffer
	if err := png.Encode(&oversized, image.NewGray(image.Rect(0, 0, 2049, 2048))); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte("not a PNG"), make([]byte, maxBytes+1), oversized.Bytes()} {
		var out bytes.Buffer
		if err := thumbnail(bytes.NewReader(data), &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid input produced output or succeeded")
		}
	}
}
