package synth_test

import (
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
)

// sine is a patch of one sine, held at full level, let go over 50 ms.
var sine = &synth.Patch{Osc: []synth.Osc{{Wave: "sine"}}, Amp: synth.Env{Attack: 0.002, Sustain: 1, Release: 0.05}}

func instrument(t *testing.T, p *synth.Patch) *synth.Instrument {
	t.Helper()
	in, err := synth.NewInstrument(p)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// listen renders secs of in, in blocks of 256 frames, and returns its
// left channel.
func listen(in *synth.Instrument, secs float64) []float32 {
	n := int(secs * synth.Rate)
	out := make([]float32, 0, n)
	l, r := make([]float32, 256), make([]float32, 256)
	for len(out) < n {
		m := min(256, n-len(out))
		in.Render(l[:m], r[:m])
		out = append(out, l[:m]...)
	}
	return out
}

func rms(x []float32) float64 {
	var sum float64
	for _, s := range x {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(max(len(x), 1)))
}

// hz returns how high x sounds: the period, from 1 ms to 20, over which
// it best matches itself.
func hz(x []float32) float64 {
	best, at := math.Inf(-1), 0
	for lag := synth.Rate / 1000; lag <= synth.Rate/50; lag++ {
		var sum float64
		for i := lag; i < len(x); i++ {
			sum += float64(x[i]) * float64(x[i-lag])
		}
		sum /= float64(len(x) - lag)
		// The first peak near the best, so a period twice as long loses.
		if sum > best*1.02 {
			best, at = sum, lag
		}
	}
	return synth.Rate / float64(at)
}

func TestANoteHeldSoundsUntilItsNoteOff(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(7, 69, 1, 0)
	held := listen(in, 3)
	if got := rms(held[len(held)-synth.Rate/2:]); got < 0.1 {
		t.Fatalf("held 3 s, the note's level is %.3f at its end", got)
	}
	in.NoteOff(7)
	after := listen(in, 0.5)
	if got := rms(after[:synth.Rate/100]); got < 0.05 {
		t.Errorf("the note stops at once, its level %.3f, before its release", got)
	}
	if got := rms(after[len(after)-synth.Rate/10:]); got > 1e-4 || in.Sounding() {
		t.Errorf("0.5 s after its NoteOff the note sounds on, at %.5f", got)
	}
}

func TestAPedalHoldsOnlyItsGroup(t *testing.T) {
	in := instrument(t, sine)
	in.Pedal(1, 1)
	in.NoteOn(1, 57, 0.8, 1) // 220 Hz, held by the pedal
	in.NoteOn(2, 81, 0.8, 2) // 880 Hz, its group's pedal up
	listen(in, 0.2)
	in.NoteOff(1)
	in.NoteOff(2)
	x := listen(in, 1)[synth.Rate/2:]
	if got := hz(x); math.Abs(got-220) > 5 {
		t.Errorf("after both notes' NoteOffs, with group 1's pedal down, the sound is at %.0f Hz, not group 1's 220 Hz alone", got)
	}
	if rms(x) < 0.1 {
		t.Fatalf("the pedal let group 1's note go: its level is %.3f", rms(x))
	}
	in.Pedal(2, 0)
	if rms(listen(in, 0.2)) < 0.1 {
		t.Error("group 2's pedal let go of group 1's note")
	}
	in.Pedal(1, 0)
	listen(in, 0.5)
	if in.Sounding() {
		t.Error("group 1's note sounds on after its pedal lifted")
	}
}

func TestBendMovesTheNote(t *testing.T) {
	for _, p := range []*synth.Patch{sine, {Kind: "pluck", Pluck: synth.Pluck{Decay: 4, Bright: 0.05}, Amp: synth.Env{Sustain: 1, Release: 0.1}}} {
		in := instrument(t, p)
		in.NoteOn(3, 57, 1, 0)
		if got := hz(listen(in, 0.3)[synth.Rate/10:]); math.Abs(got-220) > 6 {
			t.Fatalf("%s: A3 sounds at %.1f Hz", p.Kind, got)
		}
		in.Bend(3, 7)
		listen(in, 0.05)
		if got := hz(listen(in, 0.2)); math.Abs(got-329.6) > 8 {
			t.Errorf("%s: A3 bent up 7 semitones sounds at %.1f Hz, not E4's 329.6", p.Kind, got)
		}
		in.Bend(3, -3)
		listen(in, 0.05)
		if got := hz(listen(in, 0.2)); math.Abs(got-185) > 5 {
			t.Errorf("%s: A3 bent down 3 semitones sounds at %.1f Hz, not F#3's 185", p.Kind, got)
		}
	}
}

func TestTheDrumsPlayByGeneralMIDINotes(t *testing.T) {
	in := instrument(t, &synth.Patch{Kind: "drums"})
	in.NoteOn(1, 36, 1, 0)
	if rms(listen(in, 0.2)) < 0.05 {
		t.Error("note 36 plays no bass drum")
	}
	listen(in, 3)
	in.NoteOn(2, 10, 1, 0)
	if in.Sounding() {
		t.Error("note 10, where General MIDI has no drum, sounds")
	}
}

func TestTheKnobsTurnTheSound(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(1, 69, 1, 0)
	loud := rms(listen(in, 0.5)[synth.Rate/4:])
	in.SetKnobs(synth.Knobs{Gain: -20})
	quiet := rms(listen(in, 0.5)[synth.Rate/4:])
	if r := quiet / loud; math.Abs(r-0.1) > 0.01 {
		t.Errorf("-20 dB of gain turns the note by %.3f, not 0.1", r)
	}
	in.SetKnobs(synth.Knobs{Release: 4})
	in.NoteOff(1)
	if rms(listen(in, 0.4)[synth.Rate*3/10:]) < 0.005 {
		t.Error("a release stretched 16 times, to 0.8 s, has ended in 0.3 s")
	}
}

func TestRenderingAllocatesNothing(t *testing.T) {
	for _, p := range []*synth.Patch{
		synth.GMPatch(0), synth.GMPatch(48), synth.GMPatch(25), synth.GMPatch(81), {Kind: "drums"},
	} {
		in := instrument(t, p)
		l, r := make([]float32, 256), make([]float32, 256)
		id := int32(0)
		allocs := testing.AllocsPerRun(200, func() {
			id++
			in.Pedal(id%3, float64(id%2))
			in.NoteOn(id, 48+float64(id%24), 0.8, id%3)
			in.Bend(id-1, 0.5)
			in.Render(l, r)
			in.NoteOff(id - 2)
		})
		if allocs != 0 {
			t.Errorf("%s: a block allocates %.1f times", p.Kind, allocs)
		}
	}
}
