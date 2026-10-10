package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// chorusTypes are the kinds of a track's chorus, in the switch's order,
// and chorusNames what the switch calls each.
var (
	chorusTypes = []string{"soft", "juno1", "juno2", "juno12", "ensemble"}
	chorusNames = []string{"soft", "Juno I", "Juno II", "Juno I+II", "ensemble"}
)

// chorusUnit is a track's chorus as one control, as a synth's chorus
// sits on its panel: a knob for how much, and under it a compact
// selector of its five kinds. A chorus turned to 0 is off, and its
// selector shows it grey; a kind picked on a chorus that is off turns
// it up, so it is heard.
type chorusUnit struct {
	k     *knob
	sw    *selector
	track string
	kids  []gunim.Node
}

func newChorusUnit(base *string) *chorusUnit {
	c := &chorusUnit{k: newKnob("Chorus", base, "/Chorus", 0, 1, 0).small()}
	c.sw = newSelector(chorusNames, func(i int) gunim.Intent {
		t := chorusTypes[i]
		if t == "soft" {
			t = ""
		}
		return ChorusKind{Track: c.track, Type: t}
	}).small()
	c.kids = []gunim.Node{c.k, c.sw}
	return c
}

func (c *chorusUnit) Children() []gunim.Node { return c.kids }

func (c *chorusUnit) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
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

func (c *chorusUnit) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	// One panel round both, so they read as one control.
	p.RRect(geom.Rc(0, 0, box.W, box.H), 6, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x08}))
	for i := range kids.Len() {
		kids.At(i).Paint(p)
	}
}

// show shows track's chorus: its amount, its kind and the track's
// colour.
func (c *chorusUnit) show(track string, amount float64, kind string, col color.NRGBA) {
	c.track = track
	c.sw.off = amount <= 0
	c.sw.SetSelected(segmentedIndex(chorusTypes, kind))
	c.sw.color = col
}
