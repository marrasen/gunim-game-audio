package main

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// switcher shows one of its children, which, and lays the rest out at
// no size, out of the way, where they take no input.
type switcher struct {
	kids  []gunim.Node
	which int
}

func newSwitcher(kids ...gunim.Node) *switcher { return &switcher{kids: kids} }

func (s *switcher) Children() []gunim.Node { return s.kids }

func (s *switcher) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	var size geom.Size
	for i := range kids.Len() {
		k := kids.At(i)
		if i == s.which {
			size = k.Layout(c)
			k.Place(geom.Point{})
			continue
		}
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Pt(-1e5, -1e5))
	}
	return c.Constrain(size)
}

func (s *switcher) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	if s.which < kids.Len() {
		kids.At(s.which).Paint(p)
	}
}

// strip lays out the first n of its children side by side, each as wide
// as the room shared, up to maxW, and hides the rest.
type strip struct {
	kids []gunim.Node
	n    int
	maxW float32
	gap  float32
}

func (s *strip) Children() []gunim.Node { return s.kids }

func (s *strip) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	n := min(s.n, kids.Len())
	w := s.maxW
	if n > 0 {
		w = min(w, (c.Max.W-s.gap*float32(n-1))/float32(n))
	}
	for i := range kids.Len() {
		k := kids.At(i)
		if i < n {
			k.Layout(gunim.Tight(geom.Sz(w, c.Max.H)))
			k.Place(geom.Pt(float32(i)*(w+s.gap), 0))
			continue
		}
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Pt(-1e5, -1e5))
	}
	return geom.Sz(c.Max.W, c.Max.H)
}

func (s *strip) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for i := range min(s.n, kids.Len()) {
		kids.At(i).Paint(p)
	}
}

// side holds a panel of a fixed width that slides away to nothing, and
// back, as open says.
type side struct {
	child gunim.Node
	width float32
	open  bool
	shown float32
}

func newSide(child gunim.Node, width float32) *side {
	return &side{child: child, width: width, open: true, shown: width}
}

func (s *side) Children() []gunim.Node { return []gunim.Node{s.child} }

func (s *side) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	k.Layout(gunim.Tight(geom.Sz(s.width, c.Max.H)))
	k.Place(geom.Pt(s.width-s.shown, 0))
	return geom.Sz(s.shown, c.Max.H)
}

func (s *side) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	if s.shown < 1 {
		return
	}
	end := p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, 0, box.W, box.H), Opacity: min(1, s.shown/s.width*1.5), Clip: true})
	kids.At(0).Paint(p)
	end()
}

func (s *side) Step(dt time.Duration) bool {
	target := float32(0)
	if s.open {
		target = s.width
	}
	s.shown += (target - s.shown) * min(1, float32(dt.Seconds())*12)
	if abs(target-s.shown) < 0.5 {
		s.shown = target
	}
	return s.shown != target
}

// panel is a card with a title over what it holds, as a synth's section.
func panel(title string, kids ...gunim.Node) *widget.Card {
	head := widget.NewLabel(title)
	head.Size, head.Color = smallSize, widget.MenuHint
	col := widget.Column(append([]gunim.Node{head}, kids...)...)
	col.Cross = widget.CrossStretch
	c := widget.NewCard(col)
	c.Fill = cardFill
	return c
}

// panelWith is a card whose title row holds more, as a choice of wave.
func panelWith(head gunim.Node, kids ...gunim.Node) *widget.Card {
	col := widget.Column(append([]gunim.Node{head}, kids...)...)
	col.Cross = widget.CrossStretch
	c := widget.NewCard(col)
	c.Fill = cardFill
	return c
}

// titled returns a row of a small title, a gap, and the rest at the
// right.
func titled(title string, rest ...gunim.Node) *widget.Flex {
	head := small(title)
	gap := widget.NewSpacer()
	r := widget.Row(append([]gunim.Node{head, gap}, rest...)...).Grow(gap, 1)
	r.Cross = widget.CrossCenter
	return r
}

// sized holds n to w by h.
func sized(n gunim.Node, w, h float32) *widget.Sized { return widget.NewSized(n, w, h) }

// segmentedIndex returns the index of v in options, 0 where it is none.
func segmentedIndex(options []string, v string) int {
	for i, o := range options {
		if o == v {
			return i
		}
	}
	return 0
}

// slider shows its child sliding in from one side and fading in, as a
// page turned to the next or the one before; laid out where it rests,
// it is only drawn moving, so a click lands where it will be.
type slider struct {
	child gunim.Node
	// t runs from 0, the slide begun, to 1, at rest; dir is the side it
	// comes from, 1 the right and -1 the left.
	t, dir float32
}

// slideDur is how long a slide takes, and slideBy how far it comes.
const (
	slideDur = 260 * time.Millisecond
	slideBy  = 56
)

func newSlider(child gunim.Node) *slider { return &slider{child: child, t: 1} }

// start slides the child in from the right where dir is 1, or the left
// where it is -1.
func (s *slider) start(dir float32) { s.t, s.dir = 0, dir }

func (s *slider) Children() []gunim.Node { return []gunim.Node{s.child} }

func (s *slider) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	size := k.Layout(c)
	k.Place(geom.Pt(0, 0))
	return size
}

func (s *slider) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	if s.t >= 1 {
		kids.At(0).Paint(p)
		return
	}
	// Eased out: quick at first, settling into place.
	e := 1 - (1-s.t)*(1-s.t)*(1-s.t)
	end := p.Layer(paint.LayerOpts{Bounds: geom.Rc(-slideBy, 0, box.W+2*slideBy, box.H), Opacity: 0.25 + 0.75*e})
	pop := p.Push(paint.Translate(geom.Pt(s.dir*slideBy*(1-e), 0)))
	kids.At(0).Paint(p)
	pop()
	end()
}

func (s *slider) Step(dt time.Duration) bool {
	if s.t >= 1 {
		return false
	}
	s.t = min(1, s.t+float32(dt)/float32(slideDur))
	return true
}
