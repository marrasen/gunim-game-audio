package plugsynth

import (
	"image/color"
	"math"
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// The patch editor's readouts: small scopes that draw an oscillator's
// cycle, the filter's response and an envelope's shape, and a table of
// steps to draw a Game Boy wave in.

// scopeInk is the colour of the readouts' beams, and envInk the filter
// envelope's.
var (
	scopeInk = color.NRGBA{0x5f, 0xf2, 0xd6, 0xff}
	envInk   = color.NRGBA{0xff, 0xcf, 0x5c, 0xff}
)

// screen draws a scope's dark glass in r, with a faint graticule.
func screen(p *paint.Painter, r geom.Rect) {
	p.DrawRRect(paint.RRectOp{Rect: r, Radius: 6, Fill: paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y),
		Start: color.NRGBA{0x0b, 0x10, 0x14, 0xff}, End: color.NRGBA{0x07, 0x0a, 0x0d, 0xff}}},
		Inset: [2]paint.Shadow{{Offset: geom.Pt(0, 2), Blur: 6, Color: color.NRGBA{A: 0xb0}}}})
	for i := 1; i < 4; i++ {
		y := r.Min.Y + r.Size().H*float32(i)/4
		p.RRect(geom.Rc(r.Min.X+4, y, r.Size().W-8, 1), 0, paint.Solid(color.NRGBA{0x4f, 0xd6, 0xc0, 0x10}))
	}
}

// trace draws a line through pts, with a glow, as a scope's beam.
func trace(p *paint.Painter, pts []geom.Point, c color.NRGBA) {
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 4, audioui.Faded(c, 0.18))
	}
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 1.6, c)
	}
}

// fill makes a readout as big as it is given.
func fill(c gunim.Constraints) geom.Size { return c.Max }

// waveShape draws a cycle of oscillator i; a click turns it to the
// next wave.
type waveShape struct {
	e *patchEditor
	i int
}

// Layout implements [gunim.Node].
func (w *waveShape) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return fill(c)
}

// Paint implements [gunim.Node].
func (w *waveShape) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if w.i >= len(w.e.pt.Osc) {
		return
	}
	data := synth.OscCycle(w.e.pt.Osc[w.i], 96)
	pts := make([]geom.Point, len(data))
	for k, v := range data {
		pts[k] = geom.Pt(r.Min.X+4+(r.Size().W-8)*float32(k)/float32(max(len(data)-1, 1)),
			r.Min.Y+r.Size().H/2-float32(v)*(r.Size().H/2-5))
	}
	trace(p, pts, scopeInk)
}

// Handle implements [gunim.Handler]: a click turns to the next wave.
func (w *waveShape) Handle(e input.Event, u *gunim.UI) bool {
	d, ok := e.(input.PointerDown)
	if !ok || d.Button != input.ButtonPrimary || w.i >= len(w.e.pt.Osc) {
		return false
	}
	i := w.i
	w.e.edit("Turn to the next wave", func(pt *synth.Patch) {
		k := slices.Index(waveIDs, or2(pt.Osc[i].Wave, "saw"))
		pt.Osc[i].Wave = waveIDs[(k+1)%len(waveIDs)]
	}, u)
	return true
}

// response draws how loud the filter passes each frequency, from 20 Hz
// to 20 kHz.
type response struct{ e *patchEditor }

// Layout implements [gunim.Node].
func (fr *response) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return fill(c)
}

// Paint implements [gunim.Node].
func (fr *response) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	f := fr.e.pt.Filter
	fc := or(f.Cutoff, 20000)
	res := f.Res
	k := 2 - 1.96*min(max(res, 0), 1)
	n := max(int(r.Size().W)/3, 2)
	pts := make([]geom.Point, n)
	for i := range n {
		hz := 20 * math.Pow(1000, float64(i)/float64(n-1))
		// The state-variable filter's response, at hz.
		w := hz / fc
		den := complex(1-w*w, k*w)
		var h complex128
		switch f.Type {
		case "sidnotch":
			h = complex(1-w*w, 0) / den
		case "hp", "sidhp":
			h = complex(-w*w, 0) / den
		case "bp", "sidbp":
			h = complex(0, k*w) / den
		case "lp24":
			k2 := 2 - 1.96*min(max(res*0.5, 0), 1)
			h = 1 / den / complex(1-w*w, k2*w)
		case "lp", "sidlp":
			h = 1 / den
		default:
			h = 1
		}
		db := 20 * math.Log10(max(math.Hypot(real(h), imag(h)), 1e-6))
		y := r.Min.Y + r.Size().H*0.35 - float32(db)*r.Size().H/80
		pts[i] = geom.Pt(r.Min.X+r.Size().W*float32(i)/float32(n-1), min(max(y, r.Min.Y+3), r.Max.Y-3))
	}
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 6})
	trace(p, pts, scopeInk)
	end()
}

