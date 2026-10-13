package plugfx

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
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

	"github.com/marrasen/gunim-game-audio/synth"
)

// reverbView returns the reverb's view at size s: its decay and level
// in a chip, and the decay over the knobs in a panel, which serves in
// full too.
func reverbView(s plug.Size, h plug.Host) gunim.Node {
	v := valuesOf(&ReverbDescriptor, h)
	if s == plug.Chip {
		return &reverbChip{v: v, out: newMeter(soundOut(h))}
	}
	disp := newDecayGraph(h, v)
	pn := newPanel(h, v, disp,
		plugui.NewSection(h, &ReverbDescriptor, "SPACE", 0, SizeID, DecayID, PreDelayID),
		plugui.NewSection(h, &ReverbDescriptor, "TONE", 0, DampingID, WidthID),
		plugui.NewSection(h, &ReverbDescriptor, "OUTPUT", 0, MixID),
	)
	pn.sections[0].Knobs[0].Tooltip = "How big the room is: how far apart its first reflections come"
	pn.sections[0].Knobs[1].Tooltip = "How long the reverb takes to fall 60 dB"
	pn.sections[0].Knobs[2].Tooltip = "How long the sound takes to reach the room"
	pn.sections[1].Knobs[0].Tooltip = "How fast the highs die: more is darker"
	pn.sections[1].Knobs[1].Tooltip = "How wide the reverb sounds: none is mono"
	pn.sections[2].Knobs[0].Tooltip = "How much of the reverb is heard: all of it for a send, less for an insert"
	pn.follow = disp.follow
	return pn
}

// soundOut is the sound a host keeps of what comes out, or nil.
func soundOut(h plug.Host) plug.Sound {
	_, out := h.Sound()
	return out
}

// The decay graph's levels, at its top and its foot, in decibels.
const (
	decayTop    = 0
	decayBottom = -72
)

// highDecay is how long the highs take to fall 60 dB, of a reverb of
// decay at damping, from 0 to 100: the lines' filters take the highs
// down each time round.
func highDecay(decay, damping float64) float64 { return decay * (1 - 0.75*damping/100) }

// decayGraph draws the reverb's decay: its pre-delay, the first
// reflections its size gives, the lows falling 60 dB over its decay,
// filled, and the highs falling sooner as it is damped, glowing as the
// reverb sounds. Its handles drag the pre-delay, the size, the decay
// and the damping across.
type decayGraph struct {
	h plug.Host
	v values
	// span is how many seconds the graph shows across, gliding.
	span  *anim.Float
	glow  *anim.Float
	lit   [handles]*anim.Float
	group anim.Group
	out   *meter
	grip  grip
	hot   int
	foot  float32
	size  geom.Size
}

// The graph's handles, from 1, as grip holds them.
const (
	handlePre = iota + 1
	handleSize
	handleDecay
	handleDamping
	handles = handleDamping
)

func newDecayGraph(h plug.Host, v values) *decayGraph {
	g := &decayGraph{h: h, v: v, out: newMeter(soundOut(h)), glow: anim.NewFloat(0)}
	g.span = anim.NewFloat(float32(g.spanFor()))
	g.group.Add(g.span, g.glow)
	for i := range g.lit {
		g.lit[i] = anim.NewFloat(0)
		g.group.Add(g.lit[i])
	}
	return g
}

// spanFor is the span the graph glides to: the decay and the pre-delay,
// with room after, on a round number of seconds.
func (g *decayGraph) spanFor() float64 {
	t := g.v[PreDelayID]/1000 + g.v[DecayID]*1.2
	for _, s := range []float64{0.5, 1, 2, 3, 5, 8, 12, 16, 20, 25} {
		if t <= s {
			return s
		}
	}
	return 25
}

func (g *decayGraph) setFoot(y float32) { g.foot = y }

// follow hears a parameter set, and glides the span to fit.
func (g *decayGraph) follow(uint32, float64, *gunim.UI) {
	if s := float32(g.spanFor()); s != g.span.Target() {
		g.span.Animate(s, anim.Spring{Response: 0.45, Damping: 0.9})
	}
}

// plot is where the curves are drawn.
func (g *decayGraph) plot() geom.Rect {
	return geom.Rect{Min: geom.Pt(panelInset+8, panelInset+34), Max: geom.Pt(g.size.W-panelInset-8, g.foot-22)}
}

// xOf is where across the plot a time is, in seconds, and timeAt the
// time at x.
func (g *decayGraph) xOf(t float64) float32 {
	r := g.plot()
	return r.Min.X + r.Size().W*float32(t/max(float64(g.span.Value()), 0.1))
}

