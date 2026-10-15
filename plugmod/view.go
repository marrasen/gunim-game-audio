package plugmod

import (
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// View implements [plug.Viewer]: a scope of the wave and its rate in a
// chip; the scope over the controls in a panel; and in full, a taller
// scope, with the trace of what the LFO put out of late. In a panel or
// in full, the scope edits the Steps shape's steps and the Shaper's
// curve, while it shows them.
func (p *plugin) View(s plug.Size, h plug.Host) gunim.Node {
	switch s {
	case plug.Chip:
		return newChip(h, p)
	case plug.Panel:
		return newPanel(h, p, false)
	default:
		return newPanel(h, p, true)
	}
}

// settingsOf returns the settings as h holds them, with p's steps and
// curve.
func settingsOf(h plug.Host, p *plugin) settings {
	s := settings{pat: p.proc.pat.Load()}
	if s.pat == nil {
		s.pat = defaultPattern
	}
	for _, prm := range Descriptor.Params {
		s.take(prm.ID, h.Param(prm.ID))
	}
	return s
}

// randomShape reports whether the shape picks random values, as the seed
// picks them.
func randomShape(shape int) bool { return shape == SampleHold || shape == SmoothRandom }

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
// right. Where it edits, it draws the Steps shape's steps as bars under
// the wave, and the Shaper's points on it.
type scope struct {
	h plug.Host
	p *plugin
	// value and cycle are what the LFO playing reported at the last
	// step.
	value, cycle float64
	// trace holds the values reported, the newest at next-1, and
	// traced says it keeps them.
	trace  []float32
	next   int
	traced bool
	// edit says the scope shows the steps and the points, and area is
	// where the cycle was drawn last, from its value of 1 at the top to
	// -1 at the foot, for the edits to read.
	edit bool
	area geom.Rect
	// hot is the point under the pointer, or -1.
	hot int
}

func newScope(h plug.Host, p *plugin, traced, edit bool) *scope {
	s := &scope{h: h, p: p, traced: traced, edit: edit, hot: -1, value: h.Param(ValueID), cycle: h.Param(CycleID)}
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

// xOf and yOf return where x through the cycle and the value v stand in
// the scope's area.
func (s *scope) xOf(x float64) float32 { return s.area.Min.X + s.area.Size().W*float32(x) }
func (s *scope) yOf(v float64) float32 { return s.area.Max.Y - s.area.Size().H*float32((v+1)/2) }

// pointOf returns the place pos stands for in the scope's area: through
// the cycle, and the value.
func (s *scope) pointOf(pos geom.Point) Point {
	sz := s.area.Size()
	if sz.W <= 0 || sz.H <= 0 {
		return Point{}
	}
	return Point{float64((pos.X - s.area.Min.X) / sz.W), 1 - 2*float64((pos.Y-s.area.Min.Y)/sz.H)}
}

// pointAt returns the curve's point nearest pos, within reach of a
// pointer, or -1.
func (s *scope) pointAt(pos geom.Point) int {
	const reach = 9
	best, at := float32(reach*reach), -1
	for i, q := range s.p.current().Curve {
		dx, dy := s.xOf(q.X)-pos.X, s.yOf(q.Y)-pos.Y
		if d := dx*dx + dy*dy; d <= best {
			best, at = d, i
		}
	}
	return at
}

// paint draws the scope in r.
func (s *scope) paint(p *paint.Painter, f gunim.Frame, r geom.Rect) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	defer p.Layer(paint.LayerOpts{Bounds: r, Clip: true, Opacity: 1})()
	p.RRect(r, 6, paint.Solid(audioui.Faded(audioui.Ground.Get(f.Theme), 0.6)))
	pad := min(r.Size().H*0.12, 8)
	cyc := r
	if s.traced {
		cyc.Max.X = r.Min.X + r.Size().W*0.45
	}
	s.area = geom.Rect{Min: geom.Pt(cyc.Min.X, r.Min.Y+pad), Max: geom.Pt(cyc.Max.X, r.Max.Y-pad)}
	top, bottom := s.area.Min.Y, s.area.Max.Y
	p.RRect(geom.Rc(r.Min.X, s.yOf(0), r.Size().W, 1), 0, paint.Solid(audioui.Faded(ink, 0.12)))
	set := settingsOf(s.h, s.p)
	w := cyc.Size().W
	if s.edit && set.shape == Steps {
		// The steps' bars, from 0 to each step's value, a gap between.
		n := set.pat.count
		bw := w / float32(n)
		for i := range n {
			y0, y1 := s.yOf(0), s.yOf(set.pat.steps[i])
			p.RRect(geom.Rc(cyc.Min.X+bw*float32(i)+1, min(y0, y1), max(bw-2, 1), max(abs32(y1-y0), 1)), 2,
				paint.Solid(audioui.Faded(accent, 0.28)))
		}
	}
	// One cycle, from its start: the Phase setting is in the dot's
	// place already. A random shape's cycles differ, so it draws the
	// value it holds.
	if randomShape(set.shape) {
		p.RRect(geom.Rc(cyc.Min.X, s.yOf(s.value)-0.75, w, 1.5), 0, paint.Solid(audioui.Faded(ink, 0.35)))
	} else {
		steps := max(int(w/2), 8)
		set.phase = 0
		var was geom.Point
		for i := 0; i <= steps; i++ {
			x := float64(i) / float64(steps)
			pt := geom.Pt(s.xOf(x), s.yOf(set.wave(x*0.9999)))
			if i > 0 {
				audioui.Segment(p, was, pt, 1.5, audioui.Faded(ink, 0.7))
			}
			was = pt
		}
	}
	if s.edit && set.shape == Shaper {
		for i, q := range set.pat.curve {
			c := geom.Pt(s.xOf(q.X), s.yOf(q.Y))
			rad := float32(3.5)
			if i == s.hot {
				rad = 5
			}
			p.RRect(geom.Rc(c.X-rad, c.Y-rad, 2*rad, 2*rad), rad, paint.Solid(audioui.Faded(accent, 0.9)))
		}
	}
	dot := geom.Pt(s.xOf(s.cycle), s.yOf(s.value))
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
		audioui.Segment(p, geom.Pt(x0, s.yOf(float64(a))), geom.Pt(x1, s.yOf(float64(b))), 1.5, audioui.Faded(accent, 0.85))
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// chip shows the scope, in a plugin's chip, and how fast the LFO runs
// beside it.
type chip struct {
	h     plug.Host
	sc    *scope
	label *plugui.ChipText
}

func newChip(h plug.Host, p *plugin) *chip {
	return &chip{h: h, sc: newScope(h, p, false, false), label: plugui.NewChipText(rateText(h))}
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

// FollowState implements [plug.StateFollower]: the steps or the curve
// changed.
func (c *chip) FollowState(u *gunim.UI) { u.Invalidate() }

// rateKnob is one knob for how fast the LFO runs: the note value's,
// where it is synced, and the rate's in hertz where not. Each is a knob
// of its own parameter, so each keeps its value, and its automation,
// across a change of Sync; only the one at work shows.
type rateKnob struct {
	rate, note *plugui.Knob
	synced     bool
}

// active returns the knob at work.
func (r *rateKnob) active() *plugui.Knob {
	if r.synced {
		return r.note
	}
	return r.rate
}

// Children implements [gunim.Composite].
func (r *rateKnob) Children() []gunim.Node { return []gunim.Node{r.rate, r.note} }

// Layout implements [gunim.Node]: the knob at work as big as a knob,
// and the other of no size.
func (r *rateKnob) Layout(_ gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	on := 0
	if r.synced {
		on = 1
	}
	for i := range kids.Len() {
		size := geom.Size{}
		if i == on {
			size = geom.Sz(plugui.KnobW, plugui.KnobH)
		}
		kids.At(i).Layout(gunim.Tight(size))
		kids.At(i).Place(geom.Pt(0, 0))
	}
	return geom.Sz(plugui.KnobW, plugui.KnobH)
}

// Paint implements [gunim.Node]: only the knob at work.
func (r *rateKnob) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	if r.synced {
		kids.At(1).Paint(p)
	} else {
		kids.At(0).Paint(p)
	}
}

// The selectors' tips, by option.
var (
	shapeTips = []string{
		"A smooth swing up and down", "Straight up and straight down", "A ramp up, then a drop",
		"A drop, then a ramp down", "High for half the cycle, low for the other",
		"Slowly up and then fast, as a swell", "Fast down and then slowly, as a pluck",
		"A new random value each cycle, held", "Random values, gliding from one to the next",
		"The steps drawn in the scope, in turn", "The curve drawn in the scope: click to add a point, drag it, right-click to take it away",
	}
	syncTips  = []string{"A rate in hertz, of its own", "A note value of the song's tempo"}
	startTips = []string{"From the song's start, so it is in the same place each play",
		"From where the play starts", "On and on, through stops and jumps"}
	polarityTips = []string{"From -1 to 1, about the parameter's value", "From 0 to 1, up from the parameter's value"}
)

// stepNames are the step counts, as the Steps selector names them.
var stepNames = func() []string {
	n := make([]string, MaxSteps)
	for i := range n {
		n[i] = strconv.Itoa(i + 1)
	}
	return n
}()

// panel is the scope over the controls, in their sections.
type panel struct {
	h        plug.Host
	p        *plugin
	sc       *scope
	full     bool
	sections []*plugui.Section
	knobs    []*plugui.Knob
	rate     *rateKnob
	// The selectors: of parameters, and of the steps' count.
	shape, sync, start, polarity, count *plugui.Selector
	// secTop is where the sections' first row stands, as the last
	// layout placed them.
	secTop float32
	// held is what the pointer drags in the scope, and point the curve's
	// point it drags; last is the step it set last.
	held  int
	point int
	last  int
}

// What the pointer drags in the scope.
const (
	holdNone = iota
	holdSteps
	holdPoint
)

func newPanel(h plug.Host, p *plugin, full bool) *panel {
	v := &panel{h: h, p: p, sc: newScope(h, p, full, true), full: full}
	knob := func(id uint32, tip string) *plugui.Knob {
		prm, _ := Descriptor.Param(id)
		k := plugui.NewKnob(h, prm)
		k.Tooltip = tip
		v.knobs = append(v.knobs, k)
		return k
	}
	pick := func(id uint32) func(int, *gunim.UI) {
		return func(i int, u *gunim.UI) {
			h.Begin(id, u)
			h.Set(id, float64(i), u)
			h.End(id, u)
			v.show(u)
		}
	}
	selector := func(id uint32, title string, names, tips []string) *plugui.Selector {
		s := plugui.NewSelector(title, names, tips)
		s.OnPick = pick(id)
		return s
	}
	v.shape = selector(ShapeID, "Shape", Shapes, shapeTips)
	v.shape.Columns = 6
	v.sync = selector(SyncID, "Sync", []string{"Hz", "Tempo"}, syncTips)
	v.start = selector(StartID, "Start", Starts, startTips)
	v.polarity = selector(UnipolarID, "Polarity", []string{"Bipolar", "Unipolar"}, polarityTips)
	v.count = plugui.NewSelector("Steps", stepNames, nil)
	v.count.Columns = 8
	v.count.OnPick = func(i int, u *gunim.UI) {
		v.edit("Steps", u, func(st State) State { return st.withCount(i + 1) })
	}
	v.rate = &rateKnob{
		rate: knob(RateID, "How fast the wave runs, in cycles a second"),
		note: knob(NoteID, "How long a cycle lasts, in note values of the song's tempo"),
	}
	section := func(name string) *plugui.Section { return plugui.NewSection(h, &Descriptor, name, 0) }
	v.sections = []*plugui.Section{
		section("WAVE").Add(v.shape, 112).
			Add(knob(PhaseID, "Where in its cycle the wave starts"), plugui.KnobW).
			Add(knob(SeedID, "Which random values the random shapes and the jitter pick"), plugui.KnobW),
		section("RATE").Add(v.sync, 48).Add(v.rate, plugui.KnobW).Add(v.start, 56),
		section("CHARACTER").
			Add(knob(JitterID, "Random wobble added to the wave"), plugui.KnobW).
			Add(knob(SmoothID, "Rounds the wave's corners off, up to an average over half a cycle"), plugui.KnobW),
		section("STEPS").Add(v.count, 144).
			Add(knob(GlideID, "How much of each step slides from the step before"), plugui.KnobW),
		section("OUTPUT").
			Add(knob(DepthID, "How far the wave swings"), plugui.KnobW).
			Add(knob(OffsetID, "Moves the wave up or down"), plugui.KnobW).
			Add(v.polarity, 56),
	}
	v.show(nil)
	return v
}

// show sets the selectors as the parameters and the state stand, and
// greys the controls the settings leave idle: the rate's knob of the
// other Sync, the seed for a shape of no chance and no jitter, and the
// steps' controls for another shape.
func (v *panel) show(u *gunim.UI) {
	set := settingsOf(v.h, v.p)
	v.shape.SetSelected(set.shape, u)
	v.sync.SetSelected(boolStep(set.sync), u)
	v.start.SetSelected(set.start, u)
	v.polarity.SetSelected(boolStep(set.unipolar), u)
	v.count.SetSelected(set.pat.count-1, u)
	v.count.Locked = set.shape != Steps
	if v.rate.synced != set.sync {
		v.rate.synced = set.sync
		u.Invalidate()
	}
	for _, k := range v.knobs {
		switch k.Param().ID {
		case SeedID:
			k.Disabled = !randomShape(set.shape) && set.jitter == 0
		case GlideID:
			k.Disabled = set.shape != Steps
		}
	}
}

// boolStep is a switch's option: 1 on, 0 off.
func boolStep(on bool) int {
	if on {
		return 1
	}
	return 0
}

// edit is one gesture named name on the state: change makes the state
// from the one before.
func (v *panel) edit(name string, u *gunim.UI, change func(State) State) {
	v.h.BeginEdit(name, u)
	v.change(change(v.p.current()), u)
	v.h.EndEdit(u)
}

// change makes st the state, in a gesture under way.
func (v *panel) change(st State, u *gunim.UI) {
	v.p.set(st)
	v.h.Changed(u)
	v.show(u)
	u.Invalidate()
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
// parameters set elsewhere, unless held, the selectors follow, and the
// scope redraws.
func (v *panel) Follow(id uint32, val float64, u *gunim.UI) {
	for _, k := range v.knobs {
		if k.Param().ID == id {
			k.Follow(val, u)
		}
	}
	v.show(u)
	u.Invalidate()
}

// FollowState implements [plug.StateFollower]: the steps or the curve
// changed, as undone.
func (v *panel) FollowState(u *gunim.UI) {
	v.show(u)
	u.Invalidate()
}

// editing returns the shape the scope edits, Steps or Shaper, and false
// for a shape it doesn't.
func (v *panel) editing() (int, bool) {
	shape := settingsOf(v.h, v.p).shape
	return shape, shape == Steps || shape == Shaper
}

// over reports whether pos is over the scope's cycle, with a little
// room round it to reach its edges.
func (v *panel) over(pos geom.Point) bool {
	a := v.sc.area
	return pos.X >= a.Min.X-4 && pos.X <= a.Max.X+4 && pos.Y >= a.Min.Y-8 && pos.Y <= a.Max.Y+8
}

// setSteps sets the steps from the one set last to the one under pos,
// to the value there, as a drag draws across them.
func (v *panel) setSteps(pos geom.Point, u *gunim.UI) {
	q := v.sc.pointOf(pos)
	st := v.p.current()
	i, _ := compile(st).stepOf(min(max(q.X, 0), 0.9999))
	// From the step after the one set last, so it keeps its value.
	from := i
	switch {
	case v.last >= 0 && v.last < i:
		from = v.last + 1
	case v.last > i:
		from = v.last - 1
	}
	for j := min(from, i); j <= max(from, i); j++ {
		st = st.withStep(j, q.Y)
	}
	v.last = i
	v.change(st, u)
}

// Handle implements [gunim.Handler]: over the scope, a press and drag
// draws the steps of the Steps shape; of the Shaper, it drags the point
// under it, or adds one there, and a right click takes the point away.
func (v *panel) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		switch v.held {
		case holdSteps:
			v.setSteps(e.Pos, u)
		case holdPoint:
			v.change(v.p.current().movingPoint(v.point, v.sc.pointOf(e.Pos)), u)
		default:
			hot := -1
			if shape, ok := v.editing(); ok && shape == Shaper {
				hot = v.sc.pointAt(e.Pos)
			}
			if hot != v.sc.hot {
				v.sc.hot = hot
				u.Invalidate()
			}
			return false
		}
	case input.PointerDown:
		shape, ok := v.editing()
		if !ok || !v.over(e.Pos) || v.held != holdNone {
			return false
		}
		switch {
		case shape == Steps && e.Button == input.ButtonPrimary:
			v.h.BeginEdit("Draw steps", u)
			v.held, v.last = holdSteps, -1
			v.setSteps(e.Pos, u)
		case shape == Shaper && e.Button == input.ButtonPrimary:
			i := v.sc.pointAt(e.Pos)
			if i < 0 {
				st, at := v.p.current().withPoint(v.sc.pointOf(e.Pos))
				if at < 0 {
					return false
				}
				v.h.BeginEdit("Add point", u)
				v.change(st, u)
				i = at
			} else {
				v.h.BeginEdit("Move point", u)
			}
			v.held, v.point, v.sc.hot = holdPoint, i, i
		case shape == Shaper && e.Button == input.ButtonSecondary:
			i := v.sc.pointAt(e.Pos)
			if i < 0 || len(v.p.current().Curve) <= 1 {
				return false
			}
			v.edit("Delete point", u, func(st State) State { return st.withoutPoint(i) })
			v.sc.hot = -1
		default:
			return false
		}
	case input.PointerUp:
		if v.held == holdNone {
			return false
		}
		v.release(u)
	default:
		return false
	}
	return true
}

// release ends the drag under way.
func (v *panel) release(u *gunim.UI) {
	if v.held == holdNone {
		return
	}
	v.held = holdNone
	v.h.EndEdit(u)
	u.Invalidate()
}

// Overhear implements [gunim.Overhearer]: a second button pressed while
// a drag runs ends it where it got to.
func (v *panel) Overhear(e input.Event, u *gunim.UI) {
	if _, ok := e.(input.PointerDown); ok {
		v.release(u)
	}
}

// Cursor implements [gunim.CursorShaper]: a crosshair over a scope that
// edits.
func (v *panel) Cursor(pos geom.Point) input.Cursor {
	if _, ok := v.editing(); ok && (v.held != holdNone || v.over(pos)) {
		return input.CursorCrosshair
	}
	return input.CursorInherit
}
