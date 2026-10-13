package plugfx

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// The tips of the delay's selectors' options.
var (
	noteTips = []string{"A thirty-second note", "A sixteenth note", "An eighth note", "A quarter note, a beat",
		"A half note", "A whole note, a bar of four"}
	feelTips   = []string{"The note as it is", "Half as long again", "Two thirds as long, three in the note's two"}
	stereoTips = []string{"The echoes cross from side to side", "Each side echoes on its own side"}
)

// delayView returns the delay's view at size s: its time and level in a
// chip, and the echoes over the knobs in a panel, which serves in full
// too.
func delayView(s plug.Size, h plug.Host) gunim.Node {
	v := valuesOf(&DelayDescriptor, h)
	if s == plug.Chip {
		return &delayChip{v: v, out: newMeter(soundOut(h))}
	}
	disp := newTaps(h, v)
	pick := func(id uint32) func(int, *gunim.UI) {
		return func(i int, u *gunim.UI) {
			if id == PingPongID {
				i = 1 - i
			}
			gesture(h, id, float64(i), u)
			v[id] = float64(i)
		}
	}
	note := plugui.NewSelector("Note", NoteNames, noteTips)
	note.Columns = 3
	note.OnPick = pick(NoteID)
	feel := plugui.NewSelector("Feel", FeelNames, feelTips)
	feel.OnPick = pick(FeelID)
	stereo := plugui.NewSelector("Echoes", []string{"Ping-pong", "Stereo"}, stereoTips)
	stereo.OnPick = pick(PingPongID)
	sync := plugui.NewSection(h, &DelayDescriptor, "SYNC", SyncID).Add(note, 64).Add(feel, 64)
	free := plugui.NewSection(h, &DelayDescriptor, "TIME", 0, TimeID)
	echoes := plugui.NewSection(h, &DelayDescriptor, "ECHOES", 0, FeedbackID).Add(stereo, 48)
	filter := plugui.NewSection(h, &DelayDescriptor, "FEEDBACK FILTER", 0, LowCutID, HighCutID)
	out := plugui.NewSection(h, &DelayDescriptor, "OUTPUT", 0, DelayMixID)
	free.Knobs[0].Tooltip = "How far apart the echoes are, unsynced"
	echoes.Knobs[0].Tooltip = "How loud each time round is, of the one before"
	filter.Knobs[0].Tooltip = "Where the feedback loses its lows, time round after time round"
	filter.Knobs[1].Tooltip = "Where the feedback loses its highs, so each echo is darker"
	out.Knobs[0].Tooltip = "How much of the echoes is heard: all of them for a send, less for an insert"
	pn := newPanel(h, v, disp, sync, free, echoes, filter, out)
	show := func(u *gunim.UI) {
		note.SetSelected(stepOf(v[NoteID], len(NoteNames)), u)
		feel.SetSelected(stepOf(v[FeelID], len(FeelNames)), u)
		stereo.SetSelected(1-stepOf(v[PingPongID], 2), u)
		synced := v[SyncID] > 0.5
		note.Locked, feel.Locked = !synced, !synced
		free.Knobs[0].Disabled = synced
	}
	show(nil)
	pn.follow = func(id uint32, x float64, u *gunim.UI) {
		show(u)
		disp.follow(u)
	}
	return pn
}

// taps draws the delay's echoes along their time: in beats where it is
// synced, in seconds where not; the left side's up and the right side's
// down from the middle, each as loud as the feedback leaves it, and
// darker as the filters take more of it. The first echo's handle drags
// the time across, and the feedback's handle up and down.
type taps struct {
	h    plug.Host
	v    values
	span *anim.Float
	glow *anim.Float
	lit  [2]*anim.Float
	anim.Group
	out  *meter
	grip grip
	foot float32
	size geom.Size
}

// The handles, from 1, as grip holds them.
const (
	handleTime = iota + 1
	handleFeedback
)

// tapsFloor is the level at the foot of an echo's bar, in decibels.
const tapsFloor = -48

