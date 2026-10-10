package main

import (
	"image/color"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// gateSteps is how many steps a trance gate set here has: a bar's 16ths.
const gateSteps = 16

// gateRow is a track's trance gate as a row of steps over a bar, lit
// where the track sounds and dark where the gate shuts it; a click
// turns a step over. With no gate, every step is open, faint.
type gateRow struct {
	track string
	steps []bool
	on    bool
	color color.NRGBA
	hot   int
	size  geom.Size
}

// show shows gate, the track's, as steps.
func (g *gateRow) show(track, gate string, col color.NRGBA) {
	g.track, g.color = track, col
	g.steps = g.steps[:0]
	for _, r := range gate {
		switch r {
		case 'x', 'X', '1':
			g.steps = append(g.steps, true)
		case '.', '-', '0', '_':
			g.steps = append(g.steps, false)
		}
	}
	g.on = len(g.steps) > 0
	if !g.on {
		for range gateSteps {
			g.steps = append(g.steps, true)
		}
	}
}

func (g *gateRow) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	w := c.Max.W
	if w > 1e4 {
		w = 400
	}
	g.size = c.Constrain(geom.Sz(w, 26))
	return g.size
}

func (g *gateRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	n := max(len(g.steps), 1)
	cw := box.W / float32(n)
	for i, on := range g.steps {
		r := geom.Rc(float32(i)*cw+1.5, 2, cw-3, box.H-4)
		fill := withAlpha(ink, 0.06)
		switch {
		case on && g.on:
			fill = withAlpha(g.color, 0.85)
		case on:
			fill = withAlpha(ink, 0.14)
		}
		if i%4 == 0 && !(on && g.on) {
			fill = withAlpha(ink, 0.1)
		}
		p.RRect(r, 4, paint.Solid(fill))
		if i == g.hot {
			p.RRect(r, 4, paint.Solid(withAlpha(ink, 0.08)))
		}
	}
}

func (g *gateRow) at(x float32) int {
	n := max(len(g.steps), 1)
	return min(max(int(x/(g.size.W/float32(n))), 0), n-1)
}

func (g *gateRow) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if h := g.at(e.Pos.X); h != g.hot {
			g.hot = h
			u.Invalidate()
		}
		return true
	case input.PointerLeave:
		g.hot = -1
		u.Invalidate()
		return true
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := g.at(e.Pos.X)
		g.steps[i] = !g.steps[i]
		g.on = true
		var b strings.Builder
		for _, on := range g.steps {
			if on {
				b.WriteByte('x')
			} else {
				b.WriteByte('.')
			}
		}
		u.Send(g, SetValue{Path: "Tracks/" + g.track + "/Gate", Str: b.String(), IsStr: true})
		u.Invalidate()
		return true
	}
	return false
}