func (g *decayGraph) timeAt(x float32) float64 {
	r := g.plot()
	return float64((x-r.Min.X)/max(r.Size().W, 1)) * max(float64(g.span.Value()), 0.1)
}

// yOf is where up the plot a level is, in decibels.
func (g *decayGraph) yOf(db float64) float32 {
	r := g.plot()
	t := (db - decayBottom) / (decayTop - decayBottom)
	return r.Max.Y - r.Size().H*float32(min(max(t, 0), 1))
}

// handleAt returns where handle k stands.
func (g *decayGraph) handleAt(k int) geom.Point {
	pre := g.v[PreDelayID] / 1000
	switch k {
	case handlePre:
		return geom.Pt(g.xOf(pre), g.yOf(0))
	case handleSize:
		e := synth.ReverbEarly(g.v[SizeID])
		return geom.Pt(g.xOf(pre+e[len(e)-1]), g.yOf(-4))
	case handleDecay:
		return geom.Pt(g.xOf(pre+g.v[DecayID]), g.yOf(-60))
	default:
		return geom.Pt(g.xOf(pre+highDecay(g.v[DecayID], g.v[DampingID])), g.yOf(-60))
	}
}

// Step implements [gunim.Animator]: the span glides, and the curves
// glow as the reverb sounds.
func (g *decayGraph) Step(dt time.Duration) bool {
	moving := g.out.step(dt)
	loud := max(g.out.levels.RMS[0], g.out.levels.RMS[1])
	g.glow.Animate(min(max((loud+48)/36, 0), 1), anim.Snappy)
	return g.group.Step(dt) || moving
}

// Layout implements [gunim.Node].
func (g *decayGraph) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	g.size = c.Max
	if g.foot == 0 {
		g.foot = g.size.H
	}
	return c.Max
}

// Paint implements [gunim.Node].
func (g *decayGraph) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, accent, bright := widget.Ink.Get(th), widget.Accent.Get(th), FxBright.Get(th)
	defer paintGround(p, th, box)()
	r := g.plot()
	if r.Size().H < 20 || r.Size().W < 40 {
		return
	}
	// The grid: a line each 12 dB above the foot, and each step of
	// time across.
	for db := decayTop; db > decayBottom; db -= 12 {
		y := g.yOf(float64(db))
		p.RRect(geom.Rc(r.Min.X, y, r.Size().W, 1), 0, paint.Solid(audioui.Faded(ink, 0.06)))
		label := audioui.Shaped(strconv.Itoa(db), 9, false, true)
		label.Paint(p, geom.Pt(r.Max.X-label.Advance, y+2), audioui.Faded(ink, 0.3))
	}
	span := float64(g.span.Value())
	step := niceStep(span)
	for t := 0.0; t <= span+1e-9; t += step {
		x := g.xOf(t)
		p.RRect(geom.Rc(x, r.Min.Y, 1, r.Size().H), 0, paint.Solid(audioui.Faded(ink, 0.06)))
		label := audioui.Shaped(seconds(t), 9, false, true)
		label.Paint(p, geom.Pt(min(x+3, r.Max.X-label.Advance), r.Max.Y+6), audioui.Faded(ink, 0.35))
	}
	pre, decay := g.v[PreDelayID]/1000, g.v[DecayID]
	glow := g.glow.Value()
	// The pre-delay, a band of silence.
	p.RRect(geom.Rc(r.Min.X, r.Min.Y, g.xOf(pre)-r.Min.X, r.Size().H), 0, paint.Solid(audioui.Faded(ink, 0.04)))
	// The lows falling, filled, and the highs falling sooner.
	fill := audioui.Faded(accent, 0.16+0.16*glow)
	x0, x1 := g.xOf(pre), g.xOf(pre+decay*1.2)
	for x := float32(math.Floor(float64(x0))); x < min(x1, r.Max.X); x++ {
		db := -60 * (g.timeAt(max(x, x0)) - pre) / decay
		y := g.yOf(db)
		p.RRect(geom.Rc(x, y, 1, r.Max.Y-y), 0, paint.Solid(fill))
	}
	plugui.Darken(p, geom.Rect{Max: box.Point()}, 1)
	lines := p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: plugui.Lines(1)})
	defer lines()
	curve := func(d float64, col color.NRGBA, w float32) {
		prev := geom.Pt(x0, g.yOf(0))
		for x := x0 + 3; ; x += 3 {
			x = min(x, x1, r.Max.X)
			pt := geom.Pt(x, g.yOf(-60*(g.timeAt(x)-pre)/d))
			audioui.Segment(p, prev, pt, w, col)
			prev = pt
			if x >= min(x1, r.Max.X) || pt.Y >= r.Max.Y {
				break
			}
		}
	}
	curve(decay, audioui.Faded(accent, 0.8+0.2*glow), 2)
	curve(highDecay(decay, g.v[DampingID]), audioui.Faded(bright, 0.6+0.4*glow), 1.5)
	// The first reflections, a tick each, as loud as they come.
	for i, t := range synth.ReverbEarly(g.v[SizeID]) {
		x := g.xOf(pre + t)
		y := g.yOf(-4 - 2*float64(i))
		p.RRect(geom.Rc(x-1, y, 2, r.Max.Y-y), 1, paint.Solid(audioui.Faded(ink, 0.25+0.2*g.lit[handleSize-1].Value())))
	}
	// The -60 dB line, which the decay reaches.
	y60 := g.yOf(-60)
	for x := r.Min.X; x < r.Max.X; x += 8 {
		p.RRect(geom.Rc(x, y60, 4, 1), 0, paint.Solid(audioui.Faded(ink, 0.25)))
	}
	cols := [handles]color.NRGBA{ink, ink, accent, bright}
	for k := 1; k <= handles; k++ {
		lit := g.lit[k-1].Value()
		at := g.handleAt(k)
		if at.X > r.Max.X+8 {
			continue
		}
		handle(p, at, audioui.Faded(cols[k-1], 0.6+0.4*lit), lit)
		if lit > 0.05 {
			words := g.handleText(k)
			label := audioui.Shaped(words, 11, true, false)
			lx := min(at.X+10, r.Max.X-label.Advance)
			label.Paint(p, geom.Pt(lx, at.Y-20), audioui.Faded(cols[k-1], lit))
		}
	}
	title := audioui.Shaped(fmt.Sprintf("%s  ·  %s", seconds(decay), roomName(g.v[SizeID], decay)), 13, true, false)
	title.Paint(p, geom.Pt(panelInset+8, panelInset+4), audioui.Faded(ink, 0.8))
}

