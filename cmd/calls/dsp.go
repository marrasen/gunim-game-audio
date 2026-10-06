package main

import (
	"math"

	"github.com/marrasen/gunim/audio"
)

// biquad is a filter of two poles and two zeros, as RBJ's cookbook
// gives them.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *biquad) step(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1, f.y2, f.y1 = f.x1, x, f.y1, y
	return y
}

const rate = float64(audio.SampleRate)

func highPass(hz, q float64) biquad {
	w := 2 * math.Pi * hz / rate
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	return biquad{b0: (1 + cw) / 2 / a0, b1: -(1 + cw) / a0, b2: (1 + cw) / 2 / a0, a1: -2 * cw / a0, a2: (1 - al) / a0}
}

func lowPass(hz, q float64) biquad {
	w := 2 * math.Pi * hz / rate
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	return biquad{b0: (1 - cw) / 2 / a0, b1: (1 - cw) / a0, b2: (1 - cw) / 2 / a0, a1: -2 * cw / a0, a2: (1 - al) / a0}
}

// peaking lifts a band about hz by db.
func peaking(hz, q, db float64) biquad {
	w := 2 * math.Pi * hz / rate
	a := math.Pow(10, db/40)
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al/a
	return biquad{b0: (1 + al*a) / a0, b1: -2 * cw / a0, b2: (1 - al*a) / a0, a1: -2 * cw / a0, a2: (1 - al/a) / a0}
}
