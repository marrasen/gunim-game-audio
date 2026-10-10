package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// chorusTypes are the kinds of a track's chorus, in the switch's order,
// and chorusNames what the switch calls each.
var (
	chorusTypes = []string{"soft", "juno1", "juno2", "juno12", "ensemble"}
	chorusNames = []string{"soft", "Juno I", "Juno II", "Juno I+II", "ensemble"}
)

// chorusUnit is a track's chorus as one control, as a synth's chorus
// sits on its panel: a knob for how much, and under it a switch of five
// positions for its kind, a lamp each, as round a rotary selector, and
// the kind's name. A chorus turned to 0 is off, and its switch shows it
// dark.
type chorusUnit struct {
	k    *knob
	sw   *chorusSwitch
	kids []gunim.Node
}

func newChorusUnit(base *string) *chorusUnit {
	k := newKnob("Chorus", base, "/Chorus", 0, 1, 0).small()
	sw := &chorusSwitch{hot: -1}
	return &chorusUnit{k: k, sw: sw, kids: []gunim.Node{k, sw}}
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

// show takes the chorus's amount and kind from the track at the
// unit's path, and its colour.
func (c *chorusUnit) show(amount float64, kind string, col color.NRGBA) {
	c.sw.on = amount > 0
	c.sw.kind = max(segmentedIndex(chorusTypes, kind), 0)
	if kind == "" {
		c.sw.kind = 0
	}
	c.sw.color = col
}

// chorusSwitch is the chorus's switch of five positions: a row of
// lamps, the chosen one lit, and the chosen kind's name under them, or
// the name of the one under the pointer. A click on a lamp picks its
// kind, and the wheel steps through them.
type chorusSwitch struct {
	// track names the track it sets.
	track string
	kind  int
	on    bool
	// hot is the position under the pointer, or -1.
	hot   int
	color color.NRGBA
	size  geom.Size
}

func (s *chorusSwitch) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	s.size = c.Constrain(geom.Sz(70, 26))
	return s.size
}

func (s *chorusSwitch) cell() float32 { return s.size.W / float32(len(chorusTypes)) }

func (s *chorusSwitch) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	w := s.cell()
	const cy = 6
	for i := range chorusTypes {
		cx := (float32(i) + 0.5) * w
		chosen := i == s.kind
		// The lamp: lit in the track's colour where chosen and on, grey
		// where chosen and off, faint elsewhere, a little less so under
		// the pointer.
		lamp := withAlpha(ink, 0.14)
		switch {
		case chosen && s.on:
			p.RRect(geom.Rc(cx-5, cy-5, 10, 10), 5, paint.Solid(withAlpha(s.color, 0.3)))
			lamp = s.color
		case chosen:
			lamp = withAlpha(ink, 0.45)
		case i == s.hot:
			lamp = withAlpha(ink, 0.3)
		}
		p.RRect(geom.Rc(cx-3, cy-3, 6, 6), 3, paint.Solid(lamp))
	}
	show, alpha := s.kind, float32(0.9)
	if !s.on {
		alpha = 0.5
	}
	if s.hot >= 0 && s.hot != s.kind {
		show, alpha = s.hot, 0.6
	}
	t := audioui.Shaped(chorusNames[show], 9.5, false, false)
	t.Paint(p, geom.Pt((box.W-t.Advance)/2, 13), withAlpha(ink, alpha))
}

// pick moves the switch to position i and tells the application, which
// turns a silent chorus up so the kind is heard.
func (s *chorusSwitch) pick(i int, u *gunim.UI) {
	i = min(max(i, 0), len(chorusTypes)-1)
	if i == s.kind && s.on {
		return
	}
	s.kind, s.on = i, true
	t := chorusTypes[i]
	if t == "soft" {
		t = ""
	}
	u.Send(s, ChorusKind{Track: s.track, Type: t})
	u.Invalidate()
}

func (s *chorusSwitch) Handle(e input.Event, u *gunim.UI) bool {
	at := func(x float32) int { return min(max(int(x/s.cell()), 0), len(chorusTypes)-1) }
	switch e := e.(type) {
	case input.PointerEnter:
		s.hot = at(e.Pos.X)
		u.Invalidate()
	case input.PointerMove:
		if h := at(e.Pos.X); h != s.hot {
			s.hot = h
			u.Invalidate()
		}
	case input.PointerLeave:
		s.hot = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		s.pick(at(e.Pos.X), u)
	case input.Scroll:
		step := 1
		if e.Delta.Y > 0 {
			step = -1
		}
		s.pick(s.kind+step, u)
	default:
		return false
	}
	return true
}
