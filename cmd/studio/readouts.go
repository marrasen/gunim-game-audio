package main

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"

	"github.com/marrasen/gunim-music/synth"
)

// screen is the dark glass a readout draws on, as a synth's display.
func screen(p *paint.Painter, r geom.Rect) {
	p.DrawRRect(paint.RRectOp{Rect: r, Radius: 8, Fill: paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y),
		Start: color.NRGBA{0x0b, 0x10, 0x14, 0xff}, End: color.NRGBA{0x07, 0x0a, 0x0d, 0xff}}},
		Inset: [2]paint.Shadow{{Offset: geom.Pt(0, 2), Blur: 6, Color: color.NRGBA{A: 0xb0}}}})
	// A faint grid, as a scope's graticule.
	for i := 1; i < 4; i++ {
		y := r.Min.Y + r.Size().H*float32(i)/4
		p.RRect(geom.Rc(r.Min.X+4, y, r.Size().W-8, 1), 0, paint.Solid(color.NRGBA{0x4f, 0xd6, 0xc0, 0x10}))
	}
	for i := 1; i < 8; i++ {
		x := r.Min.X + r.Size().W*float32(i)/8
		p.RRect(geom.Rc(x, r.Min.Y+4, 1, r.Size().H-8), 0, paint.Solid(color.NRGBA{0x4f, 0xd6, 0xc0, 0x0c}))
	}
}

// trace draws a line through points, with a glow, as a scope's beam.
func trace(p *paint.Painter, pts []geom.Point, c color.NRGBA) {
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 4, withAlpha(c, 0.18))
	}
	for i := 1; i < len(pts); i++ {
		audioui.Segment(p, pts[i-1], pts[i], 1.6, c)
	}
}

// scopeColor is the colour of the readouts' beams.
var scopeColor = color.NRGBA{0x5f, 0xf2, 0xd6, 0xff}

// wave is a readout of a sound: a cycle as a line, or a whole sound as
// its lows and highs column by column, which shows its envelope.
type wave struct {
	// data is the sound, and spans says it is lows and highs in pairs.
	data  []float32
	spans bool
	color color.NRGBA
	// label names it in the corner.
	label string
	// fit scales the sound to fill the readout's height.
	fit bool
	// click, where set, is the intent a click on the readout sends.
	click func() gunim.Intent
}

func (w *wave) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.PointerDown); ok && w.click != nil {
		u.Send(w, w.click())
		return true
	}
	return false
}

func (w *wave) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

func (w *wave) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	c := w.color
	if c == (color.NRGBA{}) {
		c = scopeColor
	}
	mid := r.Min.Y + r.Size().H/2
	h := r.Size().H/2 - 6
	scale := float32(1)
	if w.fit {
		var top float32
		for _, v := range w.data {
			top = max(top, abs(v))
		}
		if top > 1e-4 {
			scale = 1 / top
		}
	}
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 8})
	if w.spans && len(w.data) >= 4 {
		n := len(w.data) / 2
		for i := range n {
			x := r.Min.X + 4 + (r.Size().W-8)*float32(i)/float32(n)
			lo, hi := w.data[2*i]*scale, w.data[2*i+1]*scale
			p.RRect(geom.Rc(x, mid-hi*h, max((r.Size().W-8)/float32(n), 1), max((hi-lo)*h, 1)), 0, paint.Solid(withAlpha(c, 0.75)))
		}
	} else if len(w.data) >= 2 {
		pts := make([]geom.Point, len(w.data))
		for i, v := range w.data {
			pts[i] = geom.Pt(r.Min.X+4+(r.Size().W-8)*float32(i)/float32(len(w.data)-1), mid-v*scale*h)
		}
		trace(p, pts, c)
	}
	end()
	if w.label != "" {
		run := audioui.Shaped(w.label, 10, false, false)
		run.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(c, 0.6))
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// oscShape returns a cycle of o's wave, as its readout draws it.
func oscShape(o synth.Osc, n int) []float32 {
	out := make([]float32, n)
	width := o.Width
	if width == 0 {
		width = 0.5
	}
	ratio, index := o.Ratio, o.Index
	if ratio == 0 {
		ratio = 1
	}
	for i := range out {
		t := float64(i) / float64(n-1)
		var v float64
		switch o.Wave {
		case "square", "pulse":
			v = -1
			if t < width {
				v = 1
			}
		case "tri", "triangle":
			v = 1 - 4*math.Abs(t-0.5)
		case "sine":
			v = math.Sin(2 * math.Pi * t)
		case "fm":
			v = math.Sin(2*math.Pi*t + index*math.Sin(2*math.Pi*ratio*t))
		default:
			v = 2*t - 1
		}
		out[i] = float32(v * 0.9)
	}
	return out
}

