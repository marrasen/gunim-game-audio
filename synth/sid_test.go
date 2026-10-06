package synth

import (
	"math"
	"testing"
)

// render plays a note of p for frames, mono.
func renderNote(t *testing.T, p *Patch, n note, frames int) []float32 {
	t.Helper()
	c, err := p.compile("test")
	if err != nil {
		t.Fatal(err)
	}
	v := newVoice(3)
	v.start(c, n, 1)
	ctx := &renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	out := make([]float32, 0, frames)
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	for len(out) < frames {
		clear(l)
		clear(r)
		v.render(l, r, ctx)
		for i := range l {
			out = append(out, (l[i]+r[i])/2)
		}
	}
	return out[:frames]
}

// crossings counts how often x crosses 0, a rough measure of how high
// it sounds.
func crossings(x []float32) int {
	n := 0
	for i := 1; i < len(x); i++ {
		if (x[i-1] < 0) != (x[i] < 0) {
			n++
		}
	}
	return n
}

func held(pitch float32) note { return note{pitch: pitch, vel: 1, gate: rate, res: -1} }

var flat = Env{Attack: 0.001, Sustain: 1, Release: 0.05}

func TestSIDNoiseIsPitched(t *testing.T) {
	p := &Patch{Osc: []Osc{{Wave: "noise"}}, Amp: flat}
	low := crossings(renderNote(t, p, held(36), 9600))
	high := crossings(renderNote(t, p, held(84), 9600))
	if low == 0 || high < 4*low {
		t.Errorf("noise crosses 0 %d times at C2 and %d at C6; want it far busier up high", low, high)
	}
}

func TestCombinedWavesAreCentred(t *testing.T) {
	for _, w := range []int{oscSawTri, oscPulseTri, oscPulseSaw} {
		tab := combined(w, 0.5)
		var sum float64
		var top float32
		for _, v := range tab {
			sum += float64(v)
			top = max(top, abs32(v))
		}
		if math.Abs(sum/tableSize) > 1e-3 || top < 0.99 {
			t.Errorf("combined wave %d: mean %.4f, peak %.3f", w, sum/tableSize, top)
		}
	}
	p := &Patch{Osc: []Osc{{Wave: "sawtri"}}, Amp: flat}
	if rms(renderNote(t, p, held(60), 4800)) < 0.05 {
		t.Error("a sawtri note is silent")
	}
}

func TestSyncAndRing(t *testing.T) {
	plain := renderNote(t, &Patch{Osc: []Osc{{Wave: "saw", Level: 0.001}, {Wave: "saw", Semi: 7}}, Amp: flat}, held(48), 4800)
	synced := renderNote(t, &Patch{Osc: []Osc{{Wave: "saw", Level: 0.001}, {Wave: "saw", Semi: 7, Sync: true}}, Amp: flat}, held(48), 4800)
	ringed := renderNote(t, &Patch{Osc: []Osc{{Wave: "saw", Level: 0.001}, {Wave: "tri", Semi: 7, Ring: true}}, Amp: flat}, held(48), 4800)
	if diff(plain, synced) < 0.05 || rms(ringed) < 0.05 {
		t.Errorf("sync changed the sound by %.3f, and the ring sounds at %.3f", diff(plain, synced), rms(ringed))
	}
	if _, err := (&Patch{Osc: []Osc{{Wave: "saw", Sync: true}}}).compile("x"); err == nil {
		t.Error("a first oscillator synced to nothing compiled")
	}
}

func TestChipArpeggioStepsThroughTheChord(t *testing.T) {
	p := &Patch{Osc: []Osc{{Wave: "pulse"}}, Amp: flat, Arpeggio: &Arpeggio{Chord: true, Hz: 50}}
	n := held(60)
	n.chord, n.nchord = [8]int8{0, 4, 7}, 3
	x := renderNote(t, p, n, 3*960)
	// Each frame is a note of the chord: C, then E, then G, each higher.
	c, e, g := crossings(x[100:900]), crossings(x[1060:1860]), crossings(x[2020:2820])
	if c >= e || e >= g {
		t.Errorf("the arpeggio's frames cross 0 %d, %d and %d times; want them to climb", c, e, g)
	}
}

func TestSIDFilter(t *testing.T) {
	saw := []Osc{{Wave: "saw"}}
	open := renderNote(t, &Patch{Osc: saw, Amp: flat}, held(48), 9600)
	for _, typ := range []string{"sidlp", "sidbp", "sidhp", "sidnotch"} {
		x := renderNote(t, &Patch{Osc: saw, Amp: flat, Filter: Filter{Type: typ, Cutoff: 800, Res: 0.7}}, held(48), 9600)
		if r := rms(x); r < 0.01 || r > 2 || diff(x, open) < 0.02 {
			t.Errorf("%s: rms %.3f, %.3f from the open saw", typ, r, diff(x, open))
		}
	}
}

func TestSIDDrums(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sbd", "ssn", "scp", "shh", "soh", "stom", "szap"} {
		hs := newHits(1)
		hs.play(c.kit[name], 1, 0, 0, 0, 0)
		l, r := make([]float32, rate), make([]float32, rate)
		for at := 0; at < rate; at += maxBlock {
			end := min(at+maxBlock, rate)
			hs.render(l[at:end], r[at:end])
		}
		if rms(l[:rate/10]) < 0.01 {
			t.Errorf("%s is silent", name)
		}
		if rms(l[rate/2:]) > 1e-4 || hs.hs[0].on {
			t.Errorf("%s rings on after half a second", name)
		}
	}
}

func rms(x []float32) float64 {
	var s float64
	for _, v := range x {
		s += float64(v * v)
	}
	return math.Sqrt(s / float64(max(len(x), 1)))
}

func diff(a, b []float32) float64 {
	var s float64
	for i := range min(len(a), len(b)) {
		d := float64(a[i] - b[i])
		s += d * d
	}
	return math.Sqrt(s / float64(max(min(len(a), len(b)), 1)))
}
