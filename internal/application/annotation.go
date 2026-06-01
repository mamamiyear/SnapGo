package application

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// Annotation describes a user-drawn mark relative to the selected screenshot.
// Coordinates are logical pixels in the same coordinate system as the overlay.
type Annotation struct {
	Tool   string  `json:"tool"`
	Color  string  `json:"color"`
	Points []Point `json:"points"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ApplyAnnotations decodes a PNG, draws all annotations, and re-encodes it.
func ApplyAnnotations(pngBytes []byte, annotations []Annotation, scale float64) ([]byte, error) {
	if len(annotations) == 0 {
		return pngBytes, nil
	}
	if scale <= 0 {
		scale = 1
	}

	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode annotated png: %w", err)
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)

	for _, ann := range annotations {
		c, err := parseHexColor(ann.Color)
		if err != nil {
			c = color.RGBA{R: 59, G: 130, B: 246, A: 255}
		}
		width := int(math.Max(2, math.Round(3*scale)))
		switch ann.Tool {
		case "pen":
			drawPolyline(dst, ann.Points, scale, width, c)
		case "rect":
			drawRectOutline(dst, ann.Points, scale, width, c)
		case "ellipse":
			drawEllipseOutline(dst, ann.Points, scale, width, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode annotated png: %w", err)
	}
	return buf.Bytes(), nil
}

func parseHexColor(hex string) (color.RGBA, error) {
	value := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(value) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid color %q", hex)
	}
	n, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{
		R: uint8(n >> 16),
		G: uint8(n >> 8),
		B: uint8(n),
		A: 255,
	}, nil
}

func drawPolyline(img *image.RGBA, points []Point, scale float64, width int, c color.RGBA) {
	if len(points) == 1 {
		drawDot(img, scalePoint(points[0], scale), width, c)
		return
	}
	for i := 1; i < len(points); i++ {
		drawLine(img, scalePoint(points[i-1], scale), scalePoint(points[i], scale), width, c)
	}
}

func drawRectOutline(img *image.RGBA, points []Point, scale float64, width int, c color.RGBA) {
	if len(points) < 2 {
		return
	}
	a := scalePoint(points[0], scale)
	b := scalePoint(points[len(points)-1], scale)
	x1, x2 := ordered(a.X, b.X)
	y1, y2 := ordered(a.Y, b.Y)
	drawLine(img, Point{X: x1, Y: y1}, Point{X: x2, Y: y1}, width, c)
	drawLine(img, Point{X: x2, Y: y1}, Point{X: x2, Y: y2}, width, c)
	drawLine(img, Point{X: x2, Y: y2}, Point{X: x1, Y: y2}, width, c)
	drawLine(img, Point{X: x1, Y: y2}, Point{X: x1, Y: y1}, width, c)
}

func drawEllipseOutline(img *image.RGBA, points []Point, scale float64, width int, c color.RGBA) {
	if len(points) < 2 {
		return
	}
	a := scalePoint(points[0], scale)
	b := scalePoint(points[len(points)-1], scale)
	x1, x2 := ordered(a.X, b.X)
	y1, y2 := ordered(a.Y, b.Y)
	rx := (x2 - x1) / 2
	ry := (y2 - y1) / 2
	if rx <= 0 || ry <= 0 {
		return
	}
	cx := x1 + rx
	cy := y1 + ry
	steps := int(math.Max(48, math.Ceil(2*math.Pi*math.Max(rx, ry)/4)))
	prev := Point{
		X: cx + rx,
		Y: cy,
	}
	for i := 1; i <= steps; i++ {
		theta := 2 * math.Pi * float64(i) / float64(steps)
		next := Point{
			X: cx + rx*math.Cos(theta),
			Y: cy + ry*math.Sin(theta),
		}
		drawLine(img, prev, next, width, c)
		prev = next
	}
}

func drawLine(img *image.RGBA, a, b Point, width int, c color.RGBA) {
	dx := b.X - a.X
	dy := b.Y - a.Y
	steps := int(math.Max(math.Abs(dx), math.Abs(dy)))
	if steps == 0 {
		drawDot(img, a, width, c)
		return
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		drawDot(img, Point{X: a.X + dx*t, Y: a.Y + dy*t}, width, c)
	}
}

func drawDot(img *image.RGBA, p Point, width int, c color.RGBA) {
	radius := float64(width) / 2
	minX := int(math.Floor(p.X - radius))
	maxX := int(math.Ceil(p.X + radius))
	minY := int(math.Floor(p.Y - radius))
	maxY := int(math.Ceil(p.Y + radius))
	bounds := img.Bounds()
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !image.Pt(x, y).In(bounds) {
				continue
			}
			if math.Hypot(float64(x)-p.X, float64(y)-p.Y) <= radius {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func scalePoint(p Point, scale float64) Point {
	return Point{X: p.X * scale, Y: p.Y * scale}
}

func ordered(a, b float64) (float64, float64) {
	if a < b {
		return a, b
	}
	return b, a
}
