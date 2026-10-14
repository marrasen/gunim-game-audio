package synth_test

import (
	"math"
	"testing"

	music "github.com/marrasen/gunim-game-audio"
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
	lo, hi := synth.Rate/1000, synth.Rate/50
	sums := make([]float64, hi+2)
	best := 0.0
	for lag := lo - 1; lag <= hi+1; lag++ {
		for i := lag; i < len(x); i++ {
			sums[lag] += float64(x[i]) * float64(x[i-lag])
		}
		sums[lag] /= float64(len(x) - lag)
		best = max(best, sums[lag])
	}
	// The first peak near the best, so a period twice as long loses,
	// placed between lags by the parabola through it.
	for lag := lo; lag <= hi; lag++ {
		a, b, c := sums[lag-1], sums[lag], sums[lag+1]
		if b >= a && b >= c && b >= 0.9*best {
			d := 0.0
			if den := a - 2*b + c; den != 0 {
				d = 0.5 * (a - c) / den
			}
			return float64(synth.Rate) / (float64(lag) + d)
		}
	}
	return 0
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
	// Note 1's ID, given up at its NoteOff, starts a new note in group 2,
	// which leaves the note the pedal holds be.
	in.NoteOn(1, 81, 0.8, 2)
	in.NoteOff(1)
	in.Pedal(2, 0)
	if x := listen(in, 0.5)[synth.Rate/4:]; rms(x) < 0.1 || math.Abs(hz(x)-220) > 5 {
		t.Errorf("a new note taking a pedalled note's ID let it go: %.3f at %.0f Hz", rms(x), hz(x))
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

// TestAKitPlaysItsOwnDrumsOnTheirNotes strikes General MIDI's notes on
// kits that name their drums otherwise, and holds each hit to the drum a
// song's pattern plays by name: the first hit of a fresh instrument
// sounds as a song's first hit of that drum does, frame by frame.
func TestAKitPlaysItsOwnDrumsOnTheirNotes(t *testing.T) {
	kit := func(names ...string) *synth.Patch {
		p := &synth.Patch{Kind: "drums", Kit: map[string]synth.Drum{}}
		for _, n := range names {
			p.Kit[n] = synth.Drum{}
		}
		return p
	}
	simmons := kit("bd", "sn", "syn1", "syn2", "syn3")
	for _, c := range []struct {
		p    *synth.Patch
		note float64
		drum string
	}{
		{kit("sbd", "ssn", "shh", "soh"), 36, "sbd"},
		{kit("sbd", "ssn", "shh", "soh"), 42, "shh"},
		{kit("sbd", "ssn", "shh", "soh"), 46, "soh"},
		{kit("sbd"), 38, "sn"},
		{kit("nbd", "nsn"), 38, "nsn"},
		{kit("bd9", "sn"), 36, "bd9"},
		{kit("bd", "bd9"), 36, "bd"},
		{simmons, 48, "syn1"},
		{simmons, 45, "syn2"},
		{simmons, 41, "syn3"},
		{kit("mtl", "cr"), 53, "mtl"},
		{kit("rd", "mtl"), 53, "rd"},
		{synth.GMKit(25), 36, "36"},
		{synth.GMKit(24), 38, "38"},
	} {
		want, err := synth.PreviewDrum(c.p, c.drum, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		in := instrument(t, c.p)
		in.NoteOn(1, c.note, 1, 0)
		got := listen(in, 0.5)
		for i := range want {
			if math.Abs(float64(got[i]-want[i])) > 1e-6 {
				t.Errorf("note %v of a kit of %d drums plays another drum than %s: frame %d is %v, %s's %v",
					c.note, len(c.p.Kit), c.drum, i, got[i], c.drum, want[i])
				break
			}
		}
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
			in.Mod(id-1, 0.7)
			in.Pressure(id-1, 0.6)
			in.SetKnobs(synth.Knobs{Cutoff: 0.5, Tune: 0.1, LFORate: 0.5, Vowel: 'e', Mod: 0.2})
			in.Render(l, r)
			in.NoteOff(id - 2)
		})
		if allocs != 0 {
			t.Errorf("%s: a block allocates %.1f times", p.Kind, allocs)
		}
	}
}

// preset returns the patch library's preset named name.
func preset(t *testing.T, name string) *synth.Patch {
	t.Helper()
	ps, err := music.Presets()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Name == name {
			return p.Patch
		}
	}
	t.Fatalf("the library has no preset %s", name)
	return nil
}

