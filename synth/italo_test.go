package synth

import (
	"math"
	"strings"
	"testing"
)

// renderHit renders drum name of a default kit for frames, mono.
func renderHit(t *testing.T, name string, frames int) []float32 {
	t.Helper()
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	hs := newHits(1)
	hs.play(c.kit[name], 1, 0, 0, 0, 0)
	l, r := make([]float32, frames), make([]float32, frames)
	for at := 0; at < frames; at += maxBlock {
		end := min(at+maxBlock, frames)
		hs.render(l[at:end], r[at:end])
	}
	return l
}

func TestSynTomsDiveAndTheCowbellRingsShort(t *testing.T) {
	for _, name := range []string{"syn1", "syn2", "syn3"} {
		x := renderHit(t, name, rate)
		if early, late := crossings(x[:rate/20]), crossings(x[rate/4:rate/4+rate/20]); early < late*3/2 {
			t.Errorf("%s crosses 0 %d times early and %d late; want its pitch to dive", name, early, late)
		}
		if rms(x[:rate/10]) < 0.05 {
			t.Errorf("%s sounds at only %.4f", name, rms(x[:rate/10]))
		}
	}
	hi, lo := crossings(renderHit(t, "syn1", rate/2)), crossings(renderHit(t, "syn3", rate/2))
	if hi <= lo {
		t.Errorf("syn1 crosses 0 %d times and syn3 %d; want syn1 the higher", hi, lo)
	}
	x := renderHit(t, "cb", rate)
	if rms(x[:rate/20]) < 0.02 || rms(x[rate/2:]) > 1e-4 {
		t.Errorf("the cowbell sounds at %.4f, then %.5f", rms(x[:rate/20]), rms(x[rate/2:]))
	}
}

// through returns how loud x is through a bandpass at hz.
func through(x []float32, hz float32) float64 {
	var f svf
	f.set(hz, 0.8)
	y := make([]float32, len(x))
	for i, s := range x {
		_, y[i], _ = f.step(s)
	}
	return rms(y)
}

func TestTheVocoderSpeaksItsVowels(t *testing.T) {
	saw := []Osc{{Wave: "saw"}}
	voc := func(v string) []float32 {
		return renderNote(t, &Patch{Osc: saw, Amp: flat, Vowel: v, Vocoder: true}, held(45), rate/2)[rate/10:]
	}
	a, i := voc("a"), voc("i")
	// "i" has its second resonance high, at 2 kHz, and its first low;
	// "a" its first at 800 Hz. The vocoder tells them apart near as well
	// as the formants do.
	f := func(v string) []float32 {
		return renderNote(t, &Patch{Osc: saw, Amp: flat, Vowel: v}, held(45), rate/2)[rate/10:]
	}
	fa, fi := f("a"), f("i")
	bright := func(x []float32) float64 { return through(x, 2200) / through(x, 760) }
	if got, want := bright(i)/bright(a), bright(fi)/bright(fa); got < 0.8*want {
		t.Errorf("the vocoder's i is %.2f times as bright as its a, and the formants' %.2f; want near as far apart", got, want)
	}
	if db := 20 * math.Log10(rms(a)/rms(fa)); math.Abs(db) > 3 {
		t.Errorf("the vocoder sings %.1f dB from the formants; want within 3", db)
	}
}

func TestEachChorusSounds(t *testing.T) {
	for name, kind := range chorusKinds {
		c := newChorus(kind)
		c.mix = 1
		l, r := make([]float32, rate), make([]float32, rate)
		for i := range l {
			l[i] = sin1(wrap(float32(i) * 440 / rate))
			r[i] = l[i]
		}
		for at := 0; at < rate; at += maxBlock {
			c.process(l[at:min(at+maxBlock, rate)], r[at:min(at+maxBlock, rate)])
		}
		var diff float64
		for i := range l {
			if math.IsNaN(float64(l[i])) || math.IsNaN(float64(r[i])) {
				t.Fatalf("chorus %q made NaN", name)
			}
			diff += float64((l[i] - r[i]) * (l[i] - r[i]))
		}
		if rms(l[rate/2:]) < 0.3 || rms(l[rate/2:]) > 1.5 {
			t.Errorf("chorus %q plays a sine at %.3f", name, rms(l[rate/2:]))
		}
		// Each sways its sides apart, so a sound in the middle widens.
		if math.Sqrt(diff/rate) < 0.01 {
			t.Errorf("chorus %q leaves the sides alike", name)
		}
	}
}

