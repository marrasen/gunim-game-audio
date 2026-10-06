package calls

import (
	"math"
	"slices"
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

func TestTheTopStepsThroughAllAndTheOddHarmonics(t *testing.T) {
	var top Param
	for _, p := range models["beeps"].Params {
		if p.Name == "top" {
			top = p
		}
	}
	var got []float64
	for v := 0.0; ; {
		got = append(got, v)
		next := top.Next(v, 1)
		if next == v {
			break
		}
		v = next
	}
	want := []float64{0, 1, 3, 5, 7, 9, 11, 13, 15}
	if !slices.Equal(got, want) {
		t.Errorf("the top steps up through %v, not %v", got, want)
	}
	if v := top.Next(3, -1); v != 1 {
		t.Errorf("down from 3 the top steps to %v, not 1", v)
	}
	for v, want := range map[float64]float64{4: 5, 2: 3, 0.4: 0, 16: 15, 5: 5} {
		if got := top.Snap(v); got != want {
			t.Errorf("the top snaps %v to %v, not %v", v, got, want)
		}
	}
}
