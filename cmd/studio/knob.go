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

	"github.com/marrasen/gunim-music/synth"
)

// knob sets a number of the song, at a path, as a synth's knob does: a
// drag up turns it up, finer with Shift; the wheel steps it; a double
// click sets it back to where it starts. Its value shows under its name.
type knob struct {
	label string
	// base and rel make its path: base the editor's, as Patches/lead,
	// and rel its own under it, as /Filter/Cutoff.
	base *string
	rel  string
	// lo and hi bound it, and def is where a double click sets it; log
	// turns it by ratios rather than steps, as a frequency is; step
	// rounds it, as a count is; bipolar fills its arc from the middle.
	lo, hi, def float64
	log         bool
	step        float64
	bipolar     bool
	// unit follows its value, as Hz or s; unset is the value the engine
	// takes where the song leaves it 0, and the knob shows then.
	unit  string
	unset float64
	// compact makes it smaller, for a mixer's narrow strips.
	compact bool
	color   color.NRGBA
	v       float64
	held    bool
	from    geom.Point
	start   float64
	size    geom.Size
}

const knobW, knobH = 50, 66

// newKnob returns a knob named label for the number at base+rel, from
// lo to hi.
func newKnob(label string, base *string, rel string, lo, hi, def float64) *knob {
	return &knob{label: label, base: base, rel: rel, lo: lo, hi: hi, def: def, color: color.NRGBA{0x5e, 0x9c, 0xff, 0xff}}
}

func (k *knob) logScale() *knob         { k.log = true; return k }
func (k *knob) steps(s float64) *knob   { k.step = s; return k }
func (k *knob) center() *knob           { k.bipolar = true; return k }
func (k *knob) units(u string) *knob    { k.unit = u; return k }
func (k *knob) unsetIs(v float64) *knob { k.unset = v; return k }
func (k *knob) small() *knob            { k.compact = true; return k }

func (k *knob) path() string { return *k.base + k.rel }

// at returns where v lies on the knob's turn, from 0 to 1.
func (k *knob) at(v float64) float64 {
	if k.log && k.lo > 0 {
		return math.Log(max(v, k.lo)/k.lo) / math.Log(k.hi/k.lo)
	}
	return (v - k.lo) / (k.hi - k.lo)
}

// value returns the value at t on the knob's turn.
func (k *knob) value(t float64) float64 {
	t = min(max(t, 0), 1)
	var v float64
	if k.log && k.lo > 0 {
		v = k.lo * math.Pow(k.hi/k.lo, t)
	} else {
		v = k.lo + (k.hi-k.lo)*t
	}
	if k.step > 0 {
		v = math.Round(v/k.step) * k.step
	}
	return v
}

// show takes the value from the song, unless the knob is held.
func (k *knob) show(song *synth.Song) {
	if !k.held {
		k.v = num(song, k.path())
		if k.v == 0 && k.unset != 0 {
			k.v = k.unset
		}
	}
}

func (k *knob) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	k.size = c.Constrain(geom.Sz(knobW, knobH))
	if k.compact {
		k.size = c.Constrain(geom.Sz(44, 56))
	}
	return k.size
}

func (k *knob) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := audioui.Ink.Get(f.Theme)
	r, text := float32(15), float32(10.5)
	if k.compact {
		r, text = 12, 9.5
	}
	c := geom.Pt(box.W/2, 4+r)
	// The turn runs from 135° to 405°, three quarters round, clockwise.
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
	if k.bipolar {
		start = 0.5
	}
	glow := k.color
	if k.held {
		glow = lighten(k.color, 0.3)
	}
	arc(start, t, 3.5, glow)
	// The cap, and its line pointing at the value.
	p.ShadowRRect(geom.Rc(c.X-r+6, c.Y-r+6, 2*r-12, 2*r-12), r-6, paint.Fill{Gradient: &paint.Gradient{
		From: geom.Pt(c.X, c.Y-r), To: geom.Pt(c.X, c.Y+r), Start: color.NRGBA{0x44, 0x4a, 0x5c, 0xff}, End: color.NRGBA{0x22, 0x26, 0x31, 0xff}}},
		paint.Shadow{Offset: geom.Pt(0, 2), Blur: 4, Color: color.NRGBA{A: 0x90}})
	a := from + sweep*t
	audioui.Segment(p, geom.Pt(c.X+(r-12)*float32(math.Cos(a)), c.Y+(r-12)*float32(math.Sin(a))),
		geom.Pt(c.X+(r-7)*float32(math.Cos(a)), c.Y+(r-7)*float32(math.Sin(a))), 2, ink)
	name := audioui.Shaped(k.label, text, false, false)
	name.Paint(p, geom.Pt((box.W-name.Advance)/2, c.Y+r+3), withAlpha(ink, 0.6))
	val := audioui.Shaped(k.format(), text, false, true)
	col := withAlpha(ink, 0.9)
	if k.held {
		col = lighten(k.color, 0.5)
	}
	val.Paint(p, geom.Pt((box.W-val.Advance)/2, c.Y+r+3+name.Height()), col)
}

// format writes the knob's value, as short as reads well.
func (k *knob) format() string {
	v := k.v
	var s string
	switch {
	case k.step >= 1:
		s = strconv.Itoa(int(math.Round(v)))
	case math.Abs(v) >= 1000:
		s = strconv.FormatFloat(v/1000, 'f', 1, 64) + "k"
	case math.Abs(v) >= 100:
		s = strconv.FormatFloat(v, 'f', 0, 64)
	case math.Abs(v) >= 10:
		s = strconv.FormatFloat(v, 'f', 1, 64)
	default:
		s = strconv.FormatFloat(v, 'f', 2, 64)
	}
	return s + k.unit
}

// set turns the knob to v, and tells the application.
func (k *knob) set(v float64, u *gunim.UI) {
	v = min(max(v, k.lo), k.hi)
	if v == k.v {
		return
	}
	k.v = v
	u.Send(k, SetValue{Path: k.path(), Num: v})
	u.Invalidate()
}

func (k *knob) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Clicks == 2 {
			k.set(k.def, u)
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
		k.set(k.value(k.start+float64((k.from.Y-e.Pos.Y)/px)), u)
		return true
	case input.PointerUp:
		if k.held {
			k.held = false
			u.Invalidate()
			return true
		}
	case input.Scroll:
		d := -float64(e.Delta.Y) / 600
		if k.step > 0 {
			d = math.Copysign(max(math.Abs(d), k.step/(k.hi-k.lo)*1.01), d)
		}
		k.set(k.value(k.at(k.v)+d), u)
		return true
	}
	return false
}

// DragsTouch says a finger drags the knob rather than scrolling.
func (k *knob) DragsTouch() bool { return true }

// knobs lays knobs out in a row, a gap apart.
func knobs(ks ...*knob) *widget.Flex {
	nodes := make([]gunim.Node, len(ks))
	for i, k := range ks {
		nodes[i] = k
	}
	r := widget.Row(nodes...)
	return r
}

// showKnobs shows each knob's value from song.
func showKnobs(song *synth.Song, ks ...*knob) {
	for _, k := range ks {
		k.show(song)
	}
}