// envelope is a readout of an envelope's shape: its attack, decay,
// sustain held a while, and release.
type envelope struct {
	base  *string
	rel   string
	song  *synth.Song
	color color.NRGBA
	label string
}

func (e *envelope) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (e *envelope) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if e.song == nil {
		return
	}
	path := *e.base + e.rel
	a := num(e.song, path+"/Attack")
	d := num(e.song, path+"/Decay")
	s := min(max(num(e.song, path+"/Sustain"), 0), 1)
	rl := num(e.song, path+"/Release")
	// Each stage's width grows with its length, as its root does, so a
	// long release does not crowd out a short attack.
	w := func(x float64) float64 { return math.Sqrt(max(x, 0.0005)) }
	hold := 0.35
	total := w(a) + w(d) + hold + w(rl)
	in := r.Size().W - 12
	x0 := r.Min.X + 6
	y := func(v float64) float32 { return r.Max.Y - 6 - float32(v)*(r.Size().H-14) }
	xa := x0 + float32(w(a)/total)*in
	xd := xa + float32(w(d)/total)*in
	xs := xd + float32(hold/total)*in
	xr := xs + float32(w(rl)/total)*in
	var pts []geom.Point
	pts = append(pts, geom.Pt(x0, y(0)), geom.Pt(xa, y(1)))
	for i := 1; i <= 12; i++ {
		t := float64(i) / 12
		pts = append(pts, geom.Pt(xa+(xd-xa)*float32(t), y(s+(1-s)*math.Exp(-5*t))))
	}
	pts = append(pts, geom.Pt(xs, y(s)))
	for i := 1; i <= 12; i++ {
		t := float64(i) / 12
		pts = append(pts, geom.Pt(xs+(xr-xs)*float32(t), y(s*math.Exp(-5*t))))
	}
	c := e.color
	if c == (color.NRGBA{}) {
		c = scopeColor
	}
	trace(p, pts, c)
	if e.label != "" {
		run := audioui.Shaped(e.label, 10, false, false)
		run.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(c, 0.6))
	}
}

// response is a readout of a filter: how loud it passes each frequency,
// from 20 Hz to 20 kHz, as its type, cutoff and resonance make it.
type response struct {
	base *string
	song *synth.Song
}

