package main

import (
	"image/color"
	"math"
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/calls"
)

// The readouts' colours.
var (
	scopeColor = color.NRGBA{0x5f, 0xf2, 0xd6, 0xff}
	knobColor  = color.NRGBA{0x5e, 0x9c, 0xff, 0xff}
	bandColor  = color.NRGBA{0xff, 0xc8, 0x57, 0xff}
	lowColor   = color.NRGBA{0xff, 0x6b, 0x5f, 0xff}
)

func withAlpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(min(max(a, 0), 1) * 255)
	return c
}

func lighten(c color.NRGBA, t float32) color.NRGBA {
	m := func(x uint8) uint8 { return uint8(float32(x) + (255-float32(x))*t) }
	return color.NRGBA{m(c.R), m(c.G), m(c.B), c.A}
}

// screen is the dark glass a readout draws on.
func screen(p *paint.Painter, r geom.Rect) {
	p.DrawRRect(paint.RRectOp{Rect: r, Radius: 8, Fill: paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y),
		Start: color.NRGBA{0x0b, 0x10, 0x14, 0xff}, End: color.NRGBA{0x07, 0x0a, 0x0d, 0xff}}},
		Inset: [2]paint.Shadow{{Offset: geom.Pt(0, 2), Blur: 6, Color: color.NRGBA{A: 0xb0}}}})
}

// knob sets a number, as a synth's knob does: a drag up turns it up,
// finer with Shift; the wheel steps it; a double click sets it back to
// where it starts. Its value shows under its name.
type knob struct {
	layer int
	p     ParamView
	v     float64
	held  bool
	from  geom.Point
	start float64
}

const knobW, knobH = 58, 70

func (k *knob) show(layer int, p ParamView) {
	k.layer = layer
	if !k.held || k.p.Name != p.Name {
		k.v = p.Value
	}
	k.p = p
}

// at returns where v lies on the knob's turn, from 0 to 1.
func (k *knob) at(v float64) float64 {
	if k.p.Log && k.p.Lo > 0 {
		return math.Log(max(v, k.p.Lo)/k.p.Lo) / math.Log(k.p.Hi/k.p.Lo)
	}
	return (v - k.p.Lo) / (k.p.Hi - k.p.Lo)
}

// value returns the value at t on the knob's turn.
func (k *knob) value(t float64) float64 {
	t = min(max(t, 0), 1)
	var v float64
	if k.p.Log && k.p.Lo > 0 {
		v = k.p.Lo * math.Pow(k.p.Hi/k.p.Lo, t)
	} else {
		v = k.p.Lo + (k.p.Hi-k.p.Lo)*t
	}
	return k.param().Snap(v)
}

// param returns the knob's number as package calls has it, to step it
// as the engine does.
func (k *knob) param() calls.Param {
	return calls.Param{Lo: k.p.Lo, Hi: k.p.Hi, Step: k.p.Step, Odd: k.p.Odd}
}

func (k *knob) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(knobW, knobH))
}

func (k *knob) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	r, text := float32(15), float32(10.5)
	c := geom.Pt(box.W/2, 4+r)
	const from, sweep = 0.75 * math.Pi, 1.5 * math.Pi
	arc := func(t0, t1 float64, w float32, col color.NRGBA) {
		if t1 < t0 {
			t0, t1 = t1, t0
		}
		n := max(int((t1-t0)*28), 1)
		for i := range n {
			a0 := from + sweep*(t0+(t1-t0)*float64(i)/float64(n))
			a1 := from + sweep*(t0+(t1-t0)*float64(i+1)/float64(n))
			audioui.Segment(p, geom.Pt(c.X+r*float32(math.Cos(a0)), c.Y+r*float32(math.Sin(a0))),
				geom.Pt(c.X+r*float32(math.Cos(a1)), c.Y+r*float32(math.Sin(a1))), w, col)
		}
	}
	arc(0, 1, 3.5, color.NRGBA{0xff, 0xff, 0xff, 0x16})
	t := min(max(k.at(k.v), 0), 1)
	start := 0.0
	if k.p.Lo < 0 && k.p.Hi > 0 {
		start = k.at(0)
	}
	glow := knobColor
	if k.held {
		glow = lighten(knobColor, 0.3)
	}
	arc(start, t, 3.5, glow)
	p.ShadowRRect(geom.Rc(c.X-r+6, c.Y-r+6, 2*r-12, 2*r-12), r-6, paint.Fill{Gradient: &paint.Gradient{
		From: geom.Pt(c.X, c.Y-r), To: geom.Pt(c.X, c.Y+r), Start: color.NRGBA{0x44, 0x4a, 0x5c, 0xff}, End: color.NRGBA{0x22, 0x26, 0x31, 0xff}}},
		paint.Shadow{Offset: geom.Pt(0, 2), Blur: 4, Color: color.NRGBA{A: 0x90}})
	a := from + sweep*t
	audioui.Segment(p, geom.Pt(c.X+(r-12)*float32(math.Cos(a)), c.Y+(r-12)*float32(math.Sin(a))),
		geom.Pt(c.X+(r-7)*float32(math.Cos(a)), c.Y+(r-7)*float32(math.Sin(a))), 2, ink)
	name := audioui.Shaped(k.p.Label, text, false, false)
	name.Paint(p, geom.Pt((box.W-name.Advance)/2, c.Y+r+3), withAlpha(ink, 0.6))
	val := audioui.Shaped(k.format(), text, false, true)
	col := withAlpha(ink, 0.9)
	if k.held {
		col = lighten(knobColor, 0.5)
	}
	val.Paint(p, geom.Pt((box.W-val.Advance)/2, c.Y+r+3+name.Height()), col)
}