// envShape draws an envelope: its attack, its decay to its sustain,
// held a while, and its release.
type envShape struct {
	e  *patchEditor
	at func(*synth.Patch) *synth.Env
}

// Layout implements [gunim.Node].
func (es *envShape) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return geom.Sz(c.Max.W, min(c.Max.H, 56))
}

// Paint implements [gunim.Node].
func (es *envShape) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	env := *es.at(es.e.pt)
	s := min(max(env.Sustain, 0), 1)
	// Each stage's width grows with its length's root, so a long
	// release leaves a short attack room.
	w := func(x float64) float64 { return math.Sqrt(max(x, 0.0005)) }
	hold := 0.35
	total := w(env.Attack) + w(env.Decay) + hold + w(env.Release)
	in := r.Size().W - 12
	x0 := r.Min.X + 6
	y := func(v float64) float32 { return r.Max.Y - 6 - float32(v)*(r.Size().H-14) }
	xa := x0 + float32(w(env.Attack)/total)*in
	xd := xa + float32(w(env.Decay)/total)*in
	xs := xd + float32(hold/total)*in
	xr := xs + float32(w(env.Release)/total)*in
	pts := []geom.Point{geom.Pt(x0, y(0)), geom.Pt(xa, y(1))}
	for i := 1; i <= 12; i++ {
		t := float64(i) / 12
		pts = append(pts, geom.Pt(xa+(xd-xa)*float32(t), y(s+(1-s)*math.Exp(-5*t))))
	}
	pts = append(pts, geom.Pt(xs, y(s)))
	for i := 1; i <= 12; i++ {
		t := float64(i) / 12
		pts = append(pts, geom.Pt(xs+(xr-xs)*float32(t), y(s*math.Exp(-5*t))))
	}
	c := scopeInk
	if es.at(es.e.pt) == &es.e.pt.FilterEnv {
		c = envInk
	}
	trace(p, pts, c)
}

// tableEdit draws a Game Boy wave of oscillator i, 32 steps of 0 to 15:
// a drag draws it, as one edit. A second button ends the drag where it
// got to.
type tableEdit struct {
	e    *patchEditor
	i    int
	held bool
	size geom.Size
}

// steps returns the oscillator's table, or the Game Boy's own where it
// has none.
func (te *tableEdit) steps() []int {
	if te.i < len(te.e.pt.Osc) && len(te.e.pt.Osc[te.i].Table) == 32 {
		return te.e.pt.Osc[te.i].Table
	}
	return synth.GBWave
}

// Layout implements [gunim.Node].
func (te *tableEdit) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	te.size = c.Max
	return c.Max
}

// Paint implements [gunim.Node].
func (te *tableEdit) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	steps := te.steps()
	w := (r.Size().W - 8) / float32(len(steps))
	h := r.Size().H - 8
	for i, v := range steps {
		bh := max(float32(v)/15*h, 1)
		p.RRect(geom.Rc(r.Min.X+4+float32(i)*w, r.Max.Y-4-bh, max(w-1, 1), bh), 1, paint.Solid(audioui.Faded(scopeInk, 0.85)))
	}
}

// draw sets the step under at to its height.
func (te *tableEdit) draw(at geom.Point, u *gunim.UI) {
	if te.size.W < 9 || te.i >= len(te.e.pt.Osc) {
		return
	}
	steps := slices.Clone(te.steps())
	i := min(max(int((at.X-4)/((te.size.W-8)/float32(len(steps)))), 0), len(steps)-1)
	v := min(max(int(math.Round(float64((te.size.H-4-at.Y)/(te.size.H-8)*15))), 0), 15)
	if steps[i] == v {
		return
	}
	steps[i] = v
	k := te.i
	te.e.change(func(pt *synth.Patch) { pt.Osc[k].Table = steps }, u)
	u.Invalidate()
}

// Handle implements [gunim.Handler].
func (te *tableEdit) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if te.held {
			// A second button ends the drag where it got to.
			te.held = false
			te.e.end(u)
			return true
		}
		if e.Button != input.ButtonPrimary {
			return false
		}
		te.held = true
		te.e.begin("Draw the wave", u)
		te.draw(e.Pos, u)
		return true
	case input.PointerMove:
		if te.held {
			te.draw(e.Pos, u)
			return true
		}
	case input.PointerUp:
		if te.held {
			te.held = false
			te.e.end(u)
		}
		return true
	}
	return false
}

// DragsTouch says a finger draws rather than scrolling.
func (te *tableEdit) DragsTouch() bool { return true }
