package synth

import "testing"

// renderKitHit renders drum name of kit at velocity vel for frames, mono.
func renderKitHit(t *testing.T, kit map[string]Drum, name string, vel float32, frames int) []float32 {
	t.Helper()
	c, err := (&Patch{Kind: "drums", Kit: kit}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	hs := newHits(1)
	hs.play(c.kit[name], vel, 0, 0, 0, 0)
	l, r := make([]float32, frames), make([]float32, frames)
	for at := 0; at < frames; at += maxBlock {
		end := min(at+maxBlock, frames)
		hs.render(l[at:end], r[at:end], false)
	}
	return l
}

// swings returns how many frames x's lowest pitch takes to swing n times, heard through a lowpass at 300 Hz.
func swings(x []float32, n int) int {
	var f svf
	f.set(300, 0)
	var last float32
	for i, s := range x {
		lp, _, _ := f.step(s)
		if (last < 0) != (lp < 0) && i > 0 {
			if n--; n == 0 {
				return i
			}
		}
		last = lp
	}
	return len(x)
}

// brightness returns the share of x's first 50 ms that sounds through a bandpass at 3 kHz.
func brightness(x []float32) float64 { return through(x[:rate/20], 3000) / rms(x[:rate/20]) }

func TestAHarderHitOnAModelledHeadIsBrighterAndHigher(t *testing.T) {
	kit := map[string]Drum{"tom": {Type: "mtom", Bend: 0.6}}
	for _, name := range []string{"mbd", "tom"} {
		hard, soft := renderKitHit(t, kit, name, 1, rate/2), renderKitHit(t, kit, name, 0.3, rate/2)
		if bh, bs := brightness(hard), brightness(soft); bh < bs*1.2 {
			t.Errorf("%s hit hard has %.4f of its sound at 3 kHz and soft %.4f; want hard the brighter", name, bh, bs)
		}
		if ph, ps := swings(hard[rate/100:], 4), swings(soft[rate/100:], 4); float64(ph) > 0.95*float64(ps) {
			t.Errorf("%s hit hard swings 4 times in %d frames 10 ms after it is hit and soft in %d; want hard the higher", name, ph, ps)
		}
	}
}

func TestTheModelledKickFallsAndRingsOut(t *testing.T) {
	x := renderKitHit(t, nil, "mbd", 1, 2*rate)
	if early, late := swings(x[rate/50:], 4), swings(x[rate/4:], 4); float64(early) > 0.92*float64(late) {
		t.Errorf("the kick swings 4 times in %d frames 20 ms after it is hit and in %d later; want its pitch falling", early, late)
	}
	if rms(x[:rate/10]) < 0.15 || rms(x[rate*3/2:]) > 2e-3 {
		t.Errorf("the kick sounds at %.3f, then %.5f", rms(x[:rate/10]), rms(x[rate*3/2:]))
	}
}

func TestModelledTomsAreTunedLowToHigh(t *testing.T) {
	var last int
	for _, name := range []string{"mlt", "mmt", "mht"} {
		x := renderKitHit(t, nil, name, 0.5, rate)
		n := crossings(x[rate/2 : rate/2+rate/10])
		if n <= last {
			t.Errorf("%s crosses 0 %d times in 100 ms, the tom below it %d", name, n, last)
		}
		last = n
	}
}

func TestAModelledHeadStruckAtItsEdgeIsBrighter(t *testing.T) {
	kit := map[string]Drum{"in": {Type: "mtom", Strike: 0.05}, "out": {Type: "mtom", Strike: 0.8}}
	in, out := renderKitHit(t, kit, "in", 1, rate/2), renderKitHit(t, kit, "out", 1, rate/2)
	if bi, bo := through(in[:rate/10], 400), through(out[:rate/10], 400); bo < bi*1.2 {
		t.Errorf("the tom struck at its centre sounds %.4f at 400 Hz and at its edge %.4f; want the edge the brighter", bi, bo)
	}
}

func TestEachHitOnAModelledHeadDiffersALittle(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	hs := newHits(1)
	render := func() []float32 {
		hs.play(c.kit["mmt"], 1, 0, 0, 0, 0)
		l, r := make([]float32, rate/4), make([]float32, rate/4)
		hs.render(l, r, false)
		return l
	}
	a, b := render(), render()
	var diff []float32
	for i := range a {
		diff = append(diff, a[i]-b[i])
	}
	if d, s := rms(diff), rms(a); d == 0 || d > s {
		t.Errorf("two hits differ by %.4f, at %.4f; want them a little apart", d, s)
	}
}

func TestAModelledHeadSoundsTheSameInAnyBlockSize(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mbd", "mmt", "msn", "mhh", "moh", "mcr", "mrd"} {
		var want []float32
		for _, block := range []int{256, 1, 37} {
			hs := newHits(1)
			hs.play(c.kit[name], 0.8, 0, 0, 0, 0)
			l, r := make([]float32, rate/2), make([]float32, rate/2)
			for at := 0; at < len(l); at += block {
				end := min(at+block, len(l))
				hs.render(l[at:end], r[at:end], true)
			}
			if want == nil {
				want = l
				continue
			}
			for i := range l {
				if l[i] != want[i] {
					t.Fatalf("%s in blocks of %d frame %d is %v, in blocks of 256 %v", name, block, i, l[i], want[i])
				}
			}
		}
	}
}