// format writes the knob's value, as short as reads well.
func (k *knob) format() string {
	v := k.v
	if i := int(math.Round(v)); len(k.p.Choices) > 0 && i >= 0 && i < len(k.p.Choices) {
		return k.p.Choices[i]
	}
	if k.p.Zero != "" && v == 0 {
		return k.p.Zero
	}
	var s string
	switch {
	case k.p.Step >= 1:
		s = strconv.Itoa(int(math.Round(v)))
	case math.Abs(v) >= 1000:
		s = strconv.FormatFloat(v/1000, 'f', 2, 64) + "k"
	case math.Abs(v) >= 100:
		s = strconv.FormatFloat(v, 'f', 0, 64)
	case math.Abs(v) >= 10:
		s = strconv.FormatFloat(v, 'f', 1, 64)
	default:
		s = strconv.FormatFloat(v, 'f', 2, 64)
	}
	return s + k.p.Unit
}

// set turns the knob to v, and tells the application; done says the
// knob was let go.
func (k *knob) set(v float64, done bool, u *gunim.UI) {
	v = min(max(v, k.p.Lo), k.p.Hi)
	if v == k.v && !done {
		return
	}
	k.v = v
	u.Send(k, ParamSet{Layer: k.layer, Name: k.p.Name, Value: v, Done: done})
	u.Invalidate()
}

func (k *knob) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Clicks == 2 {
			k.set(k.p.Def, true, u)
			return true
		}
		k.held, k.from, k.start = true, e.Pos, k.at(k.v)
		u.Invalidate()
		return true
	case input.PointerMove:
		if !k.held {
			return false
		}
		px := float32(160)
		if e.Mods&input.ModShift != 0 {
			px = 800
		}
		k.set(k.value(k.start+float64((k.from.Y-e.Pos.Y)/px)), false, u)
		return true
	case input.PointerUp:
		if k.held {
			k.held = false
			k.set(k.v, true, u)
			return true
		}
	case input.Scroll:
		// A stepped knob steps to its next value each notch.
		if k.p.Step > 0 {
			if e.Delta.Y != 0 {
				dir := 1
				if e.Delta.Y > 0 {
					dir = -1
				}
				k.set(k.param().Next(k.v, dir), false, u)
			}
			return true
		}
		k.set(k.value(k.at(k.v)-float64(e.Delta.Y)/600), false, u)
		return true
	}
	return false
}

// DragsTouch says a finger drags the knob rather than scrolling.
func (k *knob) DragsTouch() bool { return true }

// grid lays out the first n of its children in rows, as many to a row
// as fit, each a cell big, and hides the rest.
type grid struct {
	kids []gunim.Node
	n    int
	cell geom.Size
}

func (g *grid) Children() []gunim.Node { return g.kids }

func (g *grid) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	cols := max(int(c.Max.W/g.cell.W), 1)
	n := min(g.n, kids.Len())
	for i := range kids.Len() {
		k := kids.At(i)
		if i < n {
			k.Layout(gunim.Tight(g.cell))
			k.Place(geom.Pt(float32(i%cols)*g.cell.W, float32(i/cols)*g.cell.H))
			continue
		}
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Pt(-1e5, -1e5))
	}
	rows := (n + cols - 1) / cols
	return c.Constrain(geom.Sz(c.Max.W, float32(rows)*g.cell.H))
}

