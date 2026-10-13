package synth

import (
	"math"
	"slices"
	"testing"
)

// unitNoise is a burst of white noise in frames of sound.
func unitNoise(frames, burst int) (l, r []float32) {
	l, r = make([]float32, frames), make([]float32, frames)
	g := newRand(9)
	for i := range burst {
		l[i], r[i] = float32(g.float()*0.6-0.3), float32(g.float()*0.6-0.3)
	}
	return l, r
}

// runBoth runs the same sound through a and b, a block at a time, and
// fails t where they differ, or make silence.
func runBoth(t *testing.T, what string, a, b func(l, r []float32)) {
	t.Helper()
	l, r := unitNoise(48000, 6000)
	l2, r2 := slices.Clone(l), slices.Clone(r)
	for at := 0; at < len(l); at += 300 {
		end := min(at+300, len(l))
		a(l[at:end], r[at:end])
		b(l2[at:end], r2[at:end])
	}
	loud := float32(0)
	for i := range l {
		if l[i] != l2[i] || r[i] != r2[i] {
			t.Fatalf("%s: frame %d is %g, %g, where a song's is %g, %g", what, i, l[i], r[i], l2[i], r2[i])
		}
		loud = max(loud, abs32(l[i]))
	}
	if loud < 0.01 {
		t.Fatalf("%s: silence", what)
	}
}

func TestTheUnitsSoundAsASongsEffects(t *testing.T) {
	rv := NewReverbUnit(Rate)
	rv.Set(1.1, 3, 0.3, 0.04)
	song := newReverb()
	song.set(1.1, 3, 0.3, 0.04)
	runBoth(t, "the reverb", rv.Process, song.process)

	dl := NewDelayUnit(Rate, 2, 0)
	dl.SetBeats(0.75, 133, 0.5, 0.4)
	sd := newDelay()
	sd.set(0.75, 133, 0.5, 0.4)
	runBoth(t, "the delay", dl.Process, sd.process)

	for _, kind := range ChorusKinds {
		c := NewChorusUnit(Rate, "soft")
		c.Set(kind, 0.7)
		var in inserts
		in.set(&Track{Chorus: 0.7, ChorusType: kind})
		runBoth(t, "chorus "+kind, c.Process, in.process)
	}
	for _, kind := range DistortKinds {
		d := NewDistortionUnit(Rate)
		d.Set(0.6, kind)
		var in inserts
		in.set(&Track{Distort: 0.6, DistortType: kind})
		runBoth(t, "distortion "+kind, d.Process, in.process)
	}
}

// decayTime measures how long the reverb's sound takes to fall from 20
// dB to 50 dB under its peak, times two, in seconds: its decay, as a
// room's RT60 is measured.
func decayTime(rate, decay float64) float64 {
	rv := NewReverbUnit(rate)
	rv.Set(1, decay, 0.5, 0.01)
	n := int(rate * decay * 1.5)
	l, r := make([]float32, n), make([]float32, n)
	l[0], r[0] = 1, 1
	rv.Process(l, r)
	// The level, a 10 ms window at a time.
	win := int(rate / 100)
	var levels []float64
	for at := 0; at+win <= n; at += win {
		var e float64
		for _, v := range l[at : at+win] {
			e += float64(v * v)
		}
		levels = append(levels, 10*math.Log10(e/float64(win)+1e-30))
	}
	top := slices.Max(levels)
	from, to := -1, -1
	for i, v := range levels {
		if from < 0 && v < top-20 && i > slices.Index(levels, top) {
			from = i
		}
		if from >= 0 && to < 0 && v < top-50 {
			to = i
		}
	}
	return float64(to-from) * 0.01 * 2
}

func TestTheReverbDecaysAlikeAtAnyRate(t *testing.T) {
	want := decayTime(Rate, 2)
	if want < 1.6 || want > 2.4 {
		t.Fatalf("a decay of 2 s measures %.2f s at 48 kHz", want)
	}
	for _, rate := range []float64{44100, 96000, 192000} {
		if got := decayTime(rate, 2); math.Abs(got-want) > 0.15*want {
			t.Errorf("a decay of 2 s measures %.2f s at %g Hz, and %.2f s at 48 kHz", got, rate, want)
		}
	}
}

func TestThePhaserNotchesItsCorner(t *testing.T) {
	// Held still at its corner, 800 Hz, six stages turn a sine of 800
	// Hz half a cycle round, so mixed with the sine they cancel it.
	ph := NewPhaserUnit(Rate)
	ph.Set(0.5, 0, 0, 1)
	l, r := make([]float32, Rate), make([]float32, Rate)
	for i := range l {
		l[i] = float32(math.Sin(2 * math.Pi * 800 * float64(i) / Rate))
		r[i] = l[i]
	}
	ph.Process(l, r)
	var most float32
	for _, v := range l[Rate/2:] {
		most = max(most, abs32(v))
	}
	if most > 0.02 {
		t.Errorf("a sine at the corner comes out at %g of its level", most)
	}
	// Swept, it lets the sine through, and the sides sweep apart.
	ph = NewPhaserUnit(Rate)
	ph.Set(2, 1, 0.5, 1)
	l, r = unitNoise(Rate, Rate)
	ph.Process(l, r)
	same := 0
	for i := range l {
		if math.IsNaN(float64(l[i])) || abs32(l[i]) > 2 {
			t.Fatalf("frame %d is %g", i, l[i])
		}
		if l[i] == r[i] {
			same++
		}
	}
	if same > len(l)/2 {
		t.Errorf("%d of %d frames are alike left and right", same, len(l))
	}
}
