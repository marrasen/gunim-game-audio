package synth

import (
	"math"
	"testing"
)

// circuitNames are the drums of the kit modelled on drum machines' circuits.
var circuitNames = []string{"tr8bd", "tr8sn", "tr8lt", "tr8mt", "tr8ht", "tr8lc", "tr8mc", "tr8hc", "tr8rs", "tr8cp",
	"tr8cl", "tr8ma", "tr8cb", "tr8hh", "tr8oh", "tr8cy", "tr9bd", "tr9sn", "tr9lt", "tr9mt", "tr9ht", "tr9rs", "tr9cp",
	"tr9hh", "tr9oh", "tr9cr", "tr9rd", "sdsbd", "sdssn", "sdslt", "sdsmt", "sdsht"}

func TestEveryCircuitDrumSoundsAndFallsSilent(t *testing.T) {
	for _, name := range circuitNames {
		x := renderKitHit(t, nil, name, 1, 8*rate)
		var peak float32
		for _, s := range x {
			if math.IsNaN(float64(s)) {
				t.Fatalf("%s makes NaN", name)
			}
			peak = max(peak, abs32(s))
		}
		if loud := rms(x[:rate/20]); loud < 0.02 || peak > 1.5 {
			t.Errorf("%s sounds at %.4f, its peak %.2f; want it heard, and not too loud", name, loud, peak)
		}
		if late := rms(x[7*rate:]); late > 1e-4 {
			t.Errorf("%s still sounds at %.5f seven seconds on; want it silent", name, late)
		}
	}
}

func TestACircuitDrumSoundsTheSameInAnyBlockSize(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range circuitNames {
		var want []float32
		for _, block := range []int{256, 1, 37} {
			hs := newHits(1)
			hs.play(c.kit[name], 0.8, 0, 0, 0, 0)
			l, r := make([]float32, rate/2), make([]float32, rate/2)
			for at := 0; at < len(l); at += block {
				end := min(at+block, len(l))
				hs.render(l[at:end], r[at:end], true)
			}
			if want == nil {
				want = l
				continue
			}
			for i := range l {
				if l[i] != want[i] {
					t.Fatalf("%s in blocks of %d frame %d is %v, in blocks of 256 %v", name, block, i, l[i], want[i])
				}
			}
		}
	}
}

func TestCircuitTomsAndCongasAreTunedLowToHigh(t *testing.T) {
	for _, set := range [][]string{{"tr8lt", "tr8mt", "tr8ht"}, {"tr8lc", "tr8mc", "tr8hc"}, {"tr9lt", "tr9mt", "tr9ht"},
		{"sdslt", "sdsmt", "sdsht"}} {
		last := math.MaxInt
		for _, name := range set {
			x := renderKitHit(t, nil, name, 0.8, rate)
			if n := swings(x[rate/20:], 4); n >= last {
				t.Errorf("%s swings four times in %d frames, the drum below it in %d; want it higher", name, n, last)
			} else {
				last = n
			}
		}
	}
}

func TestA909KickSweepsDownFast(t *testing.T) {
	x := renderKitHit(t, nil, "tr9bd", 1, rate)
	if early, late := swings(x, 2), swings(x[rate/10:], 2); early > late/2 {
		t.Errorf("the 909 kick swings twice in %d frames as it is hit and in %d later; want it sweeping down", early, late)
	}
}

func TestACircuitClosedHatChokesItsOpenHat(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"tr8hh", "tr8oh"}, {"tr9hh", "tr9oh"}} {
		ring := func(close bool) float64 {
			hs := newHits(2)
			hs.play(c.kit[pair[1]], 1, 0, 0, 0, 0)
			l, r := make([]float32, rate), make([]float32, rate)
			hs.render(l[:rate/20], r[:rate/20], true)
			if close {
				hs.play(c.kit[pair[0]], 0.01, 0, 0, 0, 0)
			}
			hs.render(l[rate/20:], r[rate/20:], true)
			return rms(l[rate*2/5 : rate/2])
		}
		if open, closed := ring(false), ring(true); closed > open/20 {
			t.Errorf("%s rings at %.4f, and at %.4f once %s is hit; want it choked", pair[1], open, closed, pair[0])
		}
	}
}

func Test909CymbalsAreGrainyAllTheWayDown(t *testing.T) {
	for _, c := range []struct {
		kind int
		name string
	}{{drTR909Hat, "tr9hh"}, {drTR909Crash, "tr9cr"}} {
		x := renderKitHit(t, nil, c.name, 1, rate)
		orig := cymbals[c.kind]
		smooth := orig
		smooth.crush = 0
		cymbals[c.kind] = smooth
		y := renderKitHit(t, nil, c.name, 1, rate)
		cymbals[c.kind] = orig
		grain := func(from, to int) float64 {
			d := make([]float32, to-from)
			for i := range d {
				d[i] = x[from+i] - y[from+i]
			}
			return rms(d) / rms(y[from:to])
		}
		end := rate / 10
		if c.kind == drTR909Crash {
			end = rate
		}
		if early, late := grain(0, end/4), grain(end*3/4, end); early < 0.003 || early > 0.1 || late < 0.003 || late > 0.1 {
			t.Errorf("%s is grainy by %.4f of its level as it is hit and %.4f later; want a 6-bit sample's grain throughout",
				c.name, early, late)
		}
	}
}

func TestGeneralMIDIsLatinNotesPlayThe808s(t *testing.T) {
	for key, want := range map[int]string{62: "tr8hc", 63: "tr8mc", 64: "tr8lc", 70: "tr8ma", 75: "tr8cl"} {
		if got := kitDrum(nil, key); got != want {
			t.Errorf("note %d plays %q; want %s", key, got, want)
		}
	}
}