// oneHit is a song of a snare on the first beat of each bar, sent to the
// gated reverb by gated.
func oneHit(gated float64) *Song {
	return &Song{Title: "gate", BPM: 120, Key: "C", Mode: "wander",
		Patches: map[string]*Patch{"kit": {Kind: "drums"}},
		Tracks:  []*Track{{Name: "sn", Core: true, Patch: "kit", Pattern: "sn ~ ~ ~", Gated: gated}},
		Mix:     Mix{Gated: &Gated{Hold: 0.2}, Threshold: -1},
	}
}

func TestTheGatedReverbShutsAfterItsHold(t *testing.T) {
	render := func(s *Song) []float32 {
		p := NewPlayer(s, 1)
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		buf := make([]float32, 2*rate)
		p.Read(buf)
		mono := make([]float32, rate)
		for i := range mono {
			mono[i] = (buf[2*i] + buf[2*i+1]) / 2
		}
		return mono
	}
	dry, wet := render(oneHit(0)), render(oneHit(1))
	diff := func(from, to float64) float64 {
		d := make([]float32, int((to-from)*rate))
		for i := range d {
			j := int(from*rate) + i
			d[i] = wet[j] - dry[j]
		}
		return rms(d)
	}
	if open := diff(0.05, 0.18); open < 0.01 {
		t.Errorf("the room under the gate sounds at %.4f; want it heard", open)
	}
	if shut := diff(0.3, 0.9); shut > 1e-4 {
		t.Errorf("the room sounds at %.5f after the gate shut; want silence", shut)
	}
}

func TestMonoBassCentresTheBass(t *testing.T) {
	s := &Song{Title: "mono", BPM: 120, Key: "C", Mode: "wander",
		Patches: map[string]*Patch{"sub": {Osc: []Osc{{Wave: "sine"}}, Amp: flat}},
		Tracks:  []*Track{{Name: "sub", Core: true, Patch: "sub", Pattern: "C2", Pan: -1, Legato: 1}},
		Mix:     Mix{MonoBass: 150, Threshold: -1},
	}
	p := NewPlayer(s, 1)
	buf := make([]float32, 2*rate)
	p.Read(buf)
	var l, r float64
	for i := rate / 4; i < rate; i++ {
		l += float64(buf[2*i] * buf[2*i])
		r += float64(buf[2*i+1] * buf[2*i+1])
	}
	if ratio := math.Sqrt(r / l); ratio < 0.9 {
		t.Errorf("a bass hard left plays on the right at %.2f of the left; want it in the middle", ratio)
	}
}

func TestDriftDetunesEachNote(t *testing.T) {
	c, err := (&Patch{Osc: []Osc{{Wave: "saw"}}, Amp: flat, Drift: 10}).compile("drift")
	if err != nil {
		t.Fatal(err)
	}
	v := newVoice(5)
	seen := map[float32]bool{}
	for range 8 {
		v.start(c, held(60), 1)
		if math.Abs(float64(v.drift)) > 0.1 {
			t.Fatalf("a note drifts %.3f semitones; want within 10 cents", v.drift)
		}
		seen[v.drift] = true
	}
	if len(seen) < 6 {
		t.Errorf("8 notes drift %d ways; want each its own", len(seen))
	}
}

func TestAnUnknownChorusSaysWhy(t *testing.T) {
	s := oneHit(0)
	s.Tracks[0].ChorusType = "flanger"
	if err := s.Check(); err == nil || !strings.Contains(err.Error(), "flanger") {
		t.Errorf("got %v, want an error naming the chorus", err)
	}
}
