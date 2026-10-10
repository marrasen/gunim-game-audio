package synth

import (
	"math"
	"slices"
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

func TestThe909KickFallsAndTheMetalRings(t *testing.T) {
	x := renderHit(t, "bd9", rate)
	if early, late := crossings(x[:rate/100]), crossings(x[rate/20:rate/20+rate/100]); early < 2*late {
		t.Errorf("the 909 kick crosses 0 %d times in its first 10 ms and %d later; want it falling fast", early, late)
	}
	if rms(x[:rate/10]) < 0.1 || rms(x[rate*4/5:]) > 2e-3 {
		t.Errorf("the 909 kick sounds at %.3f, then %.5f", rms(x[:rate/10]), rms(x[rate*4/5:]))
	}
	// Metal rings on, and stops at its end.
	m := renderHit(t, "mtl", 2*rate)
	if rms(m[:rate/10]) < 0.05 || rms(m[rate/2:rate/2+rate/10]) < 0.01 || rms(m[rate*17/10:]) > 1e-4 {
		t.Errorf("the metal sounds at %.3f, rings at %.5f, and ends at %.5f", rms(m[:rate/10]), rms(m[rate/2:rate/2+rate/10]), rms(m[rate*17/10:]))
	}
}

func TestSlideGlidesOnlyIntoATiedNote(t *testing.T) {
	c, err := (&Patch{Osc: []Osc{{Wave: "saw"}}, Amp: flat, Poly: 1, Glide: 0.08, Slide: true}).compile("acid")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	run := func(v *voice, frames int) {
		for range frames / maxBlock {
			v.render(l, r, ctx)
		}
	}
	// A note let go before the next: the next jumps to its pitch.
	v := newVoice(1)
	v.start(c, note{pitch: 48, vel: 1, gate: rate / 20, res: -1}, 1)
	run(v, rate/5)
	v.start(c, note{pitch: 60, vel: 1, gate: rate / 5, res: -1}, 2)
	if v.pitch != 60 {
		t.Errorf("after a rest the note starts at %.2f, want 60", v.pitch)
	}
	// A note still held: the next slides up from it.
	v = newVoice(1)
	v.start(c, note{pitch: 48, vel: 1, gate: rate, res: -1}, 1)
	run(v, rate/10)
	v.start(c, note{pitch: 60, vel: 1, gate: rate / 5, res: -1}, 2)
	run(v, maxBlock)
	if v.pitch > 55 {
		t.Errorf("a tied note is at %.2f a moment after it starts; want it sliding up from 48", v.pitch)
	}
}

func TestEachDistortionDistorts(t *testing.T) {
	for kind := range distortKinds {
		var d distortion
		d.set(0.8, kind)
		l, r := make([]float32, rate/2), make([]float32, rate/2)
		for i := range l {
			l[i] = 0.4 * sin1(wrap(float32(i)*110/rate))
			r[i] = l[i]
		}
		d.process(l, r)
		peak := float32(0)
		for _, v := range l {
			if math.IsNaN(float64(v)) {
				t.Fatalf("%q made NaN", kind)
			}
			peak = max(peak, abs32(v))
		}
		// Driven hard, a sine of 110 Hz gains harmonics: much of it now
		// lies above 300 Hz, where a sine has nothing.
		var hp [2]svf
		hp[0].set(300, 0.3)
		hp[1].set(300, 0.3)
		high := make([]float32, len(l))
		for i, v := range l {
			_, _, v = hp[0].step(v)
			_, _, high[i] = hp[1].step(v)
		}
		share := rms(high[rate/10:]) / rms(l[rate/10:])
		if rms(l) < 0.05 || peak > 1.05 || share < 0.3 {
			t.Errorf("%q plays a sine at %.3f, its peak %.3f, %.2f of it above 300 Hz", kind, rms(l), peak, share)
		}
	}
}

func TestRingAndSmash(t *testing.T) {
	// Ringed fully at 440 Hz, a tone of 100 Hz becomes 340 and 540 Hz:
	// little is left below 200 Hz.
	in := inserts{}
	in.set(&Track{Ring: 1, RingHz: 440})
	l, r := make([]float32, rate/2), make([]float32, rate/2)
	for i := range l {
		l[i] = 0.5 * sin1(wrap(float32(i)*100/rate))
		r[i] = l[i]
	}
	in.process(l, r)
	var lp [2]svf
	lp[0].set(200, 0.3)
	lp[1].set(200, 0.3)
	low := make([]float32, len(l))
	for i, v := range l {
		v, _, _ = lp[0].step(v)
		low[i], _, _ = lp[1].step(v)
	}
	if share := rms(low[rate/10:]) / rms(l[rate/10:]); share > 0.1 {
		t.Errorf("ringed, %.2f of a 100 Hz tone is left under 200 Hz", share)
	}
	// Smashed, a hit's tail comes up against its attack.
	hit := func(smash float64) (attack, tail float64) {
		in := inserts{}
		in.set(&Track{Smash: smash})
		l := renderHit(t, "sn", rate/2)
		r := slices.Clone(l)
		in.process(l, r)
		return rms(l[:rate/50]), rms(l[rate/8 : rate/4])
	}
	a0, t0 := hit(0)
	a1, t1 := hit(1)
	if t1/a1 < 2*t0/a0 {
		t.Errorf("smashed, the snare's tail is %.3f of its attack, against %.3f; want it pumped up", t1/a1, t0/a0)
	}
}

func TestAccentsStack(t *testing.T) {
	c, err := (&Patch{Osc: []Osc{{Wave: "saw"}}, Filter: Filter{Type: "lp24", Cutoff: 300, Res: 0.6, Env: 2},
		Amp: flat, FilterEnv: Env{Attack: 0.001, Decay: 1, Release: 0.05}, Poly: 1, Accent: 1.5}).compile("acid")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	v := newVoice(1)
	play := func(vel float32) float32 {
		v.start(c, note{pitch: 36, vel: vel, gate: rate / 10, res: -1}, 1)
		for range rate / 8 / maxBlock {
			v.render(l, r, ctx)
		}
		return v.accent
	}
	if a := play(0.7); a != 0 {
		t.Fatalf("a note not accented charges the accent to %.2f", a)
	}
	first := play(1)
	play(1)
	third := play(1)
	if third < first*1.4 {
		t.Errorf("three accents an eighth apart leave %.2f, against %.2f after one; want them stacked", third, first)
	}
}
