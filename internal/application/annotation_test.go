package application

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

func TestApplyAnnotationsDrawsIntoPNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	draw.Draw(src, src.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode source: %v", err)
	}

	out, err := ApplyAnnotations(buf.Bytes(), []Annotation{
		{
			Tool:  "rect",
			Color: "#ef4444",
			Points: []Point{
				{X: 5, Y: 5},
				{X: 30, Y: 30},
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("apply annotations: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := color.RGBAModel.Convert(img.At(5, 5)).(color.RGBA); got.R != 0xef || got.G != 0x44 || got.B != 0x44 {
		t.Fatalf("expected annotated pixel at rectangle edge, got %#v", got)
	}
}

func TestApplyAnnotationsNoAnnotationsReturnsOriginalBytes(t *testing.T) {
	original := []byte("not decoded when no annotations")
	out, err := ApplyAnnotations(original, nil, 1)
	if err != nil {
		t.Fatalf("apply annotations: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("expected original bytes")
	}
}