func newTaps(h plug.Host, v values) *taps {
	t := &taps{h: h, v: v, out: newMeter(soundOut(h)), glow: anim.NewFloat(0)}
	t.span = anim.NewFloat(float32(t.spanFor()))
	t.Add(t.span, t.glow)
	for i := range t.lit {
		t.lit[i] = anim.NewFloat(0)
		t.Add(t.lit[i])
	}
	return t
}

// settings are the delay's settings as the view's parameters say.
func (t *taps) settings() delaySettings {
	var s delaySettings
	for _, id := range []uint32{SyncID, NoteID, FeelID, TimeID, FeedbackID, PingPongID, LowCutID, HighCutID} {
		s.take(id, t.v[id])
	}
	return s
}

// gap is how far apart the echoes are, in the graph's units: beats
// where synced, seconds where not.
func (t *taps) gap() float64 {
	s := t.settings()
	if s.sync {
		return s.beats()
	}
	return s.ms / 1000
}

// spanFor is the span the graph glides to: room for the echoes until
// they fall under its foot, four at least and twelve at most.
func (t *taps) spanFor() float64 {
	s := t.settings()
	n := 1.0
	if s.feedback > 0.001 {
		n = min(max(math.Ceil(tapsFloor/(20*math.Log10(s.feedback))), 2), 6)
	}
	if s.pingPong {
		n *= 2
	}
	return t.gap() * (max(n, 4) + 0.6)
}

func (t *taps) setFoot(y float32) { t.foot = y }

// follow glides the span to fit.
func (t *taps) follow(*gunim.UI) {
	if s := float32(t.spanFor()); s != t.span.Target() {
		t.span.Animate(s, anim.Spring{Response: 0.45, Damping: 0.9})
	}
}

// plot is where the echoes are drawn.
func (t *taps) plot() geom.Rect {
	return geom.Rect{Min: geom.Pt(panelInset+8, panelInset+34), Max: geom.Pt(t.size.W-panelInset-8, t.foot-22)}
}

// xOf is where across the plot a time is, in the graph's units.
func (t *taps) xOf(at float64) float32 {
	r := t.plot()
	return r.Min.X + r.Size().W*float32(at/max(float64(t.span.Value()), 1e-3))
}

func (t *taps) timeAt(x float32) float64 {
	r := t.plot()
	return float64((x-r.Min.X)/max(r.Size().W, 1)) * max(float64(t.span.Value()), 1e-3)
}

// barOf is how far from the middle an echo's bar reaches, at db.
func (t *taps) barOf(db float64) float32 {
	r := t.plot()
	return r.Size().H / 2 * float32(min(max((db-tapsFloor)/-tapsFloor, 0), 1))
}

// echo is one echo: when it comes, on which side, and how loud.
type echo struct {
	at     float64
	right  bool
	db     float64
	index  int
	stereo bool
}

// echoes lists the echoes the graph shows.
func (t *taps) echoes() []echo {
	s := t.settings()
	gap := t.gap()
	var out []echo
	for k := 1; ; k++ {
		at := gap * float64(k)
		if at > float64(t.span.Value()) || k > 64 {
			break
		}
		rounds := k - 1
		if s.pingPong {
			rounds = (k - 1) / 2
		}
		db := 20 * math.Log10(math.Pow(max(s.feedback, 1e-6), float64(rounds)))
		out = append(out, echo{at: at, right: s.pingPong && k%2 == 0, db: db, index: k, stereo: !s.pingPong})
	}
	return out
}

// handleAt returns where handle k stands.
func (t *taps) handleAt(k int) geom.Point {
	r := t.plot()
	mid := (r.Min.Y + r.Max.Y) / 2
	es := t.echoes()
	if k == handleTime || len(es) < 3 {
		return geom.Pt(t.xOf(t.gap()), mid-t.barOf(0))
	}
	// The feedback's: the first echo the feedback turns down, on the
	// left.
	e := es[1]
	if !e.stereo {
		e = es[2]
	}
	return geom.Pt(t.xOf(e.at), mid-t.barOf(e.db))
}