func TestTheModelledSnaresWiresBuzzAsItRings(t *testing.T) {
	x := renderKitHit(t, nil, "msn", 1, rate)
	if ring, gone := through(x[rate/10:rate/5], 1500), through(x[rate*7/10:], 1500); ring < 0.002 || gone > ring/15 {
		t.Errorf("the snare's wires buzz at %.4f as it rings and %.5f later; want them following its ring", ring, gone)
	}
}

func TestASnaresWiresRattleOnAfterItsHeadFallsQuiet(t *testing.T) {
	orig := heads[drModalSnare]
	defer func() { heads[drModalSnare] = orig }()
	h := orig
	h.ring, h.buzzRing = 0.02, 0.2
	heads[drModalSnare] = h
	x := renderKitHit(t, nil, "msn", 1, rate)
	if late := through(x[rate*3/10:rate*4/10], 1500); late < 1e-3 {
		t.Errorf("the snare's wires rattle at %.5f 300 ms after a hit; want them still rattling", late)
	}
}

func TestAModelledClosedHatChokesTheOpenOne(t *testing.T) {
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	ring := func(close bool) float64 {
		hs := newHits(2)
		hs.play(c.kit["moh"], 1, 0, 0, 0, 0)
		l, r := make([]float32, rate), make([]float32, rate)
		hs.render(l[:rate/20], r[:rate/20], true)
		if close {
			hs.play(c.kit["mhh"], 0.01, 0, 0, 0, 0)
		}
		hs.render(l[rate/20:], r[rate/20:], true)
		return rms(l[rate*2/5 : rate/2])
	}
	if open, closed := ring(false), ring(true); closed > open/20 {
		t.Errorf("the open hat rings at %.4f, and at %.4f once the closed hat is hit; want it choked", open, closed)
	}
}

func TestModelledCymbalsRingBrightAndLong(t *testing.T) {
	hat := renderKitHit(t, nil, "mhh", 1, 2*rate)
	for _, name := range []string{"mcr", "mrd"} {
		x := renderKitHit(t, nil, name, 1, 2*rate)
		if lo, hi := through(x[:rate/5], 300), through(x[:rate/5], 6000); lo > hi/2 {
			t.Errorf("%s sounds %.4f at 300 Hz and %.4f at 6 kHz; want it bright", name, lo, hi)
		}
		if ring, h := rms(x[rate:rate*6/5]), rms(hat[rate:rate*6/5]); ring < 0.01 || ring < 10*h {
			t.Errorf("%s rings at %.4f a second on, the closed hat at %.5f; want it ringing on", name, ring, h)
		}
	}
}
