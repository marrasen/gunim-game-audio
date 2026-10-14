package plugsynth

import (
	"image/color"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// View implements [plug.Viewer]: the sound's name and the level in a
// chip; the sound, steps to the sounds beside it, and the knobs in a
// panel; and in full, what the sound is too, and a meter of what comes
// out. The host's browser picks the sounds, from Presets.
func (p *plugin) View(s plug.Size, h plug.Host) gunim.Node {
	switch s {
	case plug.Chip:
		return newChip(p, h)
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

// chip shows the sound, in a plugin's chip in a chain, and, beside it,
// the level that comes out, left over right. The name turns over as the
// sound changes.
type chip struct {
	anim.Group
	p        *plugin
	name     string
	was      string
	turn     *anim.Float
	out      *meter
	measured geom.Size
}

func newChip(p *plugin, h plug.Host) *chip {
	_, out := h.Sound()
	c := &chip{p: p, name: p.sound().Name, turn: anim.NewFloat(1), out: newMeter(out)}
	c.Add(c.turn)
	return c
}

// chipBars is how wide the chip's bars are, and chipText how big its
// words.
const (
	chipBars = 34
	chipText = 12
)

// Layout implements [gunim.Node]: as big as it is given, at least the
// height of its words.
func (c *chip) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	c.measured = cs.Constrain(geom.Sz(cs.Max.W, chipText*1.6))
	return c.measured
}

// Paint implements [gunim.Node]: the sound's name, centred on the
// height and fading where it is cut, and each channel's RMS over its
// peak, fainter.
func (c *chip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink, ground, over := widget.Ink.Get(f.Theme), audioui.Ground.Get(f.Theme), audioui.Over.Get(f.Theme)
	room := box.W - chipBars - 8
	t := c.turn.Value()
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, 0, room+2, box.H), Clip: true, Opacity: 1})()
		if t < 1 && c.was != "" {
			run := audioui.Shaped(c.was, chipText, false, false)
			run.PaintFaded(p, geom.Pt(0, (box.H-run.Height())/2-box.H*0.6*t), audioui.Faded(ink, 0.9*(1-t)), room, 18)
		}
		run := audioui.Shaped(c.name, chipText, false, false)
		run.PaintFaded(p, geom.Pt(0, (box.H-run.Height())/2+box.H*0.6*(1-t)), audioui.Faded(ink, 0.9*t), room, 18)
	}()
	x, h := box.W-chipBars, float32(3)
	top := (box.H - 2*h - 2) / 2
	lv := c.out.levels
	for ch := range 2 {
		y := top + float32(ch)*(h+2)
		p.RRect(geom.Rc(x, y, chipBars, h), h/2, paint.Solid(audioui.Faded(ground, 0.5)))
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

// Step implements [gunim.Animator]: the name turning, and the bars.
func (c *chip) Step(dt time.Duration) bool {
	moving := c.Group.Step(dt)
	return c.out.step(dt) || moving
}

// Access implements [gunim.Accessible]: the chip reads as the sound.
func (c *chip) Access() access.Info { return access.Info{Role: access.RoleLabel, Name: c.name} }

// FollowState implements [plug.StateFollower]: the chip names the sound
// playing, the old name rising away as the new one comes up.
func (c *chip) FollowState(u *gunim.UI) {
	name := c.p.sound().Name
	if name == c.name {
		return
	}
	c.was, c.name = c.name, name
	c.turn.Jump(0)
	c.turn.Animate(1, widget.Settle.Get(u.Theme()))
	u.Invalidate()
}

// panel is the sound's name, where it comes from, and steps to the
// sounds beside it in its group, over the knobs' sections. In full,
// what the sound is shows under its name, and a meter of what comes out
// stands at the right.
type panel struct {
	anim.Group
	p        *plugin
	h        plug.Host
	full     bool
	prev     *widget.IconButton
	next     *widget.IconButton
	sections []*plugui.Section
	// keys play notes of the sound, where the host lets them, and
	// shown says they have room.
	keys  *keys
	shown bool
	// aboutShown says what the sound is shows under its name: in full,
	// where the keys have room with it, or lack it without it too.
	aboutShown bool
	out        *meter
	// turn runs from 0 to 1 as the name turns over to a new sound, and
	// was is the name it turns from.
	turn *anim.Float
	was  string
	st   State
	// pages are the tabs, and the Patch and Controls pages.
	pages *pages
}

func newPanel(p *plugin, h plug.Host, full bool) *panel {
	v := &panel{p: p, h: h, full: full, st: p.sound(), turn: anim.NewFloat(1)}
	v.Add(v.turn)
	v.prev = widget.NewIconButton(icon.ChevronLeft, "The sound before")
	v.next = widget.NewIconButton(icon.ChevronRight, "The next sound")
	v.prev.OnClick = func(u *gunim.UI) gunim.Intent { v.step(-1, u); return nil }
	v.next.OnClick = func(u *gunim.UI) gunim.Intent { v.step(1, u); return nil }
	v.sections = []*plugui.Section{
		plugui.NewSection(h, &Descriptor, "FILTER", 0, CutoffID, ResID),
		plugui.NewSection(h, &Descriptor, "ENVELOPE", 0, AttackID, ReleaseID),
		plugui.NewSection(h, &Descriptor, "OUTPUT", 0, GainID),
		plugui.NewSection(h, &Descriptor, "AHEAD", 0, SwellID, GhostID),
	}
	if a, ok := h.(plug.Auditioner); ok {
		v.keys = newKeys(a)
		v.keys.follow(v.st.Patch.Kind)
	}
	_, out := h.Sound()
	v.out = newMeter(out)
	v.pages = newPages(v, h)
	v.dim()
	return v
}

// dim greys the knobs the sound has nothing for: the filter's, on a
// sound with none, and the envelope's and the swell, on drums.
func (v *panel) dim() {
	pt := v.st.Patch
	drums := pt.Kind == "drums"
	filter := pt.Filter.Type != "" && pt.Filter.Type != "none" && !drums
	for _, sec := range v.sections {
		for _, k := range sec.Knobs {
			switch k.Param().ID {
			case CutoffID, ResID:
				k.Disabled = !filter
			case AttackID, ReleaseID, SwellID:
				k.Disabled = drums
			}
		}
	}
}

// The panel's sizes: how far in from its edges it draws, and the
// header's.
const (
	panelInset = 16
	stepW      = 28
	nameY      = panelInset
	crumbY     = nameY + 30
	aboutY     = crumbY + 22
	meterTop   = 36
	// keysMin and keysMax bound how tall the keys stand.
	keysMin = 40
	keysMax = 110
)

// factsY is where the facts stand: under what the sound is, where that
// shows, or else under where it comes from.
func (v *panel) factsY(about bool) float32 {
	if about {
		return aboutY + 44
	}
	return crumbY + 26
}

// keysTop is where the keys stand, under the facts.
func (v *panel) keysTop(about bool) float32 { return v.factsY(about) + 34 }

// Children implements [gunim.Composite].
func (v *panel) Children() []gunim.Node {
	kids := []gunim.Node{v.prev, v.next}
	for _, s := range v.sections {
		kids = append(kids, s)
	}
	if v.keys != nil {
		kids = append(kids, v.keys)
	}
	return append(kids, v.pages.nodes()...)
}

// detail is the part left of the meter, in full, or the whole panel.
func (v *panel) detail(size geom.Size) geom.Rect {
	r := geom.Rect{Max: size.Point()}
	if v.full {
		r.Max.X -= audioui.MeterW + panelInset
	}
	return r
}

// Layout implements [gunim.Node]: the steps beside the name, every
// section along the foot, in rows as many as fit, each row centred;
// the keys between, as tall as the room lets. Where room is short, the
// keys grow shorter, then what the sound is gives way to them, then they
// give way to the sections.
func (v *panel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	if v.page() != SoundPage {
		return v.layoutPage(size, kids)
	}
	d := v.detail(size)
	right := d.Max.X - panelInset
	kids.At(0).Layout(gunim.Tight(geom.Sz(stepW, stepW)))
	kids.At(0).Place(geom.Pt(right-2*stepW-4, nameY-2))
	kids.At(1).Layout(gunim.Tight(geom.Sz(stepW, stepW)))
	kids.At(1).Place(geom.Pt(right-stepW, nameY-2))
	secTop := plugui.PlaceSections(d.Size(), panelInset, v.sections, func(i int, r geom.Rect) {
		kids.At(2 + i).Layout(gunim.Tight(r.Size()))
		kids.At(2 + i).Place(r.Min.Add(d.Min))
	})
	keysRoom := func(about bool) float32 { return secTop - 14 - v.keysTop(about) }
	v.aboutShown = v.full && (v.keys == nil || keysRoom(true) >= keysMin || keysRoom(false) < keysMin)
	next := 2 + len(v.sections)
	if v.keys != nil {
		top := v.keysTop(v.aboutShown)
		h := min(keysRoom(v.aboutShown), keysMax)
		w := min(d.Size().W-2*panelInset, 15*28+2*(shiftW+4))
		v.shown = h >= keysMin
		if !v.shown {
			h, w = 0, 0
		}
		kids.At(next).Layout(gunim.Tight(geom.Sz(w, h)))
		kids.At(next).Place(geom.Pt(d.Min.X+(d.Size().W-w)/2, top))
	}
	v.layoutPages(kids, right, geom.Rect{})
	return size
}

// Paint implements [gunim.Node]: the sound's name, turning over as it
// changes, where it comes from, and in full what it is and the meter;
// then the sections and the steps.
func (v *panel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	d := v.detail(box)
	x := d.Min.X + panelInset
	room := d.Max.X - panelInset - x - 2*stepW - 12
	if v.full {
		bottom := max(box.H-panelInset, 80)
		mx := box.W - audioui.MeterW - panelInset
		audioui.PaintMeter(pt, f.Theme, geom.Rect{Min: geom.Pt(mx, meterTop), Max: geom.Pt(mx+audioui.MeterW, bottom)}, v.out.levels)
	}
	// The name turns over: the old one rises and fades as the new one
	// comes up from below.
	t := v.turn.Value()
	func() {
		defer pt.Layer(paint.LayerOpts{Bounds: geom.Rc(x, nameY-4, room+4, 32), Clip: true, Opacity: 1})()
		if t < 1 && v.was != "" {
			audioui.Shaped(v.was, 20, true, false).PaintFaded(pt, geom.Pt(x, nameY-14*t), audioui.Faded(ink, 1-t), room, 24)
		}
		audioui.Shaped(v.st.Name, 20, true, false).PaintFaded(pt, geom.Pt(x, nameY+14*(1-t)), audioui.Faded(ink, t), room, 24)
	}()
	crumb := v.st.Bank
	if v.st.Group != "" {
		crumb += "  ›  " + v.st.Group
	}
	if v.page() == ControlsPage {
		crumb = "These turn the patch as it plays: right-click one to automate or modulate it"
	}
	wide := d.Max.X - panelInset - x - tabsW - 12
	bankIcon(pt, f, v.st.Bank, geom.Rc(x, crumbY+1, 12, 12), audioui.Faded(accent, 0.9))
	audioui.Shaped(crumb, 11, false, false).PaintFaded(pt, geom.Pt(x+18, crumbY), audioui.Faded(ink, 0.6), wide-18, 20)
	if v.page() != SoundPage {
		n := kids.Len()
		for i := range n {
			if v.shows(i, n) {
				kids.At(i).Paint(pt)
			}
		}
		return
	}
	if v.aboutShown {
		paintWrapped(pt, v.about(), geom.Pt(x, aboutY), d.Max.X-panelInset-x, 2, audioui.Faded(ink, 0.55))
	}
	paintFacts(pt, f, facts(v.st.Patch), geom.Pt(x, v.factsY(v.aboutShown)), d.Max.X-panelInset-x)
	n := kids.Len()
	for i := range n {
		if v.shows(i, n) {
			kids.At(i).Paint(pt)
		}
	}
}

// about is what the sound is, from the sounds' list, or the state's
// own words for a sound the list lacks.
func (v *panel) about() string {
	if s, ok := Find(v.st.Bank, v.st.Group, v.st.Name); ok {
		return s.About
	}
	return "A sound of its own, kept with the song."
}

// paintWrapped draws s from the point at, in 12 points, wrapped at room, in at most
// lines lines, the last fading out where it is cut.
func paintWrapped(p *paint.Painter, s string, at geom.Point, room float32, lines int, c color.NRGBA) {
	words := strings.Fields(s)
	y := at.Y
	for l := 0; l < lines && len(words) > 0; l++ {
		n := 1
		for n < len(words) && audioui.Shaped(strings.Join(words[:n+1], " "), 12, false, false).Advance <= room {
			n++
		}
		line := strings.Join(words[:n], " ")
		if l == lines-1 && n < len(words) {
			line = strings.Join(words, " ")
		}
		audioui.Shaped(line, 12, false, false).PaintFaded(p, geom.Pt(at.X, y), c, room, 28)
		words = words[n:]
		y += 17
	}
}

// Step implements [gunim.Animator]: the name turning, and the meter, in
// full.
func (v *panel) Step(dt time.Duration) bool {
	moving := v.Group.Step(dt)
	if v.full {
		moving = v.out.step(dt) || moving
	}
	return moving
}

// Follow implements [plug.Follower]: the knobs glide to their
// parameters set elsewhere, unless held.
func (v *panel) Follow(id uint32, val float64, u *gunim.UI) {
	for _, s := range append(v.sections, v.pages.sections...) {
		s.Follow(id, val, u)
	}
	u.Invalidate()
}

// FollowState implements [plug.StateFollower]: the panel shows the
// sound playing, its name turning over.
func (v *panel) FollowState(u *gunim.UI) {
	st := v.p.sound()
	if st.Name != v.st.Name || st.Group != v.st.Group || st.Bank != v.st.Bank {
		v.was = v.st.Name
		v.turn.Jump(0)
		v.turn.Animate(1, widget.Settle.Get(u.Theme()))
	}
	v.st = st
	v.dim()
	if v.keys != nil {
		v.keys.follow(st.Patch.Kind)
	}
	v.pages.editor.FollowState(u)
	u.Invalidate()
}

// Focusable implements [gunim.Focusable]: a press on the panel gives
// it the keyboard, for its keys.
func (v *panel) Focusable() bool { return true }

// Handle implements [gunim.Handler]: Page Up and Page Down step
// through the sounds of the sound's group, and Z and X shift the keys
// an octave down and up.
func (v *panel) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	switch {
	case k.Key == input.KeyPageUp:
		v.step(-1, u)
	case k.Key == input.KeyPageDown:
		v.step(1, u)
	case (k.Key == input.KeyZ || k.Key == input.KeyX) && k.Mods == 0 && v.keys != nil && v.shown:
		d := -1
		if k.Key == input.KeyX {
			d = 1
		}
		v.keys.shiftBy(d, u)
	default:
		return false
	}
	return true
}

// bankIcon draws the icon of bank in r.
func bankIcon(p *paint.Painter, f gunim.Frame, bank string, r geom.Rect, c color.NRGBA) {
	ic := icon.Library
	switch bank {
	case GeneralMIDI:
		ic = icon.Piano
	case Songs:
		ic = icon.ListMusic
	}
	widget.PaintIcon(p, f.Theme, ic, r, c)
}