// Step implements [gunim.Animator].
func (t *taps) Step(dt time.Duration) bool {
	moving := t.out.step(dt)
	loud := max(t.out.levels.RMS[0], t.out.levels.RMS[1])
	t.glow.Animate(min(max((loud+48)/36, 0), 1), anim.Snappy)
	return t.Group.Step(dt) || moving
}

// Layout implements [gunim.Node].
func (t *taps) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	if t.foot == 0 {
		t.foot = t.size.H
	}
	return c.Max
}

// Paint implements [gunim.Node].
func (t *taps) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, accent, right := widget.Ink.Get(th), widget.Accent.Get(th), FxRight.Get(th)
	defer paintGround(p, th, box)()
	r := t.plot()
	if r.Size().H < 20 || r.Size().W < 40 {
		return
	}
	s := t.settings()
	mid := (r.Min.Y + r.Max.Y) / 2
	// The grid: beats where synced, else steps of time.
	span := float64(t.span.Value())
	step, unit := niceStep(span), seconds
	if s.sync {
		step = max(math.Pow(2, math.Ceil(math.Log2(span/8))), 0.125)
		unit = func(b float64) string { return trimFloat(b) + " ♩" }
	}
	for at := 0.0; at <= span+1e-9; at += step {
		x := t.xOf(at)
		p.RRect(geom.Rc(x, r.Min.Y, 1, r.Size().H), 0, paint.Solid(audioui.Faded(ink, 0.06)))
		label := audioui.Shaped(unit(at), 9, false, true)
		label.Paint(p, geom.Pt(min(x+3, r.Max.X-label.Advance), r.Max.Y+6), audioui.Faded(ink, 0.35))
	}
	p.RRect(geom.Rc(r.Min.X, mid, r.Size().W, 1), 0, paint.Solid(audioui.Faded(ink, 0.18)))
	for _, db := range []float64{-12, -24, -36} {
		for _, y := range []float32{mid - t.barOf(db), mid + t.barOf(db)} {
			for x := r.Min.X; x < r.Max.X; x += 8 {
				p.RRect(geom.Rc(x, y, 4, 1), 0, paint.Solid(audioui.Faded(ink, 0.07)))
			}
		}
	}
	side := audioui.Shaped("L", 9, true, false)
	side.Paint(p, geom.Pt(r.Min.X, r.Min.Y), audioui.Faded(accent, 0.6))
	side = audioui.Shaped("R", 9, true, false)
	side.Paint(p, geom.Pt(r.Min.X, r.Max.Y-12), audioui.Faded(right, 0.6))
	// The sound going in, at 0.
	p.RRect(geom.Rc(t.xOf(0)-1.5, mid-t.barOf(0), 3, 2*t.barOf(0)), 1.5, paint.Solid(audioui.Faded(ink, 0.35)))
	// The echoes, each darker by the filters, time round after time
	// round, and glowing as they sound.
	narrow := min(max(math.Log2(s.lowCut/20)/6.6+math.Log2(20000/s.highCut)/5.3, 0), 1)
	glow := t.glow.Value()
	w := min(max(r.Size().W/float32(span/t.gap())*0.22, 3), 14)
	for _, e := range t.echoes() {
		x := t.xOf(e.at)
		h := t.barOf(e.db)
		if h < 0.5 {
			continue
		}
		fade := float32(math.Pow(1-0.35*narrow, float64(e.index-1)))
		paintBar := func(up bool, col color.NRGBA) {
			y := mid
			if up {
				y = mid - h
			}
			a := (0.35 + 0.45*fade) * (0.85 + 0.15*glow)
			if glow > 0.05 {
				p.ShadowRRect(geom.Rc(x-w/2, y, w, h), w/2, paint.Solid(audioui.Faded(col, 0)),
					paint.Shadow{Blur: 12 * glow, Color: audioui.Faded(col, 0.35*glow*fade)})
			}
			p.RRect(geom.Rc(x-w/2, y, w, h), min(w/2, 3), paint.Solid(audioui.Faded(col, a)))
		}
		switch {
		case e.stereo:
			paintBar(true, accent)
			paintBar(false, right)
		case e.right:
			paintBar(false, right)
		default:
			paintBar(true, accent)
		}
	}
	for k := 1; k <= 2; k++ {
		lit := t.lit[k-1].Value()
		at := t.handleAt(k)
		handle(p, at, audioui.Faded(ink, 0.6+0.4*lit), lit)
		if lit > 0.05 {
			words := t.handleText(k)
			label := audioui.Shaped(words, 11, true, false)
			label.Paint(p, geom.Pt(min(at.X+10, r.Max.X-label.Advance), at.Y-20), audioui.Faded(ink, lit))
		}
	}
	title := audioui.Shaped(t.titleText(), 13, true, false)
	title.Paint(p, geom.Pt(panelInset+8, panelInset+4), audioui.Faded(ink, 0.8))
}

