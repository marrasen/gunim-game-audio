package plugmod

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// View implements [plug.Viewer]: a scope of the wave and its rate in a
// chip; the scope over the knobs in a panel; and in full, a taller
// scope, with the trace of what the LFO put out of late.
func (p *plugin) View(s plug.Size, h plug.Host) gunim.Node {
	switch s {
	case plug.Chip:
		return newChip(h)
	case plug.Panel:
		return newPanel(h, false)
	default:
		return newPanel(h, true)
	}
}

// settingsOf returns the settings as h holds them.
func settingsOf(h plug.Host) settings {
	var s settings
	for _, p := range Descriptor.Params {
		s.take(p.ID, h.Param(p.ID))
	}
	return s
}

// rateText writes how fast the LFO runs: its note value where synced,
// its rate where free.
func rateText(h plug.Host) string {
	if h.Param(SyncID) >= 0.5 {
		p, _ := Descriptor.Param(NoteID)
		return p.Text(h.Param(NoteID))
	}
	p, _ := Descriptor.Param(RateID)
	return p.Text(h.Param(RateID))
}

// traceLen is how many of the window's frames the trace keeps.
const traceLen = 180

// scope draws one cycle of the wave, as the settings make it, with a dot
// where the LFO is in it, and, where it keeps a trace, what the main
// output put out over the last frames of the window, the newest at the
// right.
type scope struct {
	h plug.Host
	// value and cycle are what the LFO playing reported at the last
	// step.
	value, cycle float64
	// trace holds the values reported, the newest at next-1, and
	// traced says it keeps them.
	trace  []float32
	next   int
	traced bool
}

func newScope(h plug.Host, traced bool) *scope {
	s := &scope{h: h, traced: traced, value: h.Param(ValueID), cycle: h.Param(CycleID)}
	if traced {
		s.trace = make([]float32, traceLen)
		for i := range s.trace {
			s.trace[i] = float32(s.value)
		}
	}
	return s
}

// step takes what the LFO reports, a frame of the window, and says
// whether the scope moves: the dot, or the trace while it holds more
// than one value.
func (s *scope) step() bool {
	v, c := s.h.Param(ValueID), s.h.Param(CycleID)
	moved := v != s.value || c != s.cycle
	s.value, s.cycle = v, c
	if !s.traced {
		return moved
	}
	s.trace[s.next] = float32(v)
	s.next = (s.next + 1) % len(s.trace)
	for _, t := range s.trace {
		if t != s.trace[0] {
			return true
		}
	}
	return moved
}

// paint draws the scope in r.
func (s *scope) paint(p *paint.Painter, f gunim.Frame, r geom.Rect) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	defer p.Layer(paint.LayerOpts{Bounds: r, Clip: true, Opacity: 1})()
	p.RRect(r, 6, paint.Solid(audioui.Faded(audioui.Ground.Get(f.Theme), 0.6)))
	pad := min(r.Size().H*0.12, 8)
	top, bottom := r.Min.Y+pad, r.Max.Y-pad
	yOf := func(v float64) float32 { return bottom - (bottom-top)*float32((v+1)/2) }
	p.RRect(geom.Rc(r.Min.X, yOf(0), r.Size().W, 1), 0, paint.Solid(audioui.Faded(ink, 0.12)))
	set := settingsOf(s.h)
	cyc := r
	if s.traced {
		cyc.Max.X = r.Min.X + r.Size().W*0.45
	}
	// One cycle, from its start: the Phase setting is in the dot's
	// place already. A random shape's cycles differ, so it draws the
	// value it holds.
	w := cyc.Size().W
	if set.shape == SampleHold || set.shape == SmoothRandom {
		p.RRect(geom.Rc(cyc.Min.X, yOf(s.value)-0.75, w, 1.5), 0, paint.Solid(audioui.Faded(ink, 0.35)))
	} else {
		steps := max(int(w/2), 8)
		set.phase = 0
		var was geom.Point
		for i := 0; i <= steps; i++ {
			x := float64(i) / float64(steps)
			pt := geom.Pt(cyc.Min.X+w*float32(x), yOf(set.wave(x*0.9999)))
			if i > 0 {
				audioui.Segment(p, was, pt, 1.5, audioui.Faded(ink, 0.7))
			}
			was = pt
		}
	}
	dot := geom.Pt(cyc.Min.X+w*float32(s.cycle), yOf(s.value))
	p.RRect(geom.Rc(dot.X-3, dot.Y-3, 6, 6), 3, paint.Solid(accent))
	if !s.traced {
		return
	}
	// The trace, from the oldest at the left of its part to the
	// newest, at the right edge.
	tr := geom.Rect{Min: geom.Pt(cyc.Max.X+pad, r.Min.Y), Max: r.Max}
	p.RRect(geom.Rc(tr.Min.X-pad/2, top, 1, bottom-top), 0, paint.Solid(audioui.Faded(ink, 0.12)))
	n := len(s.trace)
	for i := 1; i < n; i++ {
		a, b := s.trace[(s.next+i-1)%n], s.trace[(s.next+i)%n]
		x0 := tr.Min.X + tr.Size().W*float32(i-1)/float32(n-1)
		x1 := tr.Min.X + tr.Size().W*float32(i)/float32(n-1)
		audioui.Segment(p, geom.Pt(x0, yOf(float64(a))), geom.Pt(x1, yOf(float64(b))), 1.5, audioui.Faded(accent, 0.85))
	}
}

