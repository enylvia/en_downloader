package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	sizes := []int{16, 32, 48, 64, 128, 256}
	pngChunks := make([][]byte, len(sizes))

	for i, s := range sizes {
		img := drawIcon(s)
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		pngChunks[i] = buf.Bytes()
	}

	// 1. Save 256x256 PNG as icon.png
	_ = os.WriteFile("internal/web/dist/icon.png", pngChunks[len(sizes)-1], 0644)

	// 2. Build multi-resolution .ico file
	icoData := buildIco(sizes, pngChunks)
	_ = os.WriteFile("app.ico", icoData, 0644)
	_ = os.WriteFile("internal/web/dist/favicon.ico", icoData, 0644)
}

func drawIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 256.0
	center := float64(size) / 2.0
	cornerR := 46.0 * scale
	pad := 10.0 * scale

	// Draw rounded square background with smooth antialiased border
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x), float64(y)

			// Signed distance to rounded rectangle
			dx := math.Max(math.Abs(fx-center)-(center-pad-cornerR), 0)
			dy := math.Max(math.Abs(fy-center)-(center-pad-cornerR), 0)
			dist := math.Hypot(dx, dy) - cornerR

			if dist > 1.5 {
				continue // outside
			}

			alpha := 1.0
			if dist > -0.5 {
				alpha = 1.0 - (dist + 0.5) / 2.0
			}

			// Neon Cyan Border check
			isBorder := dist >= -3.5*scale && dist <= 0.5
			var c color.RGBA
			if isBorder {
				// Cyan neon border (#06b6d4)
				c = color.RGBA{R: 6, G: 182, B: 212, A: uint8(alpha * 255)}
			} else {
				// Deep gradient background (#0d1527 to #060a14)
				grad := float64(y) / float64(size)
				r := uint8((1.0 - grad)*13 + grad*6)
				g := uint8((1.0 - grad)*21 + grad*10)
				b := uint8((1.0 - grad)*39 + grad*20)
				c = color.RGBA{R: r, G: g, B: b, A: uint8(alpha * 255)}
			}
			img.Set(x, y, c)
		}
	}

	// Helper to draw thick polygon line segment with round caps
	drawThickLine := func(x1, y1, x2, y2, thickness float64, col color.RGBA) {
		dx := x2 - x1
		dy := y2 - y1
		length := math.Hypot(dx, dy)
		if length == 0 {
			return
		}
		halfThick := thickness / 2.0

		minX := int(math.Floor(math.Min(x1, x2) - thickness))
		maxX := int(math.Ceil(math.Max(x1, x2) + thickness))
		minY := int(math.Floor(math.Min(y1, y2) - thickness))
		maxY := int(math.Ceil(math.Max(y1, y2) + thickness))

		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				if x < 0 || x >= size || y < 0 || y >= size {
					continue
				}
				px, py := float64(x), float64(y)
				// Project point to line segment
				t := ((px-x1)*dx + (py-y1)*dy) / (length * length)
				t = math.Max(0, math.Min(1, t))
				projX := x1 + t*dx
				projY := y1 + t*dy
				dist := math.Hypot(px-projX, py-projY)

				if dist <= halfThick {
					alpha := 1.0
					if dist > halfThick-1.0 {
						alpha = halfThick - dist
					}
					orig := img.RGBAAt(x, y)
					cOut := blend(orig, col, alpha)
					img.Set(x, y, cOut)
				}
			}
		}
	}

	// Layer 1: Top Chevron Wings
	th1 := 14.0 * scale
	drawThickLine(60*scale, 95*scale, 128*scale, 140*scale, th1, color.RGBA{R: 56, G: 189, B: 248, A: 255})  // #38bdf8
	drawThickLine(196*scale, 95*scale, 128*scale, 140*scale, th1, color.RGBA{R: 99, G: 102, B: 241, A: 255}) // #6366f1

	// Layer 2: Middle Speed Chevron
	th2 := 12.0 * scale
	drawThickLine(74*scale, 140*scale, 128*scale, 175*scale, th2, color.RGBA{R: 34, G: 211, B: 238, A: 255})  // #22d3ee
	drawThickLine(182*scale, 140*scale, 128*scale, 175*scale, th2, color.RGBA{R: 34, G: 211, B: 238, A: 255}) // #22d3ee

	// Layer 3: Bottom Energy Arrow Point
	th3 := 10.0 * scale
	drawThickLine(90*scale, 180*scale, 128*scale, 208*scale, th3, color.RGBA{R: 6, G: 182, B: 212, A: 255})  // #06b6d4
	drawThickLine(166*scale, 180*scale, 128*scale, 208*scale, th3, color.RGBA{R: 6, G: 182, B: 212, A: 255}) // #06b6d4

	return img
}

func blend(bg, fg color.RGBA, alpha float64) color.RGBA {
	a := float64(fg.A) / 255.0 * alpha
	r := uint8(float64(bg.R)*(1-a) + float64(fg.R)*a)
	g := uint8(float64(bg.G)*(1-a) + float64(fg.G)*a)
	b := uint8(float64(bg.B)*(1-a) + float64(fg.B)*a)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func buildIco(sizes []int, pngs [][]byte) []byte {
	var buf bytes.Buffer

	// ICO Header: 6 bytes
	binary.Write(&buf, binary.LittleEndian, uint16(0))           // Reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))           // Type 1: Icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(sizes))) // Count

	// Directory entries: 16 bytes per image
	offset := uint32(6 + 16*len(sizes))
	for i, s := range sizes {
		w := byte(s)
		if s == 256 {
			w = 0
		}
		buf.WriteByte(w)                                                 // Width
		buf.WriteByte(w)                                                 // Height
		buf.WriteByte(0)                                                 // Color count
		buf.WriteByte(0)                                                 // Reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))              // Planes
		binary.Write(&buf, binary.LittleEndian, uint16(32))             // BitCount
		binary.Write(&buf, binary.LittleEndian, uint32(len(pngs[i])))   // Size
		binary.Write(&buf, binary.LittleEndian, offset)                 // Offset
		offset += uint32(len(pngs[i]))
	}

	// Image payloads
	for _, p := range pngs {
		buf.Write(p)
	}

	return buf.Bytes()
}