// handleText is what handle k shows as it is lit: its value.
func (g *decayGraph) handleText(k int) string {
	switch k {
	case handlePre:
		return "Pre-delay " + millis(g.v[PreDelayID])
	case handleSize:
		return fmt.Sprintf("Size %.0f%%", g.v[SizeID]*100)
	case handleDecay:
		return "Decay " + seconds(g.v[DecayID])
	default:
		return fmt.Sprintf("Damping %.0f%%", g.v[DampingID])
	}
}

// roomName names the room a size and a decay sound as.
func roomName(size, decay float64) string {
	switch {
	case decay > 6:
		return "Cathedral"
	case size >= 1.05 || decay > 2.8:
		return "Hall"
	case size < 0.55 && decay < 1:
		return "Small room"
	case size < 0.8:
		return "Room"
	default:
		return "Chamber"
	}
}

// niceStep is a round step of time for a grid across span seconds.
func niceStep(span float64) float64 {
	for _, s := range []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5} {
		if span/s <= 8 {
			return s
		}
	}
	return 5
}

// hit returns the handle near pos, or 0.
func (g *decayGraph) hit(pos geom.Point) int {
	best, most := 0, float32(12)
	for k := 1; k <= handles; k++ {
		at := g.handleAt(k)
		if d := float32(math.Hypot(float64(pos.X-at.X), float64(pos.Y-at.Y))); d < most {
			best, most = k, d
		}
	}
	return best
}

// light lights handle k alone.
func (g *decayGraph) light(k int, u *gunim.UI) {
	g.hot = k
	for i, l := range g.lit {
		to := float32(0)
		if i+1 == k {
			to = 1
		}
		if l.Target() != to {
			l.Animate(to, widget.Quick.Get(u.Theme()))
		}
	}
}

// idOf is the parameter handle k drags.
func idOf(k int) uint32 {
	return [...]uint32{handlePre: PreDelayID, handleSize: SizeID, handleDecay: DecayID, handleDamping: DampingID}[k]
}

