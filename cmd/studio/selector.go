package main

import (
	"image/color"
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// selector picks one of a few options, as a synth's panel does, with a
// lamp an option, the one chosen lit. Labelled, each lamp has its name
// beside it, as printed on a panel, the options in rows as the room
// allows, a group starting a row of its own. Compact, the lamps stand
// in one row and the chosen one's name shows under them, or the name
// of the one under the pointer. A click on a lamp picks its option,
// and the wheel steps through them.
type selector struct {
	names []string
	// groups are the options that start a row of their own.
	groups []int
	// compact lays the lamps in one row, the chosen name under them.
	compact bool
	sel     int
	// off shows the chosen lamp grey, as a chorus turned to 0.
	off   bool
	hot   int
	color color.NRGBA
	// pick returns the intent for picking option i.
	pick  func(i int) gunim.Intent
	cells []geom.Rect
	size  geom.Size
}

const (
	selectorText = 9.5
	selectorRow  = 17
)

func newSelector(names []string, pick func(i int) gunim.Intent) *selector {
	return &selector{names: names, pick: pick, hot: -1, color: color.NRGBA{0x5e, 0x9c, 0xff, 0xff}}
}

// group starts a row of its own at each option of at.
func (s *selector) group(at ...int) *selector { s.groups = at; return s }

// small makes the selector compact, for a mixer's narrow strip.
func (s *selector) small() *selector { s.compact = true; return s }

// Selected returns the option chosen.
func (s *selector) Selected() int { return s.sel }

// SetSelected chooses option i, as the song has it.
func (s *selector) SetSelected(i int) { s.sel = min(max(i, 0), len(s.names)-1) }

func (s *selector) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	w := c.Max.W
	if w > 1e4 {
		w = 320
	}
	s.cells = s.cells[:0]
	if s.compact {
		cw := w / float32(len(s.names))
		for i := range s.names {
			s.cells = append(s.cells, geom.Rc(float32(i)*cw, 0, cw, 13))
		}
		s.size = c.Constrain(geom.Sz(w, 26))
		return s.size
	}
	// The options in rows, a group starting its own, as many columns as
	// fit, each column as wide as its widest name.
	widths := make([]float32, len(s.names))
	for i, n := range s.names {
		widths[i] = audioui.Shaped(n, selectorText, true, false).Advance + 22
	}
	var cols []float32
	place := func(n int) (rows [][]int) {
		var row []int
		for i := range s.names {
			if len(row) == n || (len(row) > 0 && slices.Contains(s.groups, i)) {
				rows, row = append(rows, row), nil
			}
			row = append(row, i)
		}
		return append(rows, row)
	}
	var rows [][]int
	for n := len(s.names); n >= 1; n-- {
		rows = place(n)
		cols = make([]float32, n)
		for _, r := range rows {
			for c, i := range r {
				cols[c] = max(cols[c], widths[i])
			}
		}
		var total float32
		for _, cw := range cols {
			total += cw
		}
		if total <= w || n == 1 {
			break
		}
	}
	var y, right float32
	for _, r := range rows {
		var x float32
		for c := range r {
			s.cells = append(s.cells, geom.Rc(x, y, cols[c], selectorRow))
			x += cols[c]
		}
		right = max(right, x)
		y += selectorRow
	}
	y -= selectorRow
	s.size = c.Constrain(geom.Sz(min(right, w), y+selectorRow))
	return s.size
}

func (s *selector) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	for i, r := range s.cells {
		chosen := i == s.sel
		cx, cy := (r.Min.X+r.Max.X)/2, r.Min.Y+6.5
		if !s.compact {
			cx, cy = r.Min.X+7, (r.Min.Y+r.Max.Y)/2
		}
		// The lamp: lit in its colour where chosen, grey where chosen
		// and off, faint elsewhere, less so under the pointer.
		lamp := withAlpha(ink, 0.14)
		switch {
		case chosen && !s.off:
			p.RRect(geom.Rc(cx-5, cy-5, 10, 10), 5, paint.Solid(withAlpha(s.color, 0.3)))
			lamp = s.color
		case chosen:
			lamp = withAlpha(ink, 0.45)
		case i == s.hot:
			lamp = withAlpha(ink, 0.3)
		}
		p.RRect(geom.Rc(cx-3, cy-3, 6, 6), 3, paint.Solid(lamp))
		if s.compact {
			continue
		}
		alpha := float32(0.5)
		switch {
		case chosen:
			alpha = 0.95
		case i == s.hot:
			alpha = 0.75
		}
		t := audioui.Shaped(s.names[i], selectorText, chosen, false)
		t.Paint(p, geom.Pt(r.Min.X+15, cy-t.Height()/2), withAlpha(ink, alpha))
	}
	if s.compact {
		show, alpha := s.sel, float32(0.9)
		if s.off {
			alpha = 0.5
		}
		if s.hot >= 0 && s.hot != s.sel {
			show, alpha = s.hot, 0.6
		}
		t := audioui.Shaped(s.names[show], selectorText, false, false)
		t.Paint(p, geom.Pt((box.W-t.Advance)/2, 13), withAlpha(ink, alpha))
	}
}

// at returns the option at pos, or -1.
func (s *selector) at(pos geom.Point) int {
	for i, r := range s.cells {
		if s.compact && pos.X >= r.Min.X && pos.X < r.Max.X {
			return i
		}
		if r.Contains(pos) {
			return i
		}
	}
	return -1
}

// choose picks option i and tells the application.
func (s *selector) choose(i int, u *gunim.UI) {
	if i < 0 || i >= len(s.names) || (i == s.sel && !s.off) {
		return
	}
	s.sel, s.off = i, false
	if s.pick != nil {
		if in := s.pick(i); in != nil {
			u.Send(s, in)
		}
	}
	u.Invalidate()
}

func (s *selector) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		s.hot = s.at(e.Pos)
		u.Invalidate()
	case input.PointerMove:
		if h := s.at(e.Pos); h != s.hot {
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
		wheel.touch(s)
		s.choose(s.at(e.Pos), u)
	case input.Scroll:
		if !wheel.allows(s, e) {
			return false
		}
		step := 1
		if e.Delta.Y > 0 {
			step = -1
		}
		s.choose(min(max(s.sel+step, 0), len(s.names)-1), u)
	default:
		return false
	}
	return true
}
