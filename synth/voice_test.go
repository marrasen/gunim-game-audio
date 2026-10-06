package synth

import "testing"

func TestAVowelTurnedOffStopsSinging(t *testing.T) {
	sings := &Patch{Osc: []Osc{{Wave: "saw"}}, Vowel: "a", Amp: Env{Attack: 0.01, Sustain: 1, Release: 0.1}}
	plain := &Patch{Osc: []Osc{{Wave: "saw"}}, Amp: Env{Attack: 0.01, Sustain: 1, Release: 0.1}}
	a, err := sings.compile("sings")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := plain.compile("plain")
	v := newVoice(1)
	v.start(a, note{pitch: 60, vel: 1, gate: rate, res: -1}, 1)
	ctx := &renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	v.render(l, r, ctx)
	if v.form.f[0].ic1 == 0 {
		t.Fatal("the voice does not sing a")
	}
	// The patch is edited, as the studio does, with its vowel off.
	v.p = b
	v.render(l, r, ctx)
	was := v.form.f[0].ic1
	v.render(l, r, ctx)
	if v.form.f[0].ic1 != was {
		t.Error("the voice sings on after its patch's vowel was turned off")
	}
	// And on again, as e.
	sings.Vowel = "e"
	e, _ := sings.compile("e")
	v.p = e
	v.render(l, r, ctx)
	if v.vowel != 'e' || v.form.f[0].ic1 == was {
		t.Errorf("the voice sings %q after its patch's vowel became e", v.vowel)
	}
}