// Handle implements [gunim.Handler]: a handle drags across, one gesture
// a drag; a double-click sets it back to its default.
func (g *decayGraph) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if g.grip.held != 0 {
			g.drag(e.Pos, u)
			break
		}
		g.light(g.hit(e.Pos), u)
	case input.PointerLeave:
		if g.grip.held == 0 {
			g.light(0, u)
		}
	case input.PointerDown:
		k := g.hit(e.Pos)
		if e.Button != input.ButtonPrimary || k == 0 {
			return false
		}
		id := idOf(k)
		if e.Clicks == 2 {
			prm, _ := ReverbDescriptor.Param(id)
			gesture(g.h, id, prm.Default, u)
			g.v[id] = prm.Default
			g.follow(id, prm.Default, u)
			break
		}
		g.grip.press(k, e, g.v[id])
		g.light(k, u)
		g.h.Begin(id, u)
	case input.PointerUp:
		if g.grip.held == 0 {
			return false
		}
		g.release(u)
		g.light(g.hit(e.Pos), u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Overhear implements [gunim.Overhearer]: a second button pressed while
// a drag runs ends it where it got to.
func (g *decayGraph) Overhear(e input.Event, u *gunim.UI) {
	if _, ok := e.(input.PointerDown); ok && g.grip.held != 0 {
		g.release(u)
		g.light(0, u)
		u.Invalidate()
	}
}

// release ends the drag under way.
func (g *decayGraph) release(u *gunim.UI) {
	g.h.End(idOf(g.grip.held), u)
	g.grip.held = 0
	g.follow(0, 0, u)
}

// drag moves the handle held to the pointer at pos.
func (g *decayGraph) drag(pos geom.Point, u *gunim.UI) {
	k := g.grip.held
	id := idOf(k)
	prm, _ := ReverbDescriptor.Param(id)
	pre := g.v[PreDelayID] / 1000
	t := g.timeAt(pos.X)
	var v float64
	switch k {
	case handlePre:
		v = t * 1000
	case handleSize:
		v = (t - pre) / synth.ReverbEarly(1)[7]
	case handleDecay:
		v = t - pre
	default:
		// From highDecay: the damping that brings the highs down here.
		v = (1 - (t-pre)/max(g.v[DecayID], 0.1)) / 0.75 * 100
	}
	v = prm.Clamp(v)
	g.v[id] = v
	g.h.Set(id, v, u)
}

// Access implements [gunim.Accessible].
func (g *decayGraph) Access() access.Info {
	return access.Info{Role: access.RoleGroup, Name: "Decay", Value: fmt.Sprintf("%s, %s, pre-delay %s, damping %.0f%%",
		roomName(g.v[SizeID], g.v[DecayID]), seconds(g.v[DecayID]), millis(g.v[PreDelayID]), g.v[DampingID])}
}

// reverbChip shows, in a plugin's chip in a chain, the decay as a
// small curve, how long it is, and the level coming out.
type reverbChip struct {
	v   values
	out *meter
}

// Step implements [gunim.Animator].
func (c *reverbChip) Step(dt time.Duration) bool { return c.out.step(dt) }

// Layout implements [gunim.Node].
func (c *reverbChip) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return cs.Constrain(geom.Sz(cs.Max.W, 20))
}

// Paint implements [gunim.Node].
func (c *reverbChip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, accent := widget.Ink.Get(th), widget.Accent.Get(th)
	// The curve, over 26 points, as the decay falls across a fixed 8 s.
	const w = 26
	pre := float32(c.v[PreDelayID] / 1000 / 8 * w)
	reach := float32(c.v[DecayID] / 8 * w)
	top, foot := box.H*0.2, box.H*0.85
	prev := geom.Pt(0, foot)
	for x := float32(0); x <= w; x += 2 {
		y := foot
		if x >= pre {
			y = top + (foot-top)*min((x-pre)/max(reach, 0.5), 1)
		}
		pt := geom.Pt(x, y)
		audioui.Segment(p, prev, pt, 1.5, accent)
		prev = pt
	}
	words := audioui.Shaped(seconds(c.v[DecayID])+"  "+percent(c.v[MixID]), plugui.ChipTextSize, false, false)
	words.PaintFaded(p, geom.Pt(w+6, (box.H-words.Height())/2), audioui.Faded(ink, plugui.ChipTextInk), box.W-w-6-30, 12)
	paintBars(p, th, geom.Rc(box.W-26, box.H/2-4, 26, 8), c.out.levels)
}

// Access implements [gunim.Accessible].
func (c *reverbChip) Access() access.Info {
	return access.Info{Role: access.RoleLabel, Name: fmt.Sprintf("Reverb %s, mix %s", seconds(c.v[DecayID]), percent(c.v[MixID]))}
}

// Follow implements [plug.Follower].
func (c *reverbChip) Follow(id uint32, v float64, u *gunim.UI) {
	c.v[id] = v
	u.Invalidate()
}
