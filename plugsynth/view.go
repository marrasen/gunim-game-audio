package plugsynth

import (
	"image/color"
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

// View implements [plug.Viewer]: the sound and the level in a chip; the
// sound's selector and the knobs in a panel; and in full, those, what
// the sound is, and a meter of what comes out.
func (p *plugin) View(s plug.Size, h plug.Host) gunim.Node {
	switch s {
	case plug.Chip:
		return newChip(h)
	case plug.Panel:
		return newPanel(p, h, false)
	default:
		return newPanel(p, h, true)
	}
}

// meter is a pair of levels, left and right, of the sound as it is
// heard, taken a frame of the window at a time.
type meter struct {
	l      plug.Listener
	levels *audioui.Levels
	// since is the time since the last chunk of sound, which the levels
	// take it over.
	since time.Duration
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

// soundText names the sound the Sound parameter's value v picks.
func soundText(v float64) string { return Sounds[soundAt(v)].Short }

// chip shows the sound, in a plugin's chip in a chain, and, beside it,
// the level that comes out, left over right.
type chip struct {
	label *plugui.ChipText
	out   *meter
}

func newChip(h plug.Host) *chip {
	_, out := h.Sound()
	return &chip{label: plugui.NewChipText(soundText(h.Param(SoundID))), out: newMeter(out)}
}

// chipBars is how wide the chip's bars are.
const chipBars = 40

// Children implements [gunim.Composite].
func (c *chip) Children() []gunim.Node { return []gunim.Node{c.label} }

// Layout implements [gunim.Node]: the sound, and the bars at the right.
func (c *chip) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kids.At(0).Layout(gunim.Constraints{Max: geom.Sz(max(cs.Max.W-chipBars-6, 0), cs.Max.H)})
	kids.At(0).Place(geom.Point{})
	return cs.Max
}

// Paint implements [gunim.Node]: each channel's RMS over its peak,
// fainter.
func (c *chip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	ink, ground, over := widget.Ink.Get(f.Theme), audioui.Ground.Get(f.Theme), audioui.Over.Get(f.Theme)
	x, h := box.W-chipBars, float32(3)
	top := (box.H - 2*h - 2) / 2
	lv := c.out.levels
	for ch := range 2 {
		y := top + float32(ch)*(h+2)
		p.RRect(geom.Rc(x, y, chipBars, h), h/2, paint.Solid(audioui.Faded(ground, 0.85)))
		if lv.Peak[ch] > -59 {
			p.RRect(geom.Rc(x, y, chipBars*audioui.MeterAt(lv.Peak[ch]), h), h/2, paint.Solid(audioui.Faded(ink, 0.3)))
		}
		if lv.RMS[ch] > -59 {
			col := ink
			if lv.Peak[ch] > -0.1 {
				col = over
			}
			p.RRect(geom.Rc(x, y, chipBars*audioui.MeterAt(lv.RMS[ch]), h), h/2, paint.Solid(audioui.Faded(col, 0.85)))
		}
	}
}

// Step implements [gunim.Animator].
func (c *chip) Step(dt time.Duration) bool { return c.out.step(dt) }

// Access implements [gunim.Accessible]: the chip reads as the sound.
func (c *chip) Access() access.Info { return c.label.Access() }

// Follow implements [plug.Follower].
func (c *chip) Follow(id uint32, v float64, u *gunim.UI) {
	if id == SoundID {
		c.label.Text = soundText(v)
		u.Invalidate()
	}
}

// panel is the sound's name over the sections: the sound's selector,
// the filter, the envelope and the level. In full, what the sound is
// shows under its name, and a meter of what comes out at the left.
type panel struct {
	p        *plugin
	full     bool
	sound    int
	pick     *plugui.Selector
	sections []*plugui.Section
	out      *meter
}

func newPanel(p *plugin, h plug.Host, full bool) *panel {
	names := make([]string, len(Sounds))
	tips := make([]string, len(Sounds))
	for i, s := range Sounds {
		names[i], tips[i] = s.Short, s.Preset+": "+p.sounds[i].about
	}
	v := &panel{p: p, full: full, sound: soundAt(h.Param(SoundID))}
	v.pick = plugui.NewSelector("Sound", names, tips)
	v.pick.SetSelected(v.sound, nil)
	v.pick.OnPick = func(i int, u *gunim.UI) {
		h.Begin(SoundID, u)
		h.Set(SoundID, float64(i), u)
		h.End(SoundID, u)
		v.sound = i
		v.dim()
	}
	v.sections = []*plugui.Section{
		plugui.NewSection(h, &Descriptor, "SOUND", 0).Add(v.pick, float32(len(Sounds))*16+16),
		plugui.NewSection(h, &Descriptor, "FILTER", 0, CutoffID, ResID),
		plugui.NewSection(h, &Descriptor, "ENVELOPE", 0, AttackID, ReleaseID),
		plugui.NewSection(h, &Descriptor, "OUTPUT", 0, GainID),
	}
	_, out := h.Sound()
	v.out = newMeter(out)
	v.dim()
	return v
}

// dim greys the knobs the sound has nothing for: the filter's, on a
// sound with none, and the envelope's, on the drums.
func (v *panel) dim() {
	s := v.p.sounds[v.sound]
	for _, sec := range v.sections {
		for _, k := range sec.Knobs {
			switch k.Param().ID {
			case CutoffID, ResID:
				k.Disabled = !s.filter
			case AttackID, ReleaseID:
				k.Disabled = !s.envelope
			}
		}
	}
}

// The panel's sizes: how far in from its edges it draws, and where the
// meter stands in full.
const (
	panelInset = 16
	meterTop   = 60
)

// Children implements [gunim.Composite].
func (v *panel) Children() []gunim.Node {
	kids := make([]gunim.Node, len(v.sections))
	for i, s := range v.sections {
		kids[i] = s
	}
	return kids
}

// Layout implements [gunim.Node]: the sections along the foot, in rows
// as many as fit, each row centred; in full, right of the meter.
func (v *panel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	left := float32(0)
	if v.full {
		left = audioui.MeterW + 2*panelInset
	}
	plugui.PlaceSections(geom.Sz(max(size.W-left, 0), size.H), panelInset, v.sections, func(i int, r geom.Rect) {
		kids.At(i).Layout(gunim.Tight(r.Size()))
		kids.At(i).Place(r.Min.Add(geom.Pt(left, 0)))
	})
	return size
}

// Paint implements [gunim.Node]: the sound's name, and in full what it
// is and the meter, then the sections.
func (v *panel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	x := float32(panelInset)
	if v.full {
		bottom := max(box.H-panelInset, meterTop+40)
		audioui.PaintMeter(pt, f.Theme, geom.Rect{Min: geom.Pt(panelInset, meterTop), Max: geom.Pt(panelInset+audioui.MeterW, bottom)}, v.out.levels)
		x += audioui.MeterW + panelInset
	}
	s := Sounds[v.sound]
	audioui.Shaped(s.Preset, 20, true, false).Paint(pt, geom.Pt(x, panelInset), ink)
	if v.full {
		fit(pt, v.p.sounds[v.sound].about, geom.Pt(x, panelInset+30), box.W-x-panelInset, audioui.Faded(ink, 0.7))
	}
	for i := range kids.Len() {
		kids.At(i).Paint(pt)
	}
}

// fit draws s at at, in 12 points, cut short with an ellipsis where it
// is wider than room.
func fit(p *paint.Painter, s string, at geom.Point, room float32, c color.NRGBA) {
	run := audioui.Shaped(s, 12, false, false)
	if run.Advance <= room {
		run.Paint(p, at, c)
		return
	}
	rs := []rune(s)
	for n := len(rs) - 1; n > 0; n-- {
		if cut := audioui.Shaped(string(rs[:n])+"…", 12, false, false); cut.Advance <= room {
			cut.Paint(p, at, c)
			return
		}
	}
}

// Step implements [gunim.Animator]: the meter, in full.
func (v *panel) Step(dt time.Duration) bool {
	if !v.full {
		return false
	}
	return v.out.step(dt)
}

// Follow implements [plug.Follower]: the knobs and the selector glide
// to their parameters set elsewhere, unless held.
func (v *panel) Follow(id uint32, val float64, u *gunim.UI) {
	for _, s := range v.sections {
		s.Follow(id, val, u)
	}
	if id == SoundID {
		v.sound = soundAt(val)
		v.pick.SetSelected(v.sound, u)
		v.dim()
	}
	u.Invalidate()
}
