//go:build ignore

// gen-icon creates a multi-resolution Windows icon from the same simple
// terminal-prompt geometry as scripts/icon/cats-icon.svg. It uses only the Go
// standard library so release builders do not need a raster graphics package.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	output := flag.String("output", "Cats.ico", "output .ico path")
	flag.Parse()
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	images := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		var data bytes.Buffer
		if err := png.Encode(&data, drawIcon(size)); err != nil {
			panic(err)
		}
		images = append(images, data.Bytes())
	}
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, uint16(0))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(len(images)))
	offset := 6 + 16*len(images)
	for index, data := range images {
		size := sizes[index]
		width := byte(size)
		if size == 256 {
			width = 0
		}
		ico.WriteByte(width)
		ico.WriteByte(width)
		ico.WriteByte(0)
		ico.WriteByte(0)
		_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
		_ = binary.Write(&ico, binary.LittleEndian, uint16(32))
		_ = binary.Write(&ico, binary.LittleEndian, uint32(len(data)))
		_ = binary.Write(&ico, binary.LittleEndian, uint32(offset))
		offset += len(data)
	}
	for _, data := range images {
		ico.Write(data)
	}
	if err := os.WriteFile(*output, ico.Bytes(), 0o644); err != nil {
		panic(err)
	}
}

func drawIcon(size int) image.Image {
	const scale = 4
	dim := size * scale
	img := image.NewNRGBA(image.Rect(0, 0, dim, dim))
	for y := 0; y < dim; y++ {
		for x := 0; x < dim; x++ {
			nx, ny := float64(x)/float64(dim), float64(y)/float64(dim)
			if roundedRect(nx, ny, .094, .094, .906, .906, .184) {
				t := (ny - .094) / .812
				img.SetNRGBA(x, y, blend(color.NRGBA{42, 51, 70, 255}, color.NRGBA{18, 21, 28, 255}, t))
			}
		}
	}
	blue := color.NRGBA{91, 157, 255, 255}
	green := color.NRGBA{106, 196, 122, 255}
	stroke(img, .330, .379, .459, .500, .058, blue)
	stroke(img, .459, .500, .330, .621, .058, blue)
	fillRounded(img, .512, .592, .689, .646, .027, green)
	return downsample(img, size, scale)
}

func roundedRect(x, y, left, top, right, bottom, radius float64) bool {
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

func stroke(img *image.NRGBA, x1, y1, x2, y2, width float64, c color.NRGBA) {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			px, py := float64(x)/w, float64(y)/h
			dx, dy := x2-x1, y2-y1
			t := ((px-x1)*dx + (py-y1)*dy) / (dx*dx + dy*dy)
			t = math.Max(0, math.Min(1, t))
			sx, sy := x1+t*dx, y1+t*dy
			if math.Hypot(px-sx, py-sy) <= width/2 {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

func fillRounded(img *image.NRGBA, left, top, right, bottom, radius float64, c color.NRGBA) {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			if roundedRect(float64(x)/w, float64(y)/h, left, top, right, bottom, radius) {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

func downsample(src *image.NRGBA, size, scale int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					p := src.NRGBAAt(x*scale+sx, y*scale+sy)
					r += uint32(p.R)
					g += uint32(p.G)
					b += uint32(p.B)
					a += uint32(p.A)
				}
			}
			n := uint32(scale * scale)
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return dst
}

func blend(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t), A: 255,
	}
}
