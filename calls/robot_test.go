package calls

import (
	"math"
	"testing"
)

// at returns the strength of frequency f in x, over its first n samples.
func at(x []float64, f float64, n int) float64 {
	var re, im float64
	for i := range min(n, len(x)) {
		a := 2 * math.Pi * f * float64(i) / rate
		re += x[i] * math.Cos(a)
		im += x[i] * math.Sin(a)
	}
	return math.Hypot(re, im)
}

// beepAt renders one 880 Hz beep at full wave, its harmonics up to top.
func beepAt(top float64) []float64 {
	m := models["beeps"]
	p := params{}
	for _, q := range m.Params {
		p[q.Name] = q.Def
	}
	p["pitch"], p["beeps"], p["length"], p["last"], p["wave"], p["top"] = 880, 1, 0.12, 1, 1, top
	return m.render(p, &rng{1})
}

func TestABeepsTopHarmonicCutsTheOnesAboveIt(t *testing.T) {
	n := int(0.1 * rate)
	five := beepAt(5)
	if third, seventh := at(five, 3*880, n), at(five, 7*880, n); seventh > 0.01*third {
		t.Errorf("with its top at 5, the 7th harmonic is %.1f against the 3rd's %.1f", seventh, third)
	}
	all := beepAt(0)
	if third, seventh := at(all, 3*880, n), at(all, 7*880, n); seventh < 0.2*third {
		t.Errorf("with no top, the 7th harmonic is %.1f against the 3rd's %.1f; want the square's", seventh, third)
	}
}