func (g *grid) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for i := range min(g.n, kids.Len()) {
		kids.At(i).Paint(p)
	}
}

// switcher shows one of its children, which, and lays the rest out at
// no size, out of the way, where they take no input; which -1 shows
// none.
type switcher struct {
	kids  []gunim.Node
	which int
}

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
	if s.which >= 0 && s.which < kids.Len() {
		kids.At(s.which).Paint(p)
	}
}

// wave is a readout of a take: its lows and highs column by column,
// which shows its shape, and a line where it plays. A click on it
// plays it.
type wave struct {
	data     []float32
	playhead float32
	h        float32
	click    gunim.Intent
}

func (w *wave) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.PointerDown); ok && w.click != nil {
		u.Send(w, w.click)
		return true
	}
	return false
}

func (w *wave) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Max.W, w.h))
}

func (w *wave) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	mid := box.H / 2
	h := box.H/2 - 6
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 8})
	defer end()
	p.RRect(geom.Rc(4, mid, box.W-8, 1), 0, paint.Solid(withAlpha(scopeColor, 0.15)))
	if len(w.data) < 4 {
		run := audioui.Shaped("Not made yet", 11, false, false)
		run.Paint(p, geom.Pt((box.W-run.Advance)/2, mid-run.Height()/2), withAlpha(widget.Ink.Get(f.Theme), 0.4))
		return
	}
	n := len(w.data) / 2
	cw := (box.W - 8) / float32(n)
	for i := range n {
		x := 4 + cw*float32(i)
		lo, hi := w.data[2*i], w.data[2*i+1]
		c := withAlpha(scopeColor, 0.8)
		if w.playhead >= 0 && float32(i)/float32(n) <= w.playhead {
			c = lighten(scopeColor, 0.6)
		}
		p.RRect(geom.Rc(x, mid-hi*h, max(cw, 1), max((hi-lo)*h, 1)), 0, paint.Solid(c))
	}
	if w.playhead >= 0 {
		x := 4 + (box.W-8)*w.playhead
		p.RRect(geom.Rc(x-1, 3, 2, box.H-6), 1, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0xd0}))
	}
}

// spectrum is a readout of a take's level by pitch, from 100 Hz to 16
// kHz, with the band a phone's speaker carries, 1 to 4 kHz, marked,
// and the lows under 300 Hz it should leave out.
type spectrum struct {
	data []float32
}

func (s *spectrum) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Max.W, 120))
}

func (s *spectrum) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	ink := widget.Ink.Get(f.Theme)
	lo, hi := math.Log(100.0), math.Log(16000.0)
	x := func(hz float64) float32 { return 4 + (box.W-8)*float32((math.Log(hz)-lo)/(hi-lo)) }
	top, floor := float32(14), float32(-60)
	y := func(db float32) float32 {
		return top + (box.H-top-16)*(1-(max(db, floor)-floor)/-floor)
	}
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 8})
	defer end()
	p.RRect(geom.Rc(x(1000), 0, x(4000)-x(1000), box.H), 0, paint.Solid(withAlpha(bandColor, 0.10)))
	p.RRect(geom.Rc(0, 0, x(300), box.H), 0, paint.Solid(withAlpha(lowColor, 0.08)))
	for _, hz := range []float64{300, 1000, 4000} {
		p.RRect(geom.Rc(x(hz), 0, 1, box.H), 0, paint.Solid(withAlpha(ink, 0.15)))
	}
	for _, m := range []struct {
		hz    float64
		label string
	}{{100, "100"}, {300, "300"}, {1000, "1k"}, {4000, "4k"}, {10000, "10k"}} {
		run := audioui.Shaped(m.label, 9.5, false, false)
		run.Paint(p, geom.Pt(x(m.hz)+3, box.H-run.Height()-2), withAlpha(ink, 0.45))
	}
	cap := audioui.Shaped("1–4 kHz: where a phone's speaker carries it", 9.5, false, false)
	cap.Paint(p, geom.Pt(x(1000)+4, 2), withAlpha(bandColor, 0.7))
	if len(s.data) < 2 {
		return
	}
	pts := make([]geom.Point, len(s.data))
	for i, v := range s.data {
		hz := math.Exp(lo + (hi-lo)*(float64(i)+0.5)/float64(len(s.data)))
		pts[i] = geom.Pt(x(hz), y(v))
	}
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 4, withAlpha(scopeColor, 0.18))
	}
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 1.6, scopeColor)
	}
}
