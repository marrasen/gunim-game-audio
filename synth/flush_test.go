package synth

import (
	"math"
	"testing"
)

// subnormal32 reports whether v is a subnormal number: under the least
// normal one, and not zero.
func subnormal32(v float32) bool { return v != 0 && math.Abs(float64(v)) < 0x1p-126 }

func TestTheFiltersHoldNoSubnormalsInTheSilenceAfterSound(t *testing.T) {
	var lp svf
	lp.setAt(800, 0.7, 48000)
	var one onePole
	one.setAt(300, 48000)
	var d distortion
	d.sr = 48000
	d.set(0.6, "amp")
	r := newReverbAt(48000)
	r.set(0.8, 0.7, 0.5, 0.02)
	const block = 256
	l, rr := make([]float32, block), make([]float32, block)
	play := func(blocks int, sound bool) {
		at := 0
		for range blocks {
			for i := range l {
				v := float32(0)
				if sound {
					v = float32(0.5 * math.Sin(2*math.Pi*220*float64(at)/48000))
				}
				l[i], rr[i] = v, v
				lp.step(v)
				one.lp(v)
				at++
			}
			d.process(l, rr)
			r.process(l, rr)
		}
	}
	play(48000/block, true)
	// Ten seconds of silence: left alone, the filters decay to
	// subnormals and stick there, many times slower to work.
	play(10*48000/block, false)
	check := func(what string, vs ...float32) {
		t.Helper()
		for _, v := range vs {
			if subnormal32(v) {
				t.Fatalf("after ten seconds of silence, %s holds %g", what, v)
			}
		}
	}
	check("the state-variable filter", lp.ic1, lp.ic2)
	check("the one-pole filter", one.z)
	for ch := range 2 {
		check("the amp's tightening", d.tight[ch].z)
		check("the amp's DC filter", d.dc[ch].z)
		check("the cabinet", d.cabHP[ch].ic1, d.cabHP[ch].ic2, d.mid[ch].ic1, d.mid[ch].ic2,
			d.cabLP[ch][0].ic1, d.cabLP[ch][0].ic2, d.cabLP[ch][1].ic1, d.cabLP[ch][1].ic2)
	}
	for k := range r.ap {
		check("the reverb's diffusion", r.ap[k].buf...)
	}
	check("the reverb's low cut", r.in.z)
}
