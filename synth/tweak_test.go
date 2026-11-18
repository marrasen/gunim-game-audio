package synth_test

import (
	"math"
	"testing"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

// tweakParts are the parts of Notte di Neon each tweak is heard over,
// each a bar like the chorus's: a kick and bass for the lows and the
// peaks, hats for the top and the sides, and strings for the room.
var tweakParts = map[string]string{
	"kick":    "bd ~ bd ~",
	"hats":    "[hh hh oh hh]*4",
	"bass":    "[b b']*8",
	"strings": "ch",
}

// tweaked plays Notte di Neon's tweakParts, tweaked by tw, for settle
// bars and then bars more, and returns the left and right channels of
// the bars after settling.
func tweaked(t *testing.T, tw synth.Tweak, settle, bars float64) (l, r []float32) {
	t.Helper()
	song, err := music.Song(music.NotteDiNeon)
	if err != nil {
		t.Fatal(err)
	}
	s := song.(*synth.Song).Clone()
	s.Mix.Tweak = tw
	s.Clap = nil
	var tracks []*synth.Track
	for _, tr := range s.Tracks {
		if pat, ok := tweakParts[tr.Name]; ok {
			tr.Pattern, tr.Bars = pat, 1
			tracks = append(tracks, tr)
		}
	}
	if len(tracks) != len(tweakParts) {
		t.Fatalf("found %d of the %d parts", len(tracks), len(tweakParts))
	}
	s.Tracks = tracks
	p := synth.NewPlayer(s, 1)
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	bar := p.Look(0).BarFrames
	play(p, int(settle*bar))
	x := play(p, int(bars*bar))
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

// TestTweaksTurnTheSongAsTheySay turns each knob once, to one end, over
// half a bar. Tone and Bass run the same sum either way, so one end
// tells. Space runs a sum of its own each way, and Width goes mono at
// one end, so those turn to both.
func TestTweaksTurnTheSongAsTheySay(t *testing.T) {
	short := func(tw synth.Tweak) (l, r []float32) { return tweaked(t, tw, 0.25, 0.5) }
	l0, r0 := short(synth.Tweak{})
	turned := func(tw synth.Tweak) []float32 { l, _ := short(tw); return l }

	// Tone: the top against the bottom moves its way.
	tilt := func(x []float32) float64 { return level(x, 3000, false) - level(x, 300, true) }
	if b := tilt(turned(synth.Tweak{Tone: 1})); b < tilt(l0)+4 {
		t.Errorf("the top over the bottom is %.1f dB, and bright %.1f; want 4 dB more", tilt(l0), b)
	}
	// Bass: the lows rise.
	if up := level(turned(synth.Tweak{Bass: 1}), 100, true); up < level(l0, 100, true)+3 {
		t.Errorf("the bass is %.1f dB, and lifted %.1f; want 3 dB more", level(l0, 100, true), up)
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
	if side(l0, r0) < 0.01 {
		t.Fatalf("the mix's sides are %.5f; want some to narrow", side(l0, r0))
	}
	if l, r := short(synth.Tweak{Width: -1}); side(l, r) > 1e-4 {
		t.Errorf("at width -1 the sides are %.5f; want mono", side(l, r))
	}
	if l, r := short(synth.Tweak{Width: 1}); side(l, r) < 1.5*side(l0, r0) {
		t.Errorf("at width 1 the sides are %.4f, against %.4f; want them wider", side(l, r), side(l0, r0))
	}
	// Drive squeezes the peaks towards the rest.
	if c := crest(turned(synth.Tweak{Drive: 1})); c > crest(l0)-1.5 {
		t.Errorf("driven, the crest is %.1f dB, against %.1f; want it squeezed", c, crest(l0))
	}
	// Punch does too, once its make-up has caught up, a bar in.
	p0, _ := tweaked(t, synth.Tweak{}, 1, 2)
	if p, _ := tweaked(t, synth.Tweak{Punch: 1}, 1, 2); crest(p) > crest(p0)-1.5 {
		t.Errorf("punched, the crest is %.1f dB, against %.1f; want it squeezed", crest(p), crest(p0))
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
	for at := 0; at+n <= len(x); at += 16 * n {
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
