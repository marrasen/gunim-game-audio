package synth_test

import (
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
)

// tweaked plays 4 bars of Notte di Neon's chorus, tweaked by tw, and
// returns its left and right channels.
func tweaked(t *testing.T, tw synth.Tweak) (l, r []float32) {
	t.Helper()
	s := songs(t)["notte-di-neon"].Clone()
	s.Mix.Tweak = tw
	p := synth.NewPlayer(s, 1)
	look := p.Look(0)
	// To the chorus, phrase 5.
	play(p, int(4*float64(look.PhraseBars)*look.BarFrames))
	x := play(p, int(4*look.BarFrames))
	l, r = make([]float32, len(x)/2), make([]float32, len(x)/2)
	for i := range l {
		l[i], r[i] = x[2*i], x[2*i+1]
		if math.IsNaN(float64(l[i])) || math.Abs(float64(l[i])) > 1 || math.Abs(float64(r[i])) > 1 {
			t.Fatalf("%+v: sample %d is %v, %v", tw, i, l[i], r[i])
		}
	}
	return l, r
}

// level returns how loud x is through a filter: below hz where low,
// else above it, in decibels.
func level(x []float32, hz float64, low bool) float64 {
	// Two one-pole filters, 12 dB an octave.
	a := 1 - math.Exp(-2*math.Pi*hz/48000)
	var z1, z2, sum float64
	for _, s := range x {
		z1 += a * (float64(s) - z1)
		z2 += a * (z1 - z2)
		y := z2
		if !low {
			y = float64(s) - z2
		}
		sum += y * y
	}
	return 10 * math.Log10(sum/float64(len(x))+1e-20)
}

func crest(x []float32) float64 {
	rms, peak, _ := stats(x)
	return 20 * math.Log10(peak/rms)
}

func TestTweaksTurnTheSongAsTheySay(t *testing.T) {
	if testing.Short() {
		t.Skip("plays the song again and again")
	}
	l0, r0 := tweaked(t, synth.Tweak{})
	turned := func(tw synth.Tweak) []float32 { l, _ := tweaked(t, tw); return l }

	// Tone: the top against the bottom moves its way.
	tilt := func(x []float32) float64 { return level(x, 3000, false) - level(x, 300, true) }
	if b, d := tilt(turned(synth.Tweak{Tone: 1})), tilt(turned(synth.Tweak{Tone: -1})); b < tilt(l0)+4 || d > tilt(l0)-4 {
		t.Errorf("the top over the bottom is %.1f dB, bright %.1f, dark %.1f; want 4 dB or more either way", tilt(l0), b, d)
	}
	// Bass: the lows rise and fall.
	if up, down := level(turned(synth.Tweak{Bass: 1}), 100, true), level(turned(synth.Tweak{Bass: -1}), 100, true); up < level(l0, 100, true)+3 || down > level(l0, 100, true)-3 {
		t.Errorf("the bass is %.1f dB, lifted %.1f, cut %.1f; want 3 dB or more either way", level(l0, 100, true), up, down)
	}
	// Width: mono at -1, the sides wider at 1.
	side := func(l, r []float32) float64 {
		d := make([]float32, len(l))
		for i := range d {
			d[i] = l[i] - r[i]
		}
		rms, _, _ := stats(d)
		return rms
	}
	if l, r := tweaked(t, synth.Tweak{Width: -1}); side(l, r) > 1e-4 {
		t.Errorf("at width -1 the sides are %.5f; want mono", side(l, r))
	}
	if l, r := tweaked(t, synth.Tweak{Width: 1}); side(l, r) < 1.5*side(l0, r0) {
		t.Errorf("at width 1 the sides are %.4f, against %.4f; want them wider", side(l, r), side(l0, r0))
	}
	// Punch and drive squeeze the peaks towards the rest.
	for _, tw := range []synth.Tweak{{Punch: 1}, {Drive: 1}} {
		if c := crest(turned(tw)); c > crest(l0)-1.5 {
			t.Errorf("%+v: the crest is %.1f dB, against %.1f; want it squeezed", tw, c, crest(l0))
		}
	}
	// Space: the rooms and echoes heard less and more; dry, the mix
	// loses their sound, and wet it gains it.
	dry, wet := turned(synth.Tweak{Space: -1}), turned(synth.Tweak{Space: 1})
	rms := func(x []float32) float64 { r, _, _ := stats(x); return r }
	if !(rms(dry) < rms(l0) && rms(wet) > rms(l0)) {
		t.Errorf("the mix is %.4f, dry %.4f, wet %.4f; want it quieter dry and louder wet", rms(l0), rms(dry), rms(wet))
	}
	// Lo-fi: the top gone, as an old radio's.
	top := func(x []float32) float64 { return spectrum(x, 6000, 24000) - spectrum(x, 500, 2000) }
	if lofi := top(turned(synth.Tweak{LoFi: 1})); lofi > top(l0)-15 {
		t.Errorf("above 6 kHz, against the mids, the lo-fi mix is %.1f dB, and the mix %.1f; want it far darker", lofi, top(l0))
	}
}

// spectrum returns how loud x is from lo to hi hertz, in decibels, from
// the spectra of windows of it.
func spectrum(x []float32, lo, hi float64) float64 {
	const n = 1024
	var sum float64
	for at := 0; at+n <= len(x); at += 8 * n {
		for k := int(lo * n / 48000); k < min(int(hi*n/48000), n/2); k++ {
			var re, im float64
			for i := range n {
				w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n)
				a := 2 * math.Pi * float64(k*i) / n
				re += float64(x[at+i]) * w * math.Cos(a)
				im -= float64(x[at+i]) * w * math.Sin(a)
			}
			sum += re*re + im*im
		}
	}
	return 10 * math.Log10(sum+1e-20)
}
