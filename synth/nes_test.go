package synth

import "testing"

func TestNESWaves(t *testing.T) {
	for _, w := range []string{"nespulse", "nestri", "nesnoise", "nesmetal", "gbwave"} {
		p := &Patch{Osc: []Osc{{Wave: w}}, Amp: flat}
		if rms(renderNote(t, p, held(60), 4800)) < 0.05 {
			t.Errorf("a %s note is silent", w)
		}
	}
	c, _ := (&Patch{Osc: []Osc{{Wave: "nespulse", Width: 0.3}}}).compile("x")
	if c.osc[0].Width != 0.25 {
		t.Errorf("a NES pulse of width 0.3 plays at %v, want 0.25", c.osc[0].Width)
	}
	if _, err := (&Patch{Osc: []Osc{{Wave: "gbwave", Table: []int{0, 16}}}}).compile("x"); err == nil {
		t.Error("a wave table of a step 16 compiled")
	}
}

func TestNESNoiseModes(t *testing.T) {
	// The short mode loops in 93 steps, the long in 32767.
	long, short := nesNoise{}, nesNoise{short: true}
	seen := func(n *nesNoise) int {
		states := map[uint16]bool{}
		for i := range 40000 {
			n.at(float32(i%16) / 16)
			states[n.reg] = true
		}
		return len(states)
	}
	if l, s := seen(&long), seen(&short); l < 30000 || s > 100 {
		t.Errorf("the long mode goes through %d states and the short %d; want 32767 and 93", l, s)
	}
}

func TestChipLevelsStep(t *testing.T) {
	p := &Patch{Osc: []Osc{{Wave: "nespulse"}}, Amp: Env{Attack: 0.2, Sustain: 1, Release: 0.1}, Chip: &Chip{Levels: 4, Hz: 60}}
	x := renderNote(t, p, held(60), 24000)
	// Within a frame the level holds, at a step of the full level the
	// held note reaches.
	frame := rate / 60
	var full float32
	for _, v := range x[20000:] {
		full = max(full, abs32(v))
	}
	for f := 1; f < 10; f++ {
		var top float32
		for _, v := range x[f*frame : (f+1)*frame] {
			top = max(top, abs32(v))
		}
		lv := float64(top/full) * 4
		if d := lv - float64(int(lv+0.5)); d > 0.15 || d < -0.15 {
			t.Errorf("frame %d peaks at %.3f, between the 4 levels", f, top)
		}
	}
}

func TestBendSlidesIn(t *testing.T) {
	p := &Patch{Osc: []Osc{{Wave: "tri"}}, Amp: flat, Bend: &Bend{Semis: 24, Time: 0.1}}
	x := renderNote(t, p, held(48), rate/2)
	if early, late := crossings(x[:rate/40]), crossings(x[rate/4:rate/4+rate/40]); early < 2*late {
		t.Errorf("the bent note crosses 0 %d times early and %d late; want it far higher early", early, late)
	}
}

func TestNESDrums(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nbd", "nsn", "nhh", "noh", "ntom", "nclk"} {
		hs := newHits(1)
		hs.play(c.kit[name], 1, 0, 0, 0, 0)
		l, r := make([]float32, rate), make([]float32, rate)
		for at := 0; at < rate; at += maxBlock {
			end := min(at+maxBlock, rate)
			hs.render(l[at:end], r[at:end])
		}
		if rms(l[:rate/20]) < 0.01 || rms(l[rate/2:]) > 1e-4 {
			t.Errorf("%s sounds at %.4f, then %.5f", name, rms(l[:rate/20]), rms(l[rate/2:]))
		}
	}
}
