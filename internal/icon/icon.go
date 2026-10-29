// Package icon draws the tray icon and prepares the branded application icon.
package icon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

const size = 32

// PNG draws a round badge with a "merge" glyph (two lines converging into one): green when
// system-wide mode is on, grey when off.
func PNG(on bool) []byte {
	bg := color.NRGBA{113, 113, 122, 255}
	if on {
		bg = color.NRGBA{34, 197, 94, 255}
	}
	segs := [][4]float64{{9, 8, 16, 16}, {23, 8, 16, 16}, {16, 16, 16, 25}}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			disc := clamp01(15.5 - math.Hypot(px-16, py-16) + 0.5)
			if disc == 0 {
				continue
			}
			glyph := 0.0
			for _, s := range segs {
				glyph = math.Max(glyph, clamp01(1.4-distToSeg(px, py, s[0], s[1], s[2], s[3])+0.5))
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(float64(bg.R)*(1-glyph) + 255*glyph),
				G: uint8(float64(bg.G)*(1-glyph) + 255*glyph),
				B: uint8(float64(bg.B)*(1-glyph) + 255*glyph),
				A: uint8(disc * 255),
			})
		}
	}
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	return pngBuf.Bytes()
}

// ICO wraps the PNG in an .ico container, which is what Windows wants.
func ICO(on bool) []byte {
	p := PNG(on)
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, [3]uint16{0, 1, 1}) // ICONDIR: reserved, type=icon, count=1
	ico.Write([]byte{size, size, 0, 0})                         // width, height, colors, reserved
	binary.Write(&ico, binary.LittleEndian, [2]uint16{1, 32})   // planes, bit depth
	binary.Write(&ico, binary.LittleEndian, uint32(len(p)))
	binary.Write(&ico, binary.LittleEndian, uint32(22)) // image offset: 6 + 16
	ico.Write(p)
	return ico.Bytes()
}

// ICOFromPNG wraps a PNG application icon in an ICO container for Windows.
func ICOFromPNG(p []byte) ([]byte, error) {
	config, err := png.DecodeConfig(bytes.NewReader(p))
	if err != nil {
		return nil, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 256 || config.Height > 256 {
		return nil, fmt.Errorf("application icon dimensions must be between 1 and 256 pixels")
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, [3]uint16{0, 1, 1})
	width, height := byte(config.Width), byte(config.Height)
	if config.Width == 256 {
		width = 0
	}
	if config.Height == 256 {
		height = 0
	}
	ico.Write([]byte{width, height, 0, 0})
	binary.Write(&ico, binary.LittleEndian, [2]uint16{1, 32})
	binary.Write(&ico, binary.LittleEndian, uint32(len(p)))
	binary.Write(&ico, binary.LittleEndian, uint32(22))
	ico.Write(p)
	return ico.Bytes(), nil
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func distToSeg(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := clamp01(((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}
