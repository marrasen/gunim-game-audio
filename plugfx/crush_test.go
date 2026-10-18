package plugfx

import (
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
)

// crushed returns a sine at hz crushed as set says, undithered.
func crushed(hz float64, bits int, rate, smooth float64) []float32 {
	c := newCrusher(48000)
	c.set(bits, rate, smooth, false)
	in := sine(48000, hz, 0.8)
	c.process(in[0], in[1])
	return in[0]
}

func TestSixteenBitsAtTheFullRatePassTheSoundWithinAStep(t *testing.T) {
	in := sine(48000, 440, 0.8)[0]
	out := crushed(440, 16, 48000, 0)
	for i := range in {
		if d := math.Abs(float64(out[i] - in[i])); d > 1.0/(1<<15) {
			t.Fatalf("frame %d is %g off", i, d)
		}
	}
}

func TestTheBitsKeepThatManyLevels(t *testing.T) {
	for _, bits := range []int{2, 4, 8} {
		levels := map[float32]bool{}
		for _, v := range crushed(440, bits, 48000, 0) {
			levels[v] = true
		}
		if n := len(levels); n > 1<<bits {
			t.Errorf("%d bits keep %d levels", bits, n)
		}
		if n := len(levels); n < 1<<(bits-1) {
			t.Errorf("%d bits keep only %d levels of a loud sine", bits, n)
		}
	}
}

func TestOneBitKeepsTheSignAlone(t *testing.T) {
	for i, v := range crushed(440, 1, 48000, 0) {
		if v != 0.5 && v != -0.5 && v != 0 {
			t.Fatalf("frame %d is %g", i, v)
		}
	}
}

func TestTheRateHoldsEachSampleTillTheNext(t *testing.T) {
	out := crushed(440, 16, 12000, 0)
	// 48 kHz over 12 kHz: each sample is held for 4 frames.
	for i := 0; i+4 <= len(out); i += 4 {
		if out[i+1] != out[i] || out[i+2] != out[i] || out[i+3] != out[i] {
			t.Fatalf("frames %d to %d are %v", i, i+3, out[i:i+4])
		}
		if i > 0 && out[i] == out[i-1] {
			t.Fatalf("frame %d holds the sample before", i)
		}
	}
}

func TestSmoothTakesAwayWhatWouldFoldBackDown(t *testing.T) {
	// An 11 kHz sine at a 8 kHz rate folds down to 3 kHz, unless Smooth
	// takes it away first.
	rms := func(x []float32) float64 {
		var s float64
		for _, v := range x[4800:] {
			s += float64(v) * float64(v)
		}
		return math.Sqrt(s / float64(len(x)-4800))
	}
	raw, smooth := rms(crushed(11000, 16, 8000, 0)), rms(crushed(11000, 16, 8000, 1))
	if raw < 0.3 {
		t.Fatalf("with no smoothing the fold comes out at %g", raw)
	}
	if smooth > raw/20 {
		t.Errorf("smoothed, the fold comes out at %g, of %g", smooth, raw)
	}
}

func TestSilenceStaysSilentUndithered(t *testing.T) {
	c := newCrusher(48000)
	c.set(4, 8000, 0.5, false)
	l, r := make([]float32, 4800), make([]float32, 4800)
	c.process(l, r)
	if p := peak(l, r); p != 0 {
		t.Errorf("silence comes out at %g", p)
	}
}

func TestADitheredCrushRendersTheSameEachTime(t *testing.T) {
	run := func() []float32 {
		c := newCrusher(48000)
		c.set(6, 22050, 0.3, true)
		c.clear()
		in := sine(4800, 300, 0.5)
		c.process(in[0], in[1])
		return append(in[0], in[1]...)
	}
	a, b := run(), run()
	if !slices.Equal(a, b) {
		t.Error("two renders differ")
	}
	if slices.Equal(a, crushed(300, 6, 22050, 0.3)[:4800]) {
		t.Error("the dither changes nothing")
	}
}

func TestColoursCrushBlendsByItsMix(t *testing.T) {
	in := noise(setup.MaxBlock*8, setup.MaxBlock*8)
	run := func(mix float64) [][]float32 {
		p := colourWith(t, []Unit{{Kind: Crush}})
		return rig{pr: p.Processor(), d: &ColourDescriptor, s: setup}.run(t, in,
			plug.Change{ID: UnitParamID(0, CrushBitsParam), Value: 3},
			plug.Change{ID: UnitParamID(0, CrushMixParam), Value: mix})
	}
	dry, wet := run(0), run(100)
	for ch := range dry {
		for i := range dry[ch] {
			if d := math.Abs(float64(dry[ch][i] - in[ch][i])); d > 1e-6 {
				t.Fatalf("at no mix, channel %d frame %d is %g off the sound", ch, i, d)
			}
		}
	}
	if slices.Equal(wet[0], in[0]) {
		t.Error("at full mix the sound passes uncrushed")
	}
}

func TestTheCrushsChipNamesItsBits(t *testing.T) {
	h := newHost(&ColourDescriptor)
	p := as[*colourPlugin](t, ColourDescriptor.New(h))
	p.setUnits([]Unit{{Kind: Crush}, {Kind: Chorus, Slot: 1}})
	c := as[*colourChip](t, p.View(plug.Chip, h))
	if got := c.Access().Name; got != "Colour: 12-bit, Juno I" {
		t.Errorf("the chip reads %q", got)
	}
}

func TestSilenceStaysSilentDithered(t *testing.T) {
	// So a host may let the crush sleep through a pause.
	c := newCrusher(48000)
	c.set(4, 8000, 0.5, true)
	in := sine(4800, 300, 0.5)
	c.process(in[0], in[1])
	l, r := make([]float32, 48000), make([]float32, 48000)
	c.process(l, r)
	if p := peak(l[24000:], r[24000:]); p != 0 {
		t.Errorf("after the sound stops, silence comes out at %g", p)
	}
}

func TestEachCardsSelectorsFollowTheirOwnKind(t *testing.T) {
	// Each slot holds every kind's parameters: a crush's Dither stays
	// where it is as the slot's distortion's Auto gain is set, and a
	// distortion's Type as its chorus's Mode is.
	_, _, pn, w, run := chainStage(t, []Unit{{Kind: Crush}, {Kind: Distortion, Slot: 1}})
	crush, dist := pn.cards[0], pn.cards[1]
	withUI(t, w, run, func(u *gunim.UI) {
		pn.Follow(UnitParamID(0, DistortAutoParam), 1, u)
		pn.Follow(UnitParamID(1, ChorusModeParam), 2, u)
	})
	if got := crush.toggle.Selected(); got != 0 {
		t.Errorf("the crush's Dither shows %d after the slot's Auto gain is set", got)
	}
	if got := dist.sel.Selected(); got != 0 {
		t.Errorf("the distortion's Type shows %d after the slot's chorus Mode is set", got)
	}
	withUI(t, w, run, func(u *gunim.UI) { pn.Follow(UnitParamID(0, CrushDitherParam), 1, u) })
	if got := crush.toggle.Selected(); got != 1 {
		t.Errorf("the crush's Dither shows %d after it is set on", got)
	}
}