// titleText says how far apart the echoes are, how loud they come back,
// and which way they go.
func (t *taps) titleText() string {
	s := t.settings()
	way := "ping-pong"
	if !s.pingPong {
		way = "stereo"
	}
	return fmt.Sprintf("%s  ·  feedback %s  ·  %s", timeText(s), percent(s.feedback*100), way)
}

// timeText writes the echoes' time: the note where synced.
func timeText(s delaySettings) string {
	if !s.sync {
		return millis(s.ms)
	}
	switch s.feel {
	case 1:
		return NoteNames[s.note] + " dotted"
	case 2:
		return NoteNames[s.note] + " triplet"
	}
	return NoteNames[s.note]
}

// trimFloat writes a number of beats short.
func trimFloat(b float64) string {
	if b == math.Trunc(b) {
		return fmt.Sprintf("%.0f", b)
	}
	return fmt.Sprintf("%g", math.Round(b*1000)/1000)
}

// handleText is what handle k shows as it is lit.
func (t *taps) handleText(k int) string {
	if k == handleTime {
		return timeText(t.settings())
	}
	return "Feedback " + percent(t.v[FeedbackID])
}

// hit returns the handle near pos, or 0.
func (t *taps) hit(pos geom.Point) int {
	best, most := 0, float32(12)
	for k := 1; k <= 2; k++ {
		at := t.handleAt(k)
		if d := float32(math.Hypot(float64(pos.X-at.X), float64(pos.Y-at.Y))); d < most {
			best, most = k, d
		}
	}
	return best
}

// light lights handle k alone.
func (t *taps) light(k int, u *gunim.UI) {
	for i, l := range t.lit {
		to := float32(0)
		if i+1 == k {
			to = 1
		}
		if l.Target() != to {
			l.Animate(to, widget.Quick.Get(u.Theme()))
		}
	}
}

// ids are the parameters handle k drags: the note and its feel where
// synced, or the time; or the feedback.
func (t *taps) ids(k int) []uint32 {
	switch {
	case k == handleFeedback:
		return []uint32{FeedbackID}
	case t.v[SyncID] > 0.5:
		return []uint32{NoteID, FeelID}
	}
	return []uint32{TimeID}
}

