package main

import (
	"github.com/marrasen/gunim/geom"
	"image/color"
	"math"
)

// A palette is how the window looks in a style: its colours, a colour
// for each channel, and how far it draws as a console's screen does,
// in fat pixels and scanlines.
type palette struct {
	// bgTop and bgBottom shade the window's background, glow lights it
	// from the middle, and panel, edge and ink draw its panels and text.
	bgTop, bgBottom, glow  color.NRGBA
	panel, edge, ink, dim  color.NRGBA
	accent, accent2, white color.NRGBA
	// ch are the channels' colours.
	ch [16]color.NRGBA
	// pixel is how large a fat pixel is, 0 for none; scan how strong the
	// scanlines are; round how round the corners are, from 0 to 1.
	pixel, scan, round float32
	// mono takes every colour drawn to the nearest of the Game Boy's four
	// greens, by its brightness, from 0 to 1.
	mono float32
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

// palettes are each style's palette.
var palettes = map[string]palette{
	// A concert hall at night: deep blue and violet, lit in gold.
	"gm": {
		bgTop: rgb(0x14122b), bgBottom: rgb(0x07070f), glow: rgb(0x3a2a6e),
		panel: rgb(0x1b1a33), edge: rgb(0x34325c), ink: rgb(0xeeeaff), dim: rgb(0x8d88b8),
		accent: rgb(0xffc35c), accent2: rgb(0xff6fae), white: rgb(0xffffff),
		ch: [16]color.NRGBA{rgb(0xffc35c), rgb(0x5ce1ff), rgb(0xff6fae), rgb(0x9b8cff), rgb(0x6ef2a3), rgb(0xff8a4c),
			rgb(0xe9f27a), rgb(0x4cc3ff), rgb(0xff5c7a), rgb(0xffffff), rgb(0xc58cff), rgb(0x5cffd8),
			rgb(0xffa8e0), rgb(0x8cffb0), rgb(0xffd98c), rgb(0x8cb4ff)},
		round: 1,
	},
	// The Commodore 64: its blue screen in its light blue border, and its
	// sixteen colours.
	"sid": {
		bgTop: rgb(0x40318d), bgBottom: rgb(0x2a1f66), glow: rgb(0x7869c4),
		panel: rgb(0x352879), edge: rgb(0x7869c4), ink: rgb(0xa59fef), dim: rgb(0x7869c4),
		accent: rgb(0xbfce72), accent2: rgb(0xb86962), white: rgb(0xffffff),
		ch: [16]color.NRGBA{rgb(0x9ae29b), rgb(0x67b6bd), rgb(0xb86962), rgb(0xbfce72), rgb(0xc7ffff), rgb(0xa057a3),
			rgb(0xd5df7c), rgb(0x8b5429), rgb(0xff9f9f), rgb(0xffffff), rgb(0x6abfc6), rgb(0xa1683c),
			rgb(0xcbd765), rgb(0xff8bff), rgb(0x9ae29b), rgb(0xadadad)},
		pixel: 4, scan: 0.22, round: 0.15,
	},
	// The NES: its black, and the bright colours of its palette.
	"nes": {
		bgTop: rgb(0x101010), bgBottom: rgb(0x000000), glow: rgb(0x24188c),
		panel: rgb(0x181818), edge: rgb(0x757575), ink: rgb(0xfcfcfc), dim: rgb(0xbcbcbc),
		accent: rgb(0xf83800), accent2: rgb(0x3cbcfc), white: rgb(0xfcfcfc),
		ch: [16]color.NRGBA{rgb(0xf83800), rgb(0x3cbcfc), rgb(0xb8f818), rgb(0xf8b800), rgb(0xf878f8), rgb(0x58d854),
			rgb(0x6888fc), rgb(0xfca044), rgb(0xe45c10), rgb(0xfcfcfc), rgb(0x00e8d8), rgb(0xd800cc),
			rgb(0xa4e4fc), rgb(0xf8d878), rgb(0xd8f878), rgb(0xf0d0b0)},
		pixel: 4, scan: 0.18, round: 0,
	},
	// The Game Boy: four greens and nothing else.
	"gb": {
		bgTop: rgb(0x9bbc0f), bgBottom: rgb(0x8bac0f), glow: rgb(0xb7d36b),
		panel: rgb(0x8bac0f), edge: rgb(0x306230), ink: rgb(0x0f380f), dim: rgb(0x306230),
		accent: rgb(0x0f380f), accent2: rgb(0x306230), white: rgb(0x0f380f),
		ch: [16]color.NRGBA{rgb(0x0f380f), rgb(0x306230), rgb(0x0f380f), rgb(0x306230), rgb(0x0f380f), rgb(0x306230),
			rgb(0x0f380f), rgb(0x306230), rgb(0x0f380f), rgb(0x0f380f), rgb(0x306230), rgb(0x0f380f),
			rgb(0x306230), rgb(0x0f380f), rgb(0x306230), rgb(0x0f380f)},
		pixel: 5, scan: 0.08, round: 0, mono: 1,
	},
	// A DOS game of 1992: its blue panels, and VGA's bright colours.
	"adlib": {
		bgTop: rgb(0x0000aa), bgBottom: rgb(0x000055), glow: rgb(0x5555ff),
		panel: rgb(0x000080), edge: rgb(0x55ffff), ink: rgb(0xffffff), dim: rgb(0xaaaaaa),
		accent: rgb(0xffff55), accent2: rgb(0xff55ff), white: rgb(0xffffff),
		ch: [16]color.NRGBA{rgb(0xffff55), rgb(0x55ffff), rgb(0xff55ff), rgb(0x55ff55), rgb(0xff5555), rgb(0x5555ff),
			rgb(0xffaa00), rgb(0xaaffaa), rgb(0xffaaff), rgb(0xffffff), rgb(0xaaaaff), rgb(0x55ffaa),
			rgb(0xffaa55), rgb(0xaa55ff), rgb(0x55aaff), rgb(0xffff99)},
		pixel: 2, scan: 0.12, round: 0.25,
	},
}

// mixPalette blends a toward b by t, from 0 to 1.
func mixPalette(a, b palette, t float32) palette {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	m := palette{
		bgTop: mix(a.bgTop, b.bgTop, t), bgBottom: mix(a.bgBottom, b.bgBottom, t), glow: mix(a.glow, b.glow, t),
		panel: mix(a.panel, b.panel, t), edge: mix(a.edge, b.edge, t), ink: mix(a.ink, b.ink, t), dim: mix(a.dim, b.dim, t),
		accent: mix(a.accent, b.accent, t), accent2: mix(a.accent2, b.accent2, t), white: mix(a.white, b.white, t),
		pixel: a.pixel + (b.pixel-a.pixel)*t, scan: a.scan + (b.scan-a.scan)*t, round: a.round + (b.round-a.round)*t,
		mono: a.mono + (b.mono-a.mono)*t,
	}
	for i := range m.ch {
		m.ch[i] = mix(a.ch[i], b.ch[i], t)
	}
	return m
}

// greens are the Game Boy's four shades, darkest first.
var greens = [4]color.NRGBA{rgb(0x0f380f), rgb(0x306230), rgb(0x8bac0f), rgb(0x9bbc0f)}

// tone takes c toward the nearest of the Game Boy's greens by the
// palette's mono.
func (pl palette) tone(c color.NRGBA) color.NRGBA {
	if pl.mono <= 0 {
		return c
	}
	y := 0.3*float32(c.R) + 0.59*float32(c.G) + 0.11*float32(c.B)
	g := greens[min(int(y/64), 3)]
	g.A = c.A
	return mix(c, g, pl.mono)
}

// mix blends colour a toward b by t.
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

// alpha returns c at alpha a, from 0 to 1, times its own.
func alpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(min(max(a, 0), 1) * float32(c.A))
	return c
}

// lighten blends c toward white by t.
func lighten(c color.NRGBA, t float32) color.NRGBA {
	return mix(c, color.NRGBA{0xff, 0xff, 0xff, c.A}, t)
}

// darken blends c toward black by t.
func darken(c color.NRGBA, t float32) color.NRGBA {
	return mix(c, color.NRGBA{0, 0, 0, c.A}, t)
}

// ease is a smooth step from 0 to 1.
func ease(x float32) float32 {
	x = min(max(x, 0), 1)
	return x * x * (3 - 2*x)
}

func sinf(x float64) float32 { return float32(math.Sin(x)) }
func cosf(x float64) float32 { return float32(math.Cos(x)) }

// snap puts v on the grid of fat pixels of size px, where there is one.
func snap(v, px float32) float32 {
	if px < 1.5 {
		return v
	}
	return float32(math.Round(float64(v/px))) * px
}

// xyxy returns the rectangle from x0, y0 to x1, y1.
func xyxy(x0, y0, x1, y1 float32) geom.Rect {
	return geom.Rect{Min: geom.Pt(x0, y0), Max: geom.Pt(x1, y1)}
}