func (fr *response) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (fr *response) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if fr.song == nil {
		return
	}
	base := *fr.base + "/Filter"
	typ := str(fr.song, base+"/Type")
	fc := num(fr.song, base+"/Cutoff")
	if fc <= 0 {
		fc = 20000
	}
	res := num(fr.song, base+"/Res")
	k := 2 - 1.96*min(max(res, 0), 1)
	n := int(r.Size().W) / 3
	pts := make([]geom.Point, n)
	for i := range n {
		hz := 20 * math.Pow(1000, float64(i)/float64(n-1))
		// The state-variable filter's response, at hz.
		w := hz / fc
		den := complex(1-w*w, k*w)
		var h complex128
		switch typ {
		case "hp":
			h = complex(-w*w, 0) / den
		case "bp":
			h = complex(0, k*w) / den
		case "lp24":
			k2 := 2 - 1.96*min(max(res*0.5, 0), 1)
			h = 1 / den / complex(1-w*w, k2*w)
		case "lp":
			h = 1 / den
		default:
			h = 1
		}
		db := 20 * math.Log10(max(cabs(h), 1e-6))
		y := r.Min.Y + r.Size().H*0.35 - float32(db)*r.Size().H/80
		pts[i] = geom.Pt(r.Min.X+r.Size().W*float32(i)/float32(n-1), min(max(y, r.Min.Y+3), r.Max.Y-3))
	}
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 8})
	trace(p, pts, scopeColor)
	end()
	label := typ
	if label == "" {
		label = "none"
	}
	run := audioui.Shaped(label, 10, false, false)
	run.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(scopeColor, 0.6))
}

func cabs(c complex128) float64 { return math.Hypot(real(c), imag(c)) }

// vu is a stereo level meter: each channel's RMS as a solid bar, its
// peak as a line over it, green to amber to red as it nears full
// scale.
type vu struct {
	m     Meter
	shown Meter
}

func (v *vu) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

// meterAt is where a level, 1 at full scale, stands on a meter, from 0
// at -60 dB to 1 at 0 dB.
func meterAt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	db := 20 * math.Log10(float64(x))
	return float32(min(max((db+60)/60, 0), 1))
}

func (v *vu) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, 4, paint.Solid(color.NRGBA{0x08, 0x0a, 0x0e, 0xff}))
	bw := (r.Size().W - 6) / 2
	grad := &paint.Gradient{From: geom.Pt(0, r.Max.Y), To: geom.Pt(0, r.Min.Y), Start: color.NRGBA{0x3c, 0xd6, 0x8a, 0xff},
		End: color.NRGBA{0xff, 0x5f, 0x52, 0xff}, Stops: []paint.Stop{{At: 0.72, Color: color.NRGBA{0x9c, 0xe0, 0x5a, 0xff}}, {At: 0.88, Color: color.NRGBA{0xff, 0xc8, 0x57, 0xff}}}}
	for ch := range 2 {
		x := r.Min.X + 2 + float32(ch)*(bw+2)
		h := r.Size().H - 4
		rms := meterAt(v.shown.RMS[ch]) * h
		pk := meterAt(v.shown.Peak[ch]) * h
		p.RRect(geom.Rc(x, r.Max.Y-2-h, bw, h), 2, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x08}))
		if rms > 0.5 {
			p.RRect(geom.Rc(x, r.Max.Y-2-rms, bw, rms), 2, paint.Fill{Gradient: grad})
		}
		if pk > 0.5 {
			c := color.NRGBA{0xd8, 0xf0, 0xe8, 0xd0}
			if v.shown.Peak[ch] >= 0.98 {
				c = color.NRGBA{0xff, 0x5f, 0x52, 0xff}
			}
			p.RRect(geom.Rc(x, r.Max.Y-2-pk, bw, 2), 1, paint.Solid(c))
		}
	}
	// Ticks every 12 dB.
	for db := -48; db <= 0; db += 12 {
		y := r.Max.Y - 2 - float32(db+60)/60*(r.Size().H-4)
		p.RRect(geom.Rc(r.Min.X, y, 2, 1), 0, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x40}))
	}
}

func (v *vu) Step(dt time.Duration) bool {
	t := float32(dt.Seconds())
	moving := false
	for ch := range 2 {
		for _, pair := range [2][2]*float32{{&v.shown.Peak[ch], &v.m.Peak[ch]}, {&v.shown.RMS[ch], &v.m.RMS[ch]}} {
			s, m := pair[0], pair[1]
			if *m > *s {
				*s += (*m - *s) * min(1, t*30)
			} else {
				*s += (*m - *s) * min(1, t*6)
			}
			moving = moving || *s > 1e-4 || *m > 0
		}
	}
	return moving
}