func TestTheSoundIsTheSameHoweverFinelyItIsRendered(t *testing.T) {
	// The pad's filter breathes by an LFO, and the bass glides.
	for _, name := range []string{"Juno pad", "Moog bass"} {
		var want []float32
		for _, chunk := range []int{256, 1, 37} {
			in := instrument(t, preset(t, name))
			var out []float32
			l, r := make([]float32, chunk), make([]float32, chunk)
			// Each run is 9600 frames, then its events.
			for _, ev := range []func(){
				func() { in.NoteOn(1, 45, 0.9, 0) },
				func() { in.NoteOn(2, 52, 0.9, 0); in.Bend(2, 0.5) },
				func() { in.NoteOff(2) },
				func() { in.NoteOff(1) },
			} {
				ev()
				for at := 0; at < 9600; at += chunk {
					m := min(chunk, 9600-at)
					in.Render(l[:m], r[:m])
					out = append(out, l[:m]...)
				}
			}
			if want == nil {
				want = out
				continue
			}
			for i := range want {
				if out[i] != want[i] {
					t.Errorf("%s rendered %d frames at a time differs from 256 at a time from frame %d: %v, not %v", name, chunk, i, out[i], want[i])
					break
				}
			}
		}
	}
}

func TestAMonoPatchGoesBackToTheNoteStillHeld(t *testing.T) {
	mono := *sine
	mono.Poly = 1
	in := instrument(t, &mono)
	in.NoteOn(1, 57, 1, 0) // A3, 220 Hz, held
	listen(in, 0.1)
	in.NoteOn(2, 64, 1, 0) // E4, 329.6 Hz
	if got := hz(listen(in, 0.2)[synth.Rate/20:]); math.Abs(got-329.6) > 6 {
		t.Fatalf("E4 over A3 held sounds at %.1f Hz", got)
	}
	in.NoteOff(2)
	x := listen(in, 0.3)[synth.Rate/20:]
	if got := hz(x); math.Abs(got-220) > 4 || rms(x) < 0.1 {
		t.Errorf("E4 let go, with A3 still held, sounds %.3f at %.1f Hz, not A3", rms(x), got)
	}
	in.NoteOff(1)
	listen(in, 0.5)
	if in.Sounding() {
		t.Error("A3 sounds on after its NoteOff")
	}
}

func TestANewNoteStartsUnbent(t *testing.T) {
	mono := *sine
	mono.Poly = 1
	in := instrument(t, &mono)
	in.NoteOn(1, 57, 1, 0)
	in.Bend(1, 7)
	listen(in, 0.1)
	in.NoteOff(1)
	// The voice still rings, letting go, as the next note takes it.
	in.NoteOn(2, 57, 1, 0)
	if got := hz(listen(in, 0.2)[synth.Rate/20:]); math.Abs(got-220) > 4 {
		t.Errorf("a new A3 on the voice a bent note played sounds at %.1f Hz, not 220", got)
	}
}

func TestLiftingAPedalLetsGoOfItsGroupAlone(t *testing.T) {
	in := instrument(t, sine)
	in.Pedal(1, 1)
	in.Pedal(2, 1)
	in.NoteOn(1, 57, 0.8, 1) // 220 Hz
	in.NoteOn(2, 81, 0.8, 2) // 880 Hz
	listen(in, 0.1)
	in.NoteOff(1)
	in.NoteOff(2)
	in.Pedal(2, 0)
	x := listen(in, 0.6)[synth.Rate/2:]
	if got := hz(x); math.Abs(got-220) > 5 || rms(x) < 0.1 {
		t.Errorf("group 2's pedal lifted, the sound is %.3f at %.0f Hz, not group 1's 220 Hz alone", rms(x), got)
	}
}

func TestFadeMovesANotesLevelInAStraightLine(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(1, 69, 1, 0)
	full := rms(listen(in, 0.1))
	// Halfway, at once: the note sounds at half its level.
	in.Fade(1, 0.5, 0)
	if r := rms(listen(in, 0.1)); math.Abs(r/full-0.5) > 0.01 {
		t.Errorf("faded to a half, the note sounds at %.3f of its level", r/full)
	}
	// To nothing over 0.1 s: the first half of it louder than the
	// second, and silent after.
	in.Fade(1, 0, synth.Rate/10)
	a, b := rms(listen(in, 0.05)), rms(listen(in, 0.05))
	if a <= b || b == 0 {
		t.Errorf("fading out, the note sounds at %.4f, then %.4f", a, b)
	}
	if r := rms(listen(in, 0.1)); r != 0 {
		t.Errorf("faded out, the note sounds at %.5f", r)
	}
	// A note struck again starts at its full level.
	in.NoteOn(1, 69, 1, 0)
	if r := rms(listen(in, 0.1)); math.Abs(r/full-1) > 0.01 {
		t.Errorf("struck again, the note sounds at %.3f of its level", r/full)
	}
}
