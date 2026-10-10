package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// chorusTypes are the kinds of a track's chorus, in the switch's order,
// and chorusNames what the switch calls each; distortTypes and
// distortNames are its distortion's.
var (
	chorusTypes  = []string{"soft", "juno1", "juno2", "juno12", "ensemble"}
	chorusNames  = []string{"soft", "Juno I", "Juno II", "Juno I+II", "ensemble"}
	distortTypes = []string{"fuzz", "amp", "fold"}
	distortNames = []string{"fuzz", "amp", "fold"}
	washTypes    = []string{"hall", "spring"}
	washNames    = []string{"hall", "spring"}
)

// kindUnit is an effect of a track's as one control, as a synth's
// chorus sits on its panel: a knob for how much, and under it a compact
// selector of its kinds, the first the default. An effect turned to 0
// is off, and its selector shows it grey; a kind picked on an effect
// that is off turns it up, so it is heard.
type kindUnit struct {
	k     *knob
	sw    *selector
	types []string
	track string
	kids  []gunim.Node
}

// newKindUnit returns a unit named label for the amount at rel under
// base, of the kinds types, called names, picked by the intent pick
// returns for a track and a kind.
func newKindUnit(label string, base *string, rel string, types, names []string, pick func(track, kind string) gunim.Intent) *kindUnit {
	c := &kindUnit{k: newKnob(label, base, rel, 0, 1, 0).small(), types: types}
	c.sw = newSelector(names, func(i int) gunim.Intent {
		t := types[i]
		if i == 0 {
			t = ""
		}
		return pick(c.track, t)
	}).small()
	c.kids = []gunim.Node{c.k, c.sw}
	return c
}

// newChorusUnit and newDistortUnit return a track's chorus and its
// distortion as units.
func newChorusUnit(base *string) *kindUnit {
	return newKindUnit("Chorus", base, "/Chorus", chorusTypes, chorusNames, func(track, kind string) gunim.Intent {
		return ChorusKind{Track: track, Type: kind}
	})
}

// newWashUnit returns a track's wash, the reverb before its distortion,
// as a unit.
func newWashUnit(base *string) *kindUnit {
	return newKindUnit("Wash", base, "/Wash", washTypes, washNames, func(track, kind string) gunim.Intent {
		return WashKind{Track: track, Type: kind}
	})
}

func newDistortUnit(base *string) *kindUnit {
	return newKindUnit("Distort", base, "/Distort", distortTypes, distortNames, func(track, kind string) gunim.Intent {
		return DistortKind{Track: track, Type: kind}
	})
}

func (c *kindUnit) Children() []gunim.Node { return c.kids }

func (c *kindUnit) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	const pad = 3
	w := max(cs.Max.W, 44+2*pad)
	if w > 1e4 {
		w = 76
	}
	k := kids.At(0).Layout(gunim.Loose(geom.Sz(44, 56)))
	kids.At(0).Place(geom.Pt((w-k.W)/2, pad))
	sw := kids.At(1).Layout(gunim.Tight(geom.Sz(w-2*pad, 26)))
	kids.At(1).Place(geom.Pt(pad, pad+k.H))
	return cs.Constrain(geom.Sz(w, pad+k.H+sw.H+pad))
}

func (c *kindUnit) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	// One panel round both, so they read as one control.
	p.RRect(geom.Rc(0, 0, box.W, box.H), 6, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x08}))
	for i := range kids.Len() {
		kids.At(i).Paint(p)
	}
}

// show shows track's effect: its amount, its kind and the track's
// colour.
func (c *kindUnit) show(track string, amount float64, kind string, col color.NRGBA) {
	c.track = track
	c.sw.off = amount <= 0
	c.sw.SetSelected(max(segmentedIndex(c.types, kind), 0))
	c.sw.color = col
}
