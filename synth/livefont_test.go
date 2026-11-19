package synth

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
	"github.com/marrasen/gunim-game-audio/soundfont"
)

// testFont registers the test bank by name for the test's run.
func testFont(t *testing.T, name string) *soundfont.Font {
	t.Helper()
	f, err := soundfont.Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	RegisterSoundFont(name, f)
	t.Cleanup(func() { RegisterSoundFont(name, nil) })
	return f
}

// hear renders secs of in, in blocks of 256 frames, and returns its
// right channel, where the test bank's square is panned.
func hear(in *Instrument, secs float64) []float32 {
	n := int(secs * rate)
	out := make([]float32, 0, n)
	l, r := make([]float32, 256), make([]float32, 256)
	for len(out) < n {
		m := min(256, n-len(out))
		in.Render(l[:m], r[:m])
		out = append(out, r[:m]...)
	}
	return out
}

func TestAnInstrumentPlaysABanksSamples(t *testing.T) {
	testFont(t, "live-test")
	in, err := NewInstrument(&Patch{SoundFont: "live-test"})
	if err != nil {
		t.Fatal(err)
	}
	if in.p.kind != kindFont {
		t.Fatal("a patch of a bank registered plays on the engine's own instruments")
	}
	in.NoteOn(1, 72, 1, 0)
	out := hear(in, 0.5)
	// The square of 100 frames at 44100 Hz is 441 Hz at its root, 60:
	// key 72 plays it an octave up, 882 Hz.
	if hz := float64(crossings(out[rate/10:])) / 2 / 0.4; math.Abs(hz-882) > 20 {
		t.Errorf("key 72 plays %.0f Hz, want 882", hz)
	}
	in.Bend(1, -12)
	if hz := float64(crossings(hear(in, 0.5)[rate/10:])) / 2 / 0.4; math.Abs(hz-441) > 15 {
		t.Errorf("bent down an octave, it plays %.0f Hz, want 441", hz)
	}
	in.NoteOff(1)
	hear(in, 1)
	if in.Sounding() {
		t.Error("a note let go still sounds a second on, past its release of a tenth of one")
	}
}

func TestThePedalHoldsABanksNotes(t *testing.T) {
	testFont(t, "live-pedal")
	in, err := NewInstrument(&Patch{SoundFont: "live-pedal"})
	if err != nil {
		t.Fatal(err)
	}
	in.Pedal(0, 1)
	in.NoteOn(1, 60, 0.8, 0)
	in.NoteOff(1)
	if rms(hear(in, 0.5)[rate*4/10:]) < 0.01 {
		t.Error("a note let go while the pedal is down rang out")
	}
	in.Pedal(0, 0)
	hear(in, 0.5)
	if in.Sounding() {
		t.Error("a note the pedal held rings on once it lifts")
	}
}

func TestABankMissingPlaysTheEnginesOwn(t *testing.T) {
	in, err := NewInstrument(&Patch{SoundFont: "no-such-bank", Program: 40})
	if err != nil {
		t.Fatal(err)
	}
	if in.p.kind == kindFont || in.Drums() {
		t.Error("a bank's violin, its bank missing, does not play General MIDI's")
	}
	for _, p := range []*Patch{{SoundFont: "no-such-bank", Bank: DrumBank}, {SoundFont: "no-such-bank", FontKit: true}} {
		in, err := NewInstrument(p)
		if err != nil {
			t.Fatal(err)
		}
		if !in.Drums() {
			t.Errorf("a bank's kit %+v, its bank missing, plays no kit", p)
		}
	}
}

func TestABanksNotesAllocateNothing(t *testing.T) {
	testFont(t, "live-allocs")
	in, err := NewInstrument(&Patch{SoundFont: "live-allocs"})
	if err != nil {
		t.Fatal(err)
	}
	l, r := make([]float32, 256), make([]float32, 256)
	id := int32(0)
	allocs := testing.AllocsPerRun(200, func() {
		id++
		in.Pedal(id%3, float64(id%2))
		in.NoteOn(id, 48+float64(id%24), 0.8, id%3)
		in.Bend(id-1, 0.5)
		in.SetKnobs(Knobs{Cutoff: -0.5, Tune: 0.1, Attack: 0.5, Pan: -0.2})
		in.Render(l, r)
		in.NoteOff(id - 2)
	})
	if allocs != 0 {
		t.Errorf("a block allocates %.1f times", allocs)
	}
}

func TestABanksSoundRetunesToItself(t *testing.T) {
	testFont(t, "live-retune")
	a, _ := NewInstrument(&Patch{SoundFont: "live-retune"})
	b, _ := NewInstrument(&Patch{SoundFont: "live-retune", Gain: 0.5})
	c, _ := NewInstrument(&Patch{SoundFont: "live-retune", Bank: DrumBank})
	if !a.Retune(b) {
		t.Error("the same program, turned down, does not play on in its place")
	}
	if a.Retune(c) {
		t.Error("another program plays on in the notes of the first")
	}
}

// A bank's tremolo can sweep its filter's cutoff past the top, where a
// voice leaves its filter out, as MuseScore General's Strings Tremolo
// does 14 times a second. The filter must run all through the note:
// switched off and back on, it clicks at every turn. So in no 5 ms of
// the note does it sound as the same note unfiltered.
func TestAFilterSweptPastTheTopRunsAllThroughTheNote(t *testing.T) {
	f := testFont(t, "sweep-test")
	rs := f.Regions(0, 0, 60, 100)
	if len(rs) == 0 {
		t.Fatal("the test bank has no region")
	}
	swept, plain := rs[0], rs[0]
	swept.Cutoff, swept.ModHz, swept.ModDelay, swept.ModLFOToFilter = 14830, 14, 0, 2000
	plain.Cutoff = 20000
	var a, b sfVoice
	a.start(&swept, 60, 100, 1, 0, true)
	b.start(&plain, 60, 100, 1, 0, true)
	w := rate / 200
	al, ar := make([]float32, w), make([]float32, w)
	bl, br := make([]float32, w), make([]float32, w)
	for at := 0; at < rate/2; at += w {
		clear(al)
		clear(ar)
		clear(bl)
		clear(br)
		a.render(al, ar)
		b.render(bl, br)
		if slices.Equal(al, bl) && slices.Equal(ar, br) {
			t.Fatalf("at %d ms the note sounds unfiltered", at*1000/rate)
		}
	}
}