// Handle implements [gunim.Handler]: the time's handle drags across,
// the feedback's up and down, one gesture a drag.
func (t *taps) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if t.grip.held != 0 {
			t.drag(e.Pos, u)
			break
		}
		t.light(t.hit(e.Pos), u)
	case input.PointerLeave:
		if t.grip.held == 0 {
			t.light(0, u)
		}
	case input.PointerDown:
		k := t.hit(e.Pos)
		if e.Button != input.ButtonPrimary || k == 0 {
			return false
		}
		t.grip.press(k, e, t.v[FeedbackID])
		t.light(k, u)
		for _, id := range t.ids(k) {
			t.h.Begin(id, u)
		}
	case input.PointerUp:
		if t.grip.held == 0 {
			return false
		}
		t.release(u)
		t.light(t.hit(e.Pos), u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Overhear implements [gunim.Overhearer]: a second button pressed while
// a drag runs ends it where it got to.
func (t *taps) Overhear(e input.Event, u *gunim.UI) {
	if _, ok := e.(input.PointerDown); ok && t.grip.held != 0 {
		t.release(u)
		t.light(0, u)
		u.Invalidate()
	}
}

// release ends the drag under way.
func (t *taps) release(u *gunim.UI) {
	for _, id := range t.ids(t.grip.held) {
		t.h.End(id, u)
	}
	t.grip.held = 0
	t.follow(u)
}

// set sets parameter id to v as the drag moves.
func (t *taps) set(id uint32, v float64, u *gunim.UI) {
	prm, _ := DelayDescriptor.Param(id)
	v = prm.Clamp(v)
	if t.v[id] != v {
		t.v[id] = v
		t.h.Set(id, v, u)
	}
}

// drag moves the handle held to the pointer at pos: the time to where
// the pointer is across, synced to the note nearest; the feedback to
// the level the pointer is at.
func (t *taps) drag(pos geom.Point, u *gunim.UI) {
	if t.grip.held == handleFeedback {
		r := t.plot()
		mid := (r.Min.Y + r.Max.Y) / 2
		db := tapsFloor * (1 - float64((mid-pos.Y)/max(r.Size().H/2, 1)))
		t.set(FeedbackID, 100*math.Pow(10, min(db, 0)/20), u)
		return
	}
	at := max(t.timeAt(pos.X), 1e-3)
	if t.v[SyncID] <= 0.5 {
		t.set(TimeID, at*1000, u)
		return
	}
	best, note, feel := math.Inf(1), 0, 0
	for n, beats := range noteBeats {
		for f, times := range feelTimes {
			if d := math.Abs(math.Log(beats * times / at)); d < best {
				best, note, feel = d, n, f
			}
		}
	}
	t.set(NoteID, float64(note), u)
	t.set(FeelID, float64(feel), u)
}

// Access implements [gunim.Accessible].
func (t *taps) Access() access.Info {
	return access.Info{Role: access.RoleGroup, Name: "Echoes", Value: t.titleText()}
}

// delayChip shows, in a plugin's chip in a chain, the echoes as a few
// ticks, the time, and the level coming out.
type delayChip struct {
	v   values
	out *meter
}

// Step implements [gunim.Animator].
func (c *delayChip) Step(dt time.Duration) bool { return c.out.step(dt) }

// Layout implements [gunim.Node].
func (c *delayChip) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return cs.Constrain(geom.Sz(cs.Max.W, 20))
}

// Paint implements [gunim.Node].
func (c *delayChip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, accent, right := widget.Ink.Get(th), widget.Accent.Get(th), FxRight.Get(th)
	var s delaySettings
	for id, v := range c.v {
		s.take(id, v)
	}
	mid := box.H / 2
	for k := range 5 {
		rounds := k
		if s.pingPong {
			rounds = k / 2
		}
		h := float32(math.Pow(max(s.feedback, 0.05), float64(rounds))) * (mid - 2)
		x := 2 + float32(k)*5
		switch {
		case !s.pingPong:
			p.RRect(geom.Rc(x, mid-h, 2, 2*h), 1, paint.Solid(accent))
		case k%2 == 1:
			p.RRect(geom.Rc(x, mid, 2, h), 1, paint.Solid(right))
		default:
			p.RRect(geom.Rc(x, mid-h, 2, h), 1, paint.Solid(accent))
		}
	}
	const w = 28
	words := audioui.Shaped(timeText(s), plugui.ChipTextSize, false, false)
	words.PaintFaded(p, geom.Pt(w+4, (box.H-words.Height())/2), audioui.Faded(ink, plugui.ChipTextInk), box.W-w-4-30, 12)
	paintBars(p, th, geom.Rc(box.W-26, box.H/2-4, 26, 8), c.out.levels)
}

// Access implements [gunim.Accessible].
func (c *delayChip) Access() access.Info {
	var s delaySettings
	for id, v := range c.v {
		s.take(id, v)
	}
	return access.Info{Role: access.RoleLabel, Name: "Delay " + timeText(s)}
}

// Follow implements [plug.Follower].
func (c *delayChip) Follow(id uint32, v float64, u *gunim.UI) {
	c.v[id] = v
	u.Invalidate()
}
