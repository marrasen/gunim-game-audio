package plugfx

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The effects' colours: the highs as they die in a reverb, the right
// side's echoes in a delay, and each kind of unit in Colour's chain.
var (
	FxBright     = theme.Color("plugfx.bright", color.NRGBA{R: 0x8a, G: 0xd4, B: 0xff, A: 0xff})
	FxRight      = theme.Color("plugfx.right", color.NRGBA{R: 0xff, G: 0xa8, B: 0x5a, A: 0xff})
	FxChorus     = theme.Color("plugfx.chorus", color.NRGBA{R: 0x8a, G: 0xd4, B: 0xff, A: 0xff})
	FxPhaser     = theme.Color("plugfx.phaser", color.NRGBA{R: 0xc0, G: 0x9a, B: 0xff, A: 0xff})
	FxDistortion = theme.Color("plugfx.distortion", color.NRGBA{R: 0xff, G: 0x8a, B: 0x5a, A: 0xff})
)

// values are the parameters as the views show them, by ID.
type values map[uint32]float64

func valuesOf(d *plug.Descriptor, h plug.Host) values {
	v := values{}
	for _, p := range d.Params {
		v[p.ID] = h.Param(p.ID)
	}
	return v
}

// gesture sets parameter id to v through h, as one step of the host's
// history.
func gesture(h plug.Host, id uint32, v float64, u *gunim.UI) {
	h.Begin(id, u)
	h.Set(id, v, u)
	h.End(id, u)
}

// meter is a pair of levels, left and right, of the sound as it is
// heard, taken a frame of the window at a time.
type meter struct {
	l      plug.Listener
	levels *audioui.Levels
	since  time.Duration
}

func newMeter(s plug.Sound) *meter {
	return &meter{l: plug.Listener{Sound: s}, levels: audioui.NewLevels()}
}

// step takes what was heard over dt, and says the levels still move.
func (m *meter) step(dt time.Duration) bool {
	m.since += dt
	frames, rate, playing := m.l.Hear(8192, dt)
	if len(frames) > 0 {
		m.levels.Take(frames, rate, m.since)
		m.since = 0
		return true
	}
	if playing {
		return true
	}
	m.since = 0
	m.levels.Quiet(dt)
	return m.levels.Peak[0] > -89 || m.levels.Peak[1] > -89 || m.levels.RMS[0] > -89 || m.levels.RMS[1] > -89
}

// paintBars paints the levels as two rows of lights, left over right,
// across r.
func paintBars(p *paint.Painter, th *theme.Live, r geom.Rect, lv *audioui.Levels) {
	audioui.ChipLEDs.Paint(p, th, r, lv)
}

// display is a panel's picture over its controls: told where the
// sections start, so what it draws stands above them.
type display interface {
	gunim.Node
	setFoot(y float32)
}

// panelInset is how far in from the panel's edges the glass stands,
// and meterW how wide the meter at its top left is.
const (
	panelInset = 16
	meterW     = 120
)

// panel is a display filling the panel, the sections along its foot,
// the level coming out at its top left, and the tuner at its top right.
type panel struct {
	v        values
	disp     display
	sections []*plugui.Section
	out      *meter
	outBox   *meterBox
	tuner    *plugui.Tuner
	// follow, where set, hears each parameter the panel follows.
	follow func(id uint32, v float64, u *gunim.UI)
}

func newPanel(h plug.Host, v values, disp display, sections ...*plugui.Section) *panel {
	_, out := h.Sound()
	p := &panel{v: v, disp: disp, sections: sections, out: newMeter(out),
		tuner: plugui.NewTuner("Tune how the sound shows behind the controls")}
	p.outBox = &meterBox{m: p.out}
	return p
}

// Children implements [gunim.Composite].
func (p *panel) Children() []gunim.Node {
	kids := make([]gunim.Node, 0, 4+len(p.sections))
	kids = append(kids, p.disp, p.outBox)
	for _, s := range p.sections {
		kids = append(kids, s)
	}
	return append(kids, p.tuner.Button, p.tuner.Card)
}

// Layout implements [gunim.Node].
func (p *panel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	kids.At(0).Layout(gunim.Tight(size))
	kids.At(0).Place(geom.Point{})
	top := plugui.PlaceSections(size, panelInset, p.sections, func(i int, r geom.Rect) {
		kids.At(2 + i).Layout(gunim.Tight(r.Size()))
		kids.At(2 + i).Place(r.Min)
	})
	p.disp.setFoot(top - plugui.SectionGap)
	kids.At(1).Layout(gunim.Tight(geom.Sz(meterW, 8)))
	kids.At(1).Place(geom.Pt(size.W-panelInset-meterW-40, panelInset+4))
	n := 2 + len(p.sections)
	kids.At(n).Layout(gunim.Tight(geom.Sz(32, 32)))
	kids.At(n).Place(geom.Pt(size.W-32-panelInset/2, panelInset/2))
	cs := kids.At(n + 1).Layout(gunim.Constraints{Max: geom.Sz(plugui.TunerW, plugui.TunerH)})
	kids.At(n + 1).Place(geom.Pt(size.W-panelInset/2-max(cs.W, plugui.TunerW), panelInset/2+36))
	return size
}

// Paint implements [gunim.Node].
func (p *panel) Paint(pt *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(pt)
	}
}

// Follow implements [plug.Follower]: the knobs glide to their
// parameters set elsewhere, unless held, and the display redraws.
func (p *panel) Follow(id uint32, v float64, u *gunim.UI) {
	p.v[id] = v
	for _, s := range p.sections {
		s.Follow(id, v, u)
	}
	if p.follow != nil {
		p.follow(id, v, u)
	}
	u.Invalidate()
}

// meterBox shows the levels coming out, left over right.
type meterBox struct{ m *meter }

// Step implements [gunim.Animator].
func (b *meterBox) Step(dt time.Duration) bool { return b.m.step(dt) }

// Layout implements [gunim.Node].
func (b *meterBox) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

// Paint implements [gunim.Node].
func (b *meterBox) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	paintBars(p, f.Theme, geom.Rect{Max: box.Point()}, b.m.levels)
}

// grip is what a display's pointer holds: a display's drags run from a
// press to its release, and a second button pressed meanwhile ends them
// where they are, as a knob's.
type grip struct {
	held int
	from geom.Point
	was  [2]float64
}

// press takes hold of what from e's place, as it was.
func (g *grip) press(what int, e input.PointerDown, was ...float64) {
	g.held, g.from = what, e.Pos
	g.was = [2]float64{}
	copy(g.was[:], was)
}

// paintGround paints a display's ground and clips what follows to it,
// and returns the call that ends the clip.
func paintGround(p *paint.Painter, th *theme.Live, box geom.Size) func() {
	radius := widget.CardRadius.Get(th)
	whole := geom.Rect{Max: box.Point()}
	p.RRect(whole, radius, paint.Solid(audioui.Ground.Get(th)))
	return p.Layer(paint.LayerOpts{Bounds: whole, Opacity: 1, Clip: true, Radius: radius})
}

// handle paints a handle a display's pointer drags, lit by lit.
func handle(p *paint.Painter, c geom.Point, col color.NRGBA, lit float32) {
	r := 5 + 2*lit
	if lit > 0.01 {
		p.ShadowRRect(geom.Rc(c.X-r, c.Y-r, 2*r, 2*r), r, paint.Solid(audioui.Faded(col, 0)),
			paint.Shadow{Blur: 10 * lit, Color: audioui.Faded(col, 0.5*lit)})
	}
	p.RRect(geom.Rc(c.X-r, c.Y-r, 2*r, 2*r), r, paint.Solid(col))
}
