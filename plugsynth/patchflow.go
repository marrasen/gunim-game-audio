package plugsynth

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// flow lays cards out in rows from the top, as many to a row as fit,
// each row centred, as a panel's sections stand at its foot. Each card
// stands in a slot kept by its key: built again, a card keeps its
// slot, and where it moves it glides there; a card new to the flow
// fades in where it lands, and a card gone fades out where it stood.
type flow struct {
	cards []*plugui.Section
	// keys name the cards, one for each, as the slots keep them; with
	// none, a card's name is its key.
	keys  []string
	slots map[string]*slot
	// width is how wide the flow was last laid out: at another width
	// the cards stand where they go at once.
	width float32
}

// key is card i's key.
func (f *flow) key(i int) string {
	if i < len(f.keys) {
		return f.keys[i]
	}
	return f.cards[i].Name
}

// Children implements [gunim.Composite].
func (f *flow) Children() []gunim.Node {
	f.fill(true)
	out := make([]gunim.Node, len(f.cards))
	for i := range f.cards {
		out[i] = f.slots[f.key(i)]
	}
	return out
}

// fill gives each card its slot: the one its key had, or a new one,
// shown at once where in says, or else to fade in. Slots of keys gone
// are let go.
func (f *flow) fill(in bool) {
	slots := make(map[string]*slot, len(f.cards))
	for i, c := range f.cards {
		k := f.key(i)
		s := f.slots[k]
		if s == nil {
			s = newSlot(k, in)
		}
		s.card = c
		slots[k] = s
	}
	f.slots = slots
}

// Layout implements [gunim.Node]: as wide as it is given, and as tall
// as its rows.
func (f *flow) Layout(c gunim.Constraints, fr gunim.Frame, kids gunim.Children) geom.Size {
	w := c.Max.W
	jump := w != f.width
	f.width = w
	f.fill(jump)
	// Placed at the foot of a view of no height, the rows' top tells how
	// tall they stand.
	h := panelInset - plugui.PlaceSections(geom.Sz(w, 0), panelInset, f.cards, func(int, geom.Rect) {})
	// The tree holds the slots as they were when it took the children:
	// slots new since come in, and the old ones go out, still standing
	// where they were as they fade.
	have := make(map[gunim.Node]gunim.Child, kids.Len())
	for k := range kids.All {
		if k.Presence() == gunim.Exiting {
			k.Layout(gunim.Tight(k.Size()))
			if s, ok := k.Node().(*slot); ok {
				k.Place(s.at.Value())
			}
			continue
		}
		have[k.Node()] = k
	}
	for n := range have {
		if s, ok := n.(*slot); !ok || f.slots[s.key] != s {
			kids.Drop(n)
		}
	}
	glide := widget.Settle.Get(fr.Theme)
	plugui.PlaceSections(geom.Sz(w, h), panelInset, f.cards, func(i int, r geom.Rect) {
		s := f.slots[f.key(i)]
		k, ok := have[s]
		if !ok {
			k = kids.Build(s)
		}
		if jump || !s.placed {
			s.at.Jump(r.Min)
		} else {
			s.at.Animate(r.Min, glide)
		}
		s.placed = true
		k.Layout(gunim.Tight(r.Size()))
		k.Place(s.at.Value())
	})
	return geom.Sz(w, h)
}

// Paint implements [gunim.Node]: the cards going out, under the cards
// that stay, so a card gliding into a place passes over the one that
// leaves it.
func (f *flow) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for _, out := range []bool{true, false} {
		for k := range kids.All {
			if (k.Presence() == gunim.Exiting) == out {
				k.Paint(p)
			}
		}
	}
}

// slot holds a card of a flow: where it stands, gliding as the flow
// moves it, and how far it has come in, as it fades in and out.
type slot struct {
	anim.Group
	key    string
	card   *plugui.Section
	at     *anim.Point
	in     *anim.Float
	placed bool
}

// newSlot returns an empty slot of key, in at once where in says, or
// else to fade in.
func newSlot(key string, in bool) *slot {
	s := &slot{key: key, at: anim.NewPoint(geom.Point{}), in: anim.NewFloat(0)}
	if in {
		s.in.Jump(1)
	}
	s.Add(s.at, s.in)
	return s
}

// Children implements [gunim.Composite].
func (s *slot) Children() []gunim.Node { return []gunim.Node{s.card} }

// Layout implements [gunim.Node]: the card, as big as the slot. A card
// built again comes in, and the old one goes out.
func (s *slot) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	var card gunim.Child
	found := false
	for k := range kids.All {
		switch {
		case k.Presence() == gunim.Exiting:
		case k.Node() == gunim.Node(s.card):
			card, found = k, true
		default:
			kids.Drop(k.Node())
		}
	}
	if !found {
		card = kids.Build(s.card)
	}
	card.Layout(gunim.Tight(c.Max))
	card.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node]: the card, faded and grown from a
// little smaller about its middle as it comes in.
func (s *slot) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	in := min(max(s.in.Value(), 0), 1)
	if in <= 0 {
		return
	}
	if in < 1 {
		r := geom.Rc(0, 0, box.W, box.H)
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Min: r.Min.Sub(geom.Pt(16, 16)), Max: r.Max.Add(geom.Pt(16, 16))}, Opacity: in})()
		defer p.Push(paint.Scale(0.92+0.08*in, geom.Pt(box.W/2, box.H/2)))()
	}
	for k := range kids.All {
		if k.Node() == gunim.Node(s.card) {
			k.Paint(p)
		}
	}
}

// Transition implements [gunim.Transitioner]: the card fades in as it
// comes, and out as it goes.
func (s *slot) Transition(pr gunim.Presence, f gunim.Frame) bool {
	switch pr {
	case gunim.Entering:
		s.in.Animate(1, widget.Quick.Get(f.Theme))
	case gunim.Exiting:
		s.in.Animate(0, widget.Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !s.in.Active()
}
