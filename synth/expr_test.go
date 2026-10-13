package synth_test

import (
	"math"
	"testing"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

// TestThePressureTurnsANoteUp presses a sine's key all the way, and its
// level rises by the 5 dB a patch with no filter answers pressure with,
// from the next control step; let go of, it falls back.
func TestThePressureTurnsANoteUp(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(1, 69, 1, 0)
	still := rms(listen(in, 0.3)[synth.Rate/10:])
	in.Pressure(1, 1)
	pressed := rms(listen(in, 0.3)[synth.Rate/10:])
	if r, want := pressed/still, math.Pow(10, 5.0/20); math.Abs(r-want) > 0.02 {
		t.Fatalf("pressed all the way, the note sounds %.3f times as loud, not %.3f", r, want)
	}
	in.Pressure(1, 0)
	if r := rms(listen(in, 0.3)[synth.Rate/10:]) / still; math.Abs(r-1) > 0.02 {
		t.Fatalf("let go, the note sounds %.3f times as loud as before", r)
	}
}

// TestTheModWheelBringsInAVibrato turns the mod wheel up on a held
// sine: its pitch, steady before, swings by the patch's vibrato either
// way of the note, and settles back once the wheel is down.
func TestTheModWheelBringsInAVibrato(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(1, 69, 1, 0)
	listen(in, 0.1)
	spread := func() (lo, hi float64) {
		lo, hi = math.Inf(1), 0
		// 40 ms windows over four cycles of the vibrato.
		for range 18 {
			f := hz(listen(in, 0.04))
			lo, hi = min(lo, f), max(hi, f)
		}
		return lo, hi
	}
	if lo, hi := spread(); hi/lo > 1.005 {
		t.Fatalf("with the wheel down, the note runs from %.1f Hz to %.1f", lo, hi)
	}
	in.Mod(1, 1)
	lo, hi := spread()
	// 0.4 semitones either way: 440 Hz swings from 430 to 450, less
	// what a window of 40 ms smooths away.
	if lo > 433 || hi < 447 || lo < 425 || hi > 455 {
		t.Fatalf("with the wheel up, the note runs from %.1f Hz to %.1f, not about 430 to 450", lo, hi)
	}
	in.Mod(1, 0)
	if lo, hi := spread(); hi/lo > 1.005 {
		t.Fatalf("with the wheel down again, the note runs from %.1f Hz to %.1f", lo, hi)
	}
}

// TestEveryPatchAnswersTheControllers plays a note of each preset of
// the library, other than drums, with the mod wheel and the pressure all
// the way up, and each sounds otherwise than without them.
func TestEveryPatchAnswersTheControllers(t *testing.T) {
	presets, err := music.Presets()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range presets {
		if p.Patch.Kind == "drums" {
			continue
		}
		play := func(mod, press float64) []float32 {
			in := instrument(t, p.Patch)
			in.NoteOn(1, 60, 0.8, 0)
			in.Mod(1, mod)
			in.Pressure(1, press)
			return listen(in, 0.4)
		}
		plain, moved := play(0, 0), play(1, 1)
		diff := 0.0
		for i := range plain {
			diff += math.Abs(float64(plain[i] - moved[i]))
		}
		if diff/float64(len(plain)) < 1e-4 {
			t.Errorf("%s sounds the same with the mod wheel and the pressure up", p.Name)
		}
	}
}

// TestRetuneKeepsTheNotesPlaying holds a note on a filtered saw, then
// retunes the instrument to the same patch with the filter opened: the
// note plays on unbroken, brighter, where a patch of another shape is
// refused.
func TestRetuneKeepsTheNotesPlaying(t *testing.T) {
	dark := &synth.Patch{Osc: []synth.Osc{{Wave: "saw"}}, Filter: synth.Filter{Type: "lp", Cutoff: 400},
		Amp: synth.Env{Attack: 0.3, Sustain: 1, Release: 0.05}}
	bright := *dark
	bright.Filter.Cutoff = 6000
	in := instrument(t, dark)
	in.NoteOn(1, 57, 1, 0)
	// Half way up its attack.
	before := listen(in, 0.15)
	if !in.Retune(instrument(t, &bright)) {
		t.Fatal("a patch with its cutoff moved is refused")
	}
	after := listen(in, 0.01)
	// No restart: the level carries on from where the attack was.
	if a, b := math.Abs(float64(before[len(before)-1])), math.Abs(float64(after[0])); b > a+0.05 {
		t.Fatalf("the note jumps from %.3f to %.3f as it is retuned", a, b)
	}
	if in.Held() != 1 || !in.Sounding() {
		t.Fatal("the note held stops as the instrument is retuned")
	}
	// Brighter: more of the sound's power changes from one frame to the
	// next.
	edge := func(x []float32) float64 {
		var d, e float64
		for i := 1; i < len(x); i++ {
			d += float64(x[i]-x[i-1]) * float64(x[i]-x[i-1])
			e += float64(x[i]) * float64(x[i])
		}
		return d / e
	}
	dim := instrument(t, dark)
	dim.NoteOn(1, 57, 1, 0)
	if e0, e1 := edge(listen(dim, 0.5)[synth.Rate/4:]), edge(listen(in, 0.35)[synth.Rate/10:]); e1 < 2*e0 {
		t.Fatalf("retuned to a cutoff of 6 kHz, the note's edge is %.1f, against %.1f at 400 Hz", e1, e0)
	}
	two := bright
	two.Osc = append([]synth.Osc{{Wave: "saw"}}, synth.Osc{Wave: "pulse"})
	if in.Retune(instrument(t, &two)) {
		t.Fatal("a patch with an oscillator more is taken in place")
	}
}

// TestTheTuneKnobMovesEveryNote turns the tune knob up a fifth, and a
// held A sounds an E.
func TestTheTuneKnobMovesEveryNote(t *testing.T) {
	in := instrument(t, sine)
	in.NoteOn(1, 69, 1, 0)
	listen(in, 0.05)
	in.SetKnobs(synth.Knobs{Tune: 7})
	listen(in, 0.01)
	if f, want := hz(listen(in, 0.1)), 440*math.Exp2(7.0/12); math.Abs(f-want) > 1 {
		t.Fatalf("tuned up 7 semitones, A sounds %.1f Hz, not %.1f", f, want)
	}
}

// TestWhisperSingsTheSongsVowels reads the vowels Wire Cathedral's
// whisper sings, in turn, from the track that plays it.
func TestWhisperSingsTheSongsVowels(t *testing.T) {
	b, err := music.Song("wire-cathedral")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := b.(*synth.Song)
	if !ok {
		t.Fatalf("wire cathedral is a %T", b)
	}
	if got := s.Vowels("whisper"); got != "a i o u e a o i" {
		t.Fatalf("whisper sings %q", got)
	}
	if got := s.Vowels("scream"); got != "a o a e" {
		t.Fatalf("scream sings %q", got)
	}
	if got := s.Vowels("synth"); got != "" {
		t.Fatalf("the synth, which sings no vowels, sings %q", got)
	}
}
