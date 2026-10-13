package synth

import "testing"

// TestAnInstrumentSingsItsVowelsInTurn strikes notes on a patch with
// Vowels: each sings the next vowel, round from the last to the first,
// and Silence starts them again from the first. The vowel knob makes
// every note sing its own vowel, and the controllers move the vowel
// along.
func TestAnInstrumentSingsItsVowelsInTurn(t *testing.T) {
	p := &Patch{Noise: 1, Vocoder: true, Vowel: "a", Vowels: "a i o", Amp: Env{Sustain: 1, Release: 0.01}, Poly: 1}
	in, err := NewInstrument(p)
	if err != nil {
		t.Fatal(err)
	}
	l, r := make([]float32, 64), make([]float32, 64)
	sung := func() byte {
		in.Render(l, r)
		return in.held[in.n-1].v.vowel
	}
	for i, want := range []byte("aioai") {
		in.NoteOn(int32(i+1), 60, 1, 0)
		if got := sung(); got != want {
			t.Fatalf("note %d sings %c, not %c", i+1, got, want)
		}
	}
	in.Silence()
	in.NoteOn(9, 60, 1, 0)
	if got := sung(); got != 'a' {
		t.Fatalf("after Silence, the first note sings %c", got)
	}
	in.SetKnobs(Knobs{Vowel: 'u'})
	if got := sung(); got != 'u' {
		t.Fatalf("with the vowel knob at u, the note sings %c", got)
	}
	in.SetKnobs(Knobs{})
	// The patch answers pressure by a vowel's step: a opens to e.
	in.Pressure(9, 1)
	if got := sung(); got != 'e' {
		t.Fatalf("pressed, the note sings %c, not e", got)
	}
	if _, err := (&Patch{Noise: 1, Vowels: "a x"}).compile("bad"); err == nil {
		t.Fatal("a patch singing x compiles")
	}
}
