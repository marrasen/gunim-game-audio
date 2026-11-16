package synth

import (
	"math"
	"slices"
	"testing"
)

// renderDrum renders drum d, named name in the default kit, struck at
// vel, for frames frames.
func renderDrum(t *testing.T, name string, d Drum, frames int) []float32 {
	t.Helper()
	c, err := (&Patch{Kind: "drums", Kit: map[string]Drum{name: d}}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	hs := newHits(1)
	hs.play(c.kit[name], 0.9, 0, 40, rate/2, 0)
	l, r := make([]float32, frames), make([]float32, frames)
	hs.render(l, r, true)
	return l
}

func shapeOf(t *testing.T, name string, d Drum) DrumShape {
	t.Helper()
	s, err := ShapeOf(d, name, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// fieldSet sets field f of d to v.
func fieldSet(d *Drum, f DrumFields, v float64) {
	switch f {
	case FieldSweep:
		d.Sweep = v
	case FieldSweepTime:
		d.SweepTime = 1 + v
	case FieldClick:
		d.Click = v
	case FieldNoise:
		d.Noise = v
	case FieldSpread:
		d.Spread = v
	}
}

var allFields = []DrumFields{FieldSweep, FieldSweepTime, FieldClick, FieldNoise, FieldSpread}

func TestEachDrumHearsTheDeepSettingsItSaysItReadsAndNoOthers(t *testing.T) {
	names := make([]string, 0, len(drumNames))
	for n := range drumNames {
		names = append(names, n)
	}
	slices.Sort(names)
	seen := map[string]bool{}
	for _, name := range names {
		typ := drumNames[name]
		if seen[typ] {
			continue
		}
		seen[typ] = true
		reads := deepKinds[drumTypes[typ]].reads
		plain := renderDrum(t, name, Drum{}, rate/2)
		for _, f := range allFields {
			var d Drum
			fieldSet(&d, f, 0.7)
			got := renderDrum(t, name, d, rate/2)
			if differs := !slices.Equal(got, plain); differs != (reads&f != 0) {
				t.Errorf("%s (%s): setting %d changes its sound %v; it says it reads it %v", name, typ, f, differs, reads&f != 0)
			}
		}
	}
}

func TestSweepMovesHowFarTheDiveStartsAboveThePitch(t *testing.T) {
	for _, name := range []string{"bd", "lt", "bd9", "tr8bd", "tr9bd", "sdsbd", "tr8lt", "sdslt", "mbd", "mlt", "syn1", "sbd", "nbd"} {
		plain := shapeOf(t, name, Drum{})
		up := shapeOf(t, name, Drum{Sweep: 1})
		end := plain.Pitch[len(plain.Pitch)-1]
		if r := up.Pitch[len(up.Pitch)-1] / end; math.Abs(float64(r-1)) > 0.03 {
			t.Errorf("%s ends at %.1f Hz with Sweep 1, at %.1f Hz without; want the same", name, up.Pitch[len(up.Pitch)-1], end)
		}
		if up.Pitch[0] < plain.Pitch[0]*1.5 {
			t.Errorf("%s starts at %.1f Hz with Sweep 1, at %.1f Hz without; want it far higher", name, up.Pitch[0], plain.Pitch[0])
		}
		down := shapeOf(t, name, Drum{Sweep: -2})
		if down.Pitch[0] > plain.Pitch[0] {
			t.Errorf("%s starts at %.1f Hz with Sweep -2, at %.1f Hz without; want it lower", name, down.Pitch[0], plain.Pitch[0])
		}
	}
}

func TestAPlainDiveStartsAnOctaveHigherWithSweepOne(t *testing.T) {
	plain := shapeOf(t, "bd", Drum{})
	up := shapeOf(t, "bd", Drum{Sweep: 1})
	if r := up.Pitch[0] / plain.Pitch[0]; math.Abs(float64(r-2)) > 0.02 {
		t.Errorf("the kick starts %.3f times as high with Sweep 1; want 2", r)
	}
}

func TestSweepTimeStretchesTheDive(t *testing.T) {
	for _, name := range []string{"bd", "bd9", "tr8bd", "tr9bd", "sdsbd", "mbd", "mlt", "tr9lt"} {
		plain := shapeOf(t, name, Drum{})
		slow := shapeOf(t, name, Drum{SweepTime: 3})
		fast := shapeOf(t, name, Drum{SweepTime: 0.25})
		at := 8
		if !(slow.Pitch[at] > plain.Pitch[at] && plain.Pitch[at] > fast.Pitch[at]) {
			t.Errorf("%s 8 ms on: %.1f Hz slow, %.1f plain, %.1f fast; want them falling in that order", name, slow.Pitch[at], plain.Pitch[at], fast.Pitch[at])
		}
	}
}

func TestClickTurnsTheClickUpAndDown(t *testing.T) {
	for _, name := range []string{"bd", "lt", "bd9", "tr8bd", "tr9bd", "sdsbd", "tr8lt", "sdssn", "mbd", "mmt", "msn", "mcr", "mhh", "mtl"} {
		peak := func(c float64) float32 { return slices.Max(shapeOf(t, name, Drum{Click: c}).Click[:3]) }
		none, own, loud := peak(-1), peak(0), peak(1)
		if none != 0 || !(loud > own*1.3) {
			t.Errorf("%s click peaks at %.4f, %.4f and %.4f for Click -1, 0 and 1; want none, then louder", name, none, own, loud)
		}
	}
}

func TestNoiseLeansTheSoundBetweenBodyAndNoise(t *testing.T) {
	for _, name := range []string{"sn", "tr8sn", "tr9sn", "sdssn", "msn", "mmt", "hh", "cr", "mhh", "mcr", "ssn", "tamb"} {
		parts := func(n float64) (b, x float32) {
			s := shapeOf(t, name, Drum{Noise: n})
			return slices.Max(s.Body), slices.Max(s.Noise)
		}
		b0, n0 := parts(0)
		b1, n1 := parts(1)
		if !(n1 > n0*1.2) || !(b1 < b0) {
			t.Errorf("%s with Noise 1: body %.3f (was %.3f), noise %.3f (was %.3f); want less body, more noise", name, b1, b0, n1, n0)
		}
	}
}

func TestSpreadMovesThePartialsApart(t *testing.T) {
	for _, name := range []string{"hh", "cr", "cb", "mtl", "tamb", "tr8hh", "tr8cy", "tr8cb", "mcr", "tr9rd"} {
		width := func(sp float64) float32 {
			p := shapeOf(t, name, Drum{Spread: sp}).Partials
			return p[len(p)-1] / p[0]
		}
		if !(width(1) > width(0) && width(0) > width(-1)) {
			t.Errorf("%s's partials span %.2f, %.2f and %.2f for Spread 1, 0 and -1; want them narrowing", name, width(1), width(0), width(-1))
		}
	}
}

func TestTheShapesPitchIsThePitchHeard(t *testing.T) {
	// The kick's body, alone, counted by its rises through 0 over a
	// stretch where its pitch has settled.
	for _, name := range []string{"bd", "tr8bd", "tr9bd", "mbd"} {
		s := shapeOf(t, name, Drum{})
		x := renderDrum(t, name, Drum{Noise: 0}, rate/4)
		from, to := rate*80/1000, rate*200/1000
		ups := 0
		for i := from + 1; i < to; i++ {
			if x[i-1] < 0 && x[i] >= 0 {
				ups++
			}
		}
		heard := float32(ups) / (float32(to-from) / rate)
		var mean float32
		for _, f := range s.Pitch[80:200] {
			mean += f
		}
		mean /= 120
		if r := heard / mean; r < 0.9 || r > 1.1 {
			t.Errorf("%s: the shape says %.1f Hz, the sound rises through 0 %.1f times a second", name, mean, heard)
		}
	}
}

func TestTheShapesLoudnessFallsAsDecaySays(t *testing.T) {
	fall := func(d Drum) int {
		b := shapeOf(t, "bd", d).Body
		top := slices.Max(b)
		for i := slices.Index(b, top); i < len(b); i++ {
			if b[i] < top/10 {
				return i
			}
		}
		return len(b)
	}
	short, long := fall(Drum{}), fall(Drum{Decay: 2})
	if r := float64(long) / float64(short); r < 1.6 || r > 2.4 {
		t.Errorf("the kick's body falls 20 dB in %d ms, with Decay 2 in %d ms; want about twice as long", short, long)
	}
}

func TestAChipDrumsShapeSteps(t *testing.T) {
	s := shapeOf(t, "sbd", Drum{})
	if s.Step != 50 {
		t.Errorf("the SID kick steps %v times a second; want 50", s.Step)
	}
	if n := shapeOf(t, "nbd", Drum{Decay: 2}).Step; n != 30 {
		t.Errorf("the NES kick at Decay 2 steps %v times a second; want 30", n)
	}
}

func TestShapeOfAnUnknownDrumFails(t *testing.T) {
	if _, err := ShapeOf(Drum{Type: "kazoo"}, "x", 1); err == nil {
		t.Error("a kazoo has a shape; want an error")
	}
}