// chip shows the scope, in a plugin's chip, and how fast the LFO runs
// beside it.
type chip struct {
	h     plug.Host
	sc    *scope
	label *plugui.ChipText
}

func newChip(h plug.Host) *chip {
	return &chip{h: h, sc: newScope(h, false), label: plugui.NewChipText(rateText(h))}
}

// chipScope is how wide the chip's scope is.
const chipScope = 44

// Children implements [gunim.Composite].
func (c *chip) Children() []gunim.Node { return []gunim.Node{c.label} }

// Layout implements [gunim.Node]: the scope at the left, the rate after
// it.
func (c *chip) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := cs.Max
	kids.At(0).Layout(gunim.Constraints{Max: geom.Sz(max(size.W-chipScope-6, 0), size.H)})
	kids.At(0).Place(geom.Pt(chipScope+6, 0))
	return size
}

// Paint implements [gunim.Node].
func (c *chip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	c.sc.paint(p, f, geom.Rc(0, 2, min(chipScope, box.W), max(box.H-4, 0)))
	kids.At(0).Paint(p)
}

// Step implements [gunim.Animator]: the dot moves as the LFO plays.
func (c *chip) Step(time.Duration) bool { return c.sc.step() }

// Access implements [gunim.Accessible]: the chip reads as the LFO's
// shape and rate.
func (c *chip) Access() access.Info {
	p, _ := Descriptor.Param(ShapeID)
	return access.Info{Role: access.RoleLabel, Name: "LFO, " + p.Text(c.h.Param(ShapeID)) + ", " + rateText(c.h)}
}

// Follow implements [plug.Follower]: the rate shows as it changes.
func (c *chip) Follow(id uint32, _ float64, u *gunim.UI) {
	if id == SyncID || id == RateID || id == NoteID {
		c.label.Text = rateText(c.h)
	}
	u.Invalidate()
}

// panel is the scope over the knobs, in their sections.
type panel struct {
	h        plug.Host
	sc       *scope
	full     bool
	sections []*plugui.Section
	// secTop is where the sections' first row stands, as the last
	// layout placed them.
	secTop float32
}

func newPanel(h plug.Host, full bool) *panel {
	v := &panel{h: h, sc: newScope(h, full), full: full}
	v.sections = []*plugui.Section{
		plugui.NewSection(h, &Descriptor, "WAVE", 0, ShapeID, PhaseID, SeedID),
		plugui.NewSection(h, &Descriptor, "RATE", 0, SyncID, RateID, NoteID, StartID),
		plugui.NewSection(h, &Descriptor, "OUTPUT", 0, DepthID, OffsetID, UnipolarID),
	}
	v.dim()
	return v
}

// dim greys the knobs the settings leave idle: the rate where synced,
// the note value where free, and the seed for a shape of no chance.
func (v *panel) dim() {
	sync := v.h.Param(SyncID) >= 0.5
	shape := settingsOf(v.h).shape
	for _, s := range v.sections {
		for _, k := range s.Knobs {
			switch k.Param().ID {
			case RateID:
				k.Disabled = sync
			case NoteID:
				k.Disabled = !sync
			case SeedID:
				k.Disabled = shape != SampleHold && shape != SmoothRandom
			}
		}
	}
}

// panelInset is how far in from its edges the panel draws.
const panelInset = 16

// Children implements [gunim.Composite].
func (v *panel) Children() []gunim.Node {
	kids := make([]gunim.Node, len(v.sections))
	for i, s := range v.sections {
		kids[i] = s
	}
	return kids
}

// Layout implements [gunim.Node]: the sections along the foot, in rows
// as many as fit.
func (v *panel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	v.secTop = plugui.PlaceSections(c.Max, panelInset, v.sections, func(i int, r geom.Rect) {
		kids.At(i).Layout(gunim.Tight(r.Size()))
		kids.At(i).Place(r.Min)
	})
	return c.Max
}

// Paint implements [gunim.Node]: the scope over the sections, as tall
// as the room above them gives it.
func (v *panel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	h := v.secTop - 2*panelInset
	if !v.full {
		h = min(h, 120)
	}
	if h >= 24 {
		w := box.W - 2*panelInset
		if !v.full {
			w = min(w, 3*h)
		}
		v.sc.paint(pt, f, geom.Rc((box.W-w)/2, panelInset, w, h))
	}
	for i := range kids.Len() {
		kids.At(i).Paint(pt)
	}
}

// Step implements [gunim.Animator]: the scope moves as the LFO plays.
func (v *panel) Step(time.Duration) bool { return v.sc.step() }

// Follow implements [plug.Follower]: the knobs glide to their
// parameters set elsewhere, unless held, and the scope redraws.
func (v *panel) Follow(id uint32, val float64, u *gunim.UI) {
	for _, s := range v.sections {
		s.Follow(id, val, u)
	}
	v.dim()
	u.Invalidate()
}
