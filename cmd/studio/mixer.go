package main

import (
	"fmt"
	"github.com/marrasen/gunim/icon"
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/synth"
)

// maxStrips is how many tracks the mixer shows.
const maxStrips = 16

// mixerPane is the mixing desk: a strip for each track, with its mute
// and solo, its knobs, a meter of its level after its fader, and its
// fader; and the master's strip, with the mix's meter, its fader, its
// compressor and how far that turns the mix down, and its loudness.
type mixerPane struct {
	*widget.Pad
	strips [maxStrips]*channel
	row    *strip
	master *masterStrip
}

func newMixerPane() *mixerPane {
	m := &mixerPane{}
	kids := make([]gunim.Node, maxStrips)
	for i := range m.strips {
		m.strips[i] = newChannel()
		kids[i] = m.strips[i]
	}
	m.row = &strip{kids: kids, maxW: 112, gap: 4}
	m.master = newMasterStrip()
	desk := widget.Row(m.row, sized(m.master, 150, 0)).Grow(m.row, 1)
	desk.Cross = widget.CrossStretch
	m.Pad = widget.NewPad(desk)
	return m
}

func (m *mixerPane) update(st Studio, u *gunim.UI) {
	if st.Doc == nil {
		return
	}
	m.row.n = min(len(st.Tracks), maxStrips)
	for i := range m.row.n {
		m.strips[i].update(st, st.Tracks[i], i, u)
	}
	m.master.update(st)
}

// channel is a track's strip on the desk.
type channel struct {
	*widget.Card
	base    string
	name    *widget.Label
	gainDB  *widget.Label
	mute    *widget.Button
	solo    *widget.Button
	ks      []*knob
	chorus  *kindUnit
	distort *kindUnit
	meter   *vu
	fader   *audioui.Fader
	swatch  *swatch
	gain    float32
}

func newChannel() *channel {
	c := &channel{name: widget.NewLabel(""), gainDB: small(""), meter: &vu{}, swatch: &swatch{}}
	c.name.Size = smallSize
	c.name.MaxLines = 1
	b := &c.base
	c.ks = []*knob{
		newKnob("Pan", b, "/Pan", -1, 1, 0).center().small(),
		newKnob("Reverb", b, "/Reverb", 0, 1, 0).small(),
		newKnob("Delay", b, "/Delay", 0, 1, 0).small(),
		newKnob("Duck", b, "/Duck", 0, 1, 0).small(),
		newKnob("Low cut", b, "/HPF", 20, 2000, 20).logScale().small().unsetIs(20),
		newKnob("High cut", b, "/LPF", 200, 20000, 20000).logScale().small().unsetIs(20000),
		newKnob("Drive", b, "/Shape", 0, 0.95, 0).small(),
		newKnob("Gated", b, "/Gated", 0, 1, 0).small(),
		newKnob("Ring", b, "/Ring", 0, 1, 0).small(),
		newKnob("Smash", b, "/Smash", 0, 1, 0).small(),
	}
	c.chorus = newChorusUnit(b)
	c.distort = newDistortUnit(b)
	c.mute = widget.NewButton("M")
	c.mute.KeepFocus, c.mute.Tooltip = true, "Mute"
	c.solo = widget.NewButton("S")
	c.solo.KeepFocus, c.solo.Tooltip = true, "Solo"
	c.fader = audioui.NewFader(func() float32 { return c.gain }, func(v float32, _ *gunim.UI) gunim.Intent {
		c.gain = v
		return SetValue{Path: c.base + "/Gain", Num: float64(v)}
	})
	c.fader.Range = 36
	rows := make([]gunim.Node, 0, len(c.ks)/2)
	for i := 0; i+1 < len(c.ks); i += 2 {
		rows = append(rows, widget.Row(c.ks[i], c.ks[i+1]))
	}
	rows = append(rows, c.chorus, c.distort)
	kcol := widget.Column(rows...)
	kcol.Cross = widget.CrossCenter
	buttons := widget.Row(c.mute, c.solo)
	head := widget.Row(sized(c.swatch, 8, 8), c.name)
	head.Cross = widget.CrossCenter
	faders := widget.Row(sized(c.meter, 12, 0), c.fader).Grow(c.fader, 1)
	faders.Cross = widget.CrossStretch
	col := widget.Column(head, buttons, kcol, faders, c.gainDB).Grow(faders, 1)
	col.Cross = widget.CrossCenter
	c.Card = widget.NewCard(col)
	c.Fill = cardFill
	return c
}

func (c *channel) update(st Studio, t TrackRow, i int, u *gunim.UI) {
	c.base = "Tracks/" + t.Name
	c.name.Text = t.Name
	c.swatch.c, c.swatch.on = hexColor(t.Color, i), t.Playing
	c.gain = t.Gain
	c.gainDB.Text = fmt.Sprintf("%+.1f dB", t.Gain)
	c.mute.Active, c.solo.Active = t.Mute, t.Solo
	c.mute.OnClick = widget.Sends(ToggleValue{Path: c.base + "/Mute"})
	c.solo.OnClick = widget.Sends(ToggleValue{Path: c.base + "/Solo"})
	c.meter.m = t.Meter
	showKnobs(st.Doc, c.ks...)
	col := hexColor(t.Color, i)
	for _, k := range c.ks {
		k.color = col
	}
	showKnobs(st.Doc, c.chorus.k, c.distort.k)
	c.chorus.k.color, c.distort.k.color = col, col
	if tr := track(st.Doc, t.Name); tr != nil {
		c.chorus.show(t.Name, tr.Chorus, tr.ChorusType, col)
		c.distort.show(t.Name, tr.Distort, tr.DistortType, col)
	}
	_ = u
}

func boolNum(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// masterStrip is the desk's master: the mix's meter and fader, its
// compressor, and its loudness.
type masterStrip struct {
	*widget.Card
	base   string
	meter  *vu
	fader  *audioui.Fader
	gain   float32
	gr     *reduction
	lufs   *widget.Label
	gainDB *widget.Label
	ks     []*knob
}

func newMasterStrip() *masterStrip {
	m := &masterStrip{base: "Mix", meter: &vu{}, gr: &reduction{}, lufs: small(""), gainDB: small("")}
	b := &m.base
	m.ks = []*knob{
		newKnob("Threshold", b, "/Threshold", -30, 0, -10).units("dB").unsetIs(-10).small(),
		newKnob("Ratio", b, "/Ratio", 1, 10, 2).unsetIs(2).small(),
	}
	m.fader = audioui.NewFader(func() float32 { return m.gain }, func(v float32, _ *gunim.UI) gunim.Intent {
		m.gain = v
		return SetValue{Path: "Mix/Gain", Num: float64(v)}
	})
	m.fader.Range = 12
	title := widget.NewLabel("MASTER")
	title.Size = smallSize
	faders := widget.Row(sized(m.meter, 22, 0), m.fader, sized(m.gr, 10, 0)).Grow(m.fader, 1)
	faders.Cross = widget.CrossStretch
	comp := widget.Row(m.ks[0], m.ks[1])
	col := widget.Column(title, small("compressor"), comp, faders, m.gainDB, m.lufs).Grow(faders, 1)
	col.Cross = widget.CrossCenter
	m.Card = widget.NewCard(col)
	m.Fill = panelFill
	return m
}

func (m *masterStrip) update(st Studio) {
	m.gain = float32(st.Doc.Mix.Gain)
	m.gainDB.Text = fmt.Sprintf("%+.1f dB", st.Doc.Mix.Gain)
	m.meter.m = st.Master
	m.gr.db = st.Reduction
	if st.LUFS > -70 {
		m.lufs.Text = fmt.Sprintf("%.1f LUFS", st.LUFS)
	} else {
		m.lufs.Text = "– LUFS"
	}
	showKnobs(st.Doc, m.ks...)
}

// reduction shows how far a compressor turns the sound down, a bar down
// from the top, 12 dB at its foot.
type reduction struct{ db float32 }

func (r *reduction) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (r *reduction) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rc(0, 0, box.W, box.H), 3, paint.Solid(color.NRGBA{0x08, 0x0a, 0x0e, 0xff}))
	h := float32(math.Min(float64(r.db)/12, 1)) * (box.H - 4)
	if h > 0.5 {
		p.RRect(geom.Rc(2, 2, box.W-4, h), 2, paint.Solid(color.NRGBA{0xff, 0xc8, 0x57, 0xff}))
	}
}

// fxPane edits the song's effects: its reverb, with the shape of its
// tail; its delay, with its echoes; its compressor, with its curve and
// how far it turns the mix down now; and the mix as a whole.
type fxPane struct {
	*widget.Scroll
	base    string
	rev     []*knob
	dly     []*knob
	comp    []*knob
	mix     []*knob
	trans   []*knob
	tail    *tailView
	echoes  *echoView
	curve   *curveView
	duck    *widget.Dropdown
	duckTo  []string
	transOn *switcher
	gated   []*knob
	gateOn  *widget.Button
	tweak   []*knob
	untweak *widget.Button
}

func newFxPane() *fxPane {
	f := &fxPane{base: "Mix"}
	b := &f.base
	f.rev = []*knob{
		newKnob("Size", b, "/Reverb/Size", 0.3, 1.5, 0.8).unsetIs(0.8),
		newKnob("Decay", b, "/Reverb/Decay", 0.2, 8, 2.2).logScale().units("s").unsetIs(2.2),
		newKnob("Tone", b, "/Reverb/Tone", 0, 1, 0.5).unsetIs(0.5),
		newKnob("Pre-delay", b, "/Reverb/PreDelay", 0, 0.2, 0.02).units("s").unsetIs(0.02),
	}
	f.dly = []*knob{
		newKnob("Beats", b, "/Delay/Beats", 0.25, 2, 0.75).steps(0.25).unsetIs(0.75),
		newKnob("Feedback", b, "/Delay/Feedback", 0, 0.95, 0.35).unsetIs(0.35),
		newKnob("Tone", b, "/Delay/Tone", 0, 1, 0.4).unsetIs(0.4),
	}
	f.comp = []*knob{
		newKnob("Threshold", b, "/Threshold", -30, 0, -10).units("dB").unsetIs(-10),
		newKnob("Ratio", b, "/Ratio", 1, 10, 2).unsetIs(2),
	}
	f.mix = []*knob{
		newKnob("Master", b, "/Gain", -12, 12, 0).center().units("dB"),
		newKnob("Swing", b, "/../Swing", 0, 0.5, 0),
		newKnob("Mono bass", b, "/MonoBass", 0, 300, 0).units("Hz"),
		newKnob("Air", b, "/Air", -6, 6, 0).center().units("dB"),
		newKnob("Tape", b, "/Tape", 0, 1, 0),
	}
	// Swing is the song's, not the mix's.
	f.mix[1].base = new(string)
	f.mix[1].rel = "Swing"
	f.trans = []*knob{
		newKnob("Riser bars", b, "/Transitions/Lift", 1, 4, 1).steps(1).unsetIs(1),
		newKnob("Level", b, "/Transitions/Gain", -24, 6, 0).units("dB"),
		newKnob("Reverb", b, "/Transitions/Reverb", 0, 1, 0.3),
	}
	f.gated = []*knob{
		newKnob("Size", b, "/Gated/Size", 0.3, 1.5, 1.2).unsetIs(1.2),
		newKnob("Tone", b, "/Gated/Tone", 0, 1, 0.6).unsetIs(0.6),
		newKnob("Hold", b, "/Gated/Hold", 0.05, 1, 0.3).units("s").unsetIs(0.3),
	}
	f.gateOn = widget.NewButton("On")
	f.gateOn.KeepFocus, f.gateOn.Tooltip = true, "A second room, cut off short by a gate each hit sent to it opens, as the 1980s gated a snare"
	f.tweak = []*knob{
		newKnob("Tone", b, "/Tweak/Tone", -1, 1, 0).center(),
		newKnob("Bass", b, "/Tweak/Bass", -1, 1, 0).center(),
		newKnob("Space", b, "/Tweak/Space", -1, 1, 0).center(),
		newKnob("Width", b, "/Tweak/Width", -1, 1, 0).center(),
		newKnob("Punch", b, "/Tweak/Punch", 0, 1, 0),
		newKnob("Drive", b, "/Tweak/Drive", 0, 1, 0),
		newKnob("Lo-fi", b, "/Tweak/LoFi", 0, 1, 0),
	}
	f.untweak = widget.NewButton("Reset")
	f.untweak.Icon, f.untweak.Tooltip = icon.RotateCcw, "Turn every tweak back to 0, the song as mixed"
	f.untweak.OnClick = widget.Sends(ClearValue{Path: "Mix/Tweak"})
	f.tail = &tailView{}
	f.echoes = &echoView{}
	f.curve = &curveView{}
	f.duck = widget.NewDropdown(widget.Labels("none"))
	f.duck.Label = "Duck to"
	f.duck.OnChange = func(i int, _ *gunim.UI) gunim.Intent {
		to := ""
		if i > 0 && i < len(f.duckTo) {
			to = f.duckTo[i]
		}
		return SetValue{Path: "Mix/Duck", Str: to, IsStr: true}
	}
	rev := panel("REVERB · the room every track sends to", sized(f.tail, 0, 110), knobs(f.rev...))
	dly := panel("DELAY · echoes in time with the song, side to side", sized(f.echoes, 0, 110), knobs(f.dly...))
	comp := panel("COMPRESSOR · glues the mix", sized(f.curve, 0, 110), knobs(f.comp...))
	top := widget.Row(rev, dly, comp).Grow(rev, 1).Grow(dly, 1).Grow(comp, 1)
	top.Cross = widget.CrossStretch
	duckRow := widget.Row(small("Sidechain: the tracks with Duck turn down as"), f.duck, small("hits"))
	duckRow.Cross = widget.CrossCenter
	mix := panel("MIX", knobs(f.mix...), duckRow)
	trans := panel("TRANSITIONS · a riser before the tier climbs, an impact as it lands", knobs(f.trans...))
	f.transOn = newSwitcher(trans, panel("TRANSITIONS", small("This song marks no tier changes.")))
	gated := panel("GATED REVERB · a big room, cut off short after each hit", f.gateOn, knobs(f.gated...))
	low := widget.Row(mix, gated, f.transOn).Grow(mix, 1.4).Grow(gated, 1).Grow(f.transOn, 1)
	low.Cross = widget.CrossStretch
	hint := small("Tone: dark to bright · Bass: cut to boost · Space: dry to wet · Width: mono to wide · Punch: squeezed harder · Drive: pushed into distortion · Lo-fi: an old radio")
	hint.MaxLines = 2
	tweak := panelWith(titled("TWEAK · turn the whole song's sound", f.untweak), widget.Row(knobs(f.tweak...), hint).Grow(hint, 1))
	col := widget.Column(tweak, top, low)
	col.Cross = widget.CrossStretch
	f.Scroll = widget.NewScroll(widget.NewPad(col))
	return f
}

func (f *fxPane) update(st Studio) {
	song := st.Doc
	if song == nil {
		return
	}
	showKnobs(song, f.rev...)
	showKnobs(song, f.dly...)
	showKnobs(song, f.comp...)
	showKnobs(song, f.mix...)
	showKnobs(song, f.trans...)
	showKnobs(song, f.gated...)
	showKnobs(song, f.tweak...)
	f.untweak.Disabled = song.Mix.Tweak == (synth.Tweak{})
	f.gateOn.Active = song.Mix.Gated != nil
	f.gateOn.OnClick = widget.Sends(ToggleValue{Path: "Mix/Gated", Seed: "Mix/Gated/Hold", Num: 0.3})
	f.tail.song, f.echoes.song, f.curve.song = song, song, song
	f.curve.db = st.Reduction
	f.duckTo = []string{"none"}
	for _, t := range song.Tracks {
		f.duckTo = append(f.duckTo, t.Name)
	}
	f.duck.SetItems(widget.Labels(f.duckTo...))
	f.duck.SetSelected(max(segmentedIndex(f.duckTo, song.Mix.Duck), 0), nil)
	f.transOn.which = 1
	if song.Mix.Transitions != nil {
		f.transOn.which = 0
	}
}

// orDefault returns v, or def where v is 0, as the engine reads it.
func orDefault(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// tailView draws a reverb's tail: the pre-delay's gap, the early echoes
// thickening, and the tail dying away over its decay.
type tailView struct{ song *synth.Song }

func (t *tailView) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (t *tailView) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if t.song == nil {
		return
	}
	rv := t.song.Mix.Reverb
	decay := orDefault(rv.Decay, 2.2)
	pre := orDefault(rv.PreDelay, 0.02)
	size := orDefault(rv.Size, 0.8)
	tone := orDefault(rv.Tone, 0.5)
	span := 6.0
	c := mixColor(color.NRGBA{0x5c, 0x8c, 0xff, 0xff}, color.NRGBA{0xa8, 0xff, 0xf0, 0xff}, float32(tone))
	n := int(r.Size().W / 2)
	rnd := newNoise(7)
	for i := range n {
		sec := span * float64(i) / float64(n)
		if sec < pre {
			continue
		}
		// The tail falls 60 dB over decay; early on, echoes come sparse,
		// sparser the bigger the room.
		a := math.Pow(10, -3*(sec-pre)/decay)
		dense := min(1, (sec-pre)/(0.08*size))
		v := float32(a) * (0.35 + 0.65*float32(dense)*rnd())
		h := v * (r.Size().H - 16)
		x := r.Min.X + 4 + (r.Size().W-8)*float32(i)/float32(n)
		p.RRect(geom.Rc(x, r.Max.Y-6-h, 1.6, h), 0, paint.Solid(withAlpha(c, 0.8)))
	}
	label := audioui.Shaped(fmt.Sprintf("%.1f s to fall 60 dB", decay), 10, false, false)
	label.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(c, 0.7))
}

// newNoise returns a source of numbers from 0 to 1, the same each time
// for the same seed, for drawing.
func newNoise(seed uint32) func() float32 {
	x := seed*2654435761 + 1
	return func() float32 {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		return float32(x%1000) / 1000
	}
}

// echoView draws a delay's echoes: each one later by its beats, quieter
// by its feedback, crossing from side to side.
type echoView struct{ song *synth.Song }

func (e *echoView) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (e *echoView) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if e.song == nil {
		return
	}
	d := e.song.Mix.Delay
	beats := orDefault(d.Beats, 0.75)
	fb := orDefault(d.Feedback, 0.35)
	tone := orDefault(d.Tone, 0.4)
	span := 8.0
	mid := r.Min.Y + r.Size().H/2
	for k := 0; ; k++ {
		at := beats * float64(k)
		if at > span {
			break
		}
		a := math.Pow(fb, float64(k))
		if k == 0 {
			a = 1
		}
		if a < 0.01 {
			break
		}
		x := r.Min.X + 6 + (r.Size().W-12)*float32(at/span)
		h := float32(a) * (r.Size().H/2 - 10)
		c := mixColor(scopeColor, color.NRGBA{0x5c, 0x6c, 0xa0, 0xff}, float32(k)*(1-float32(tone))*0.2)
		switch {
		case k == 0:
			p.RRect(geom.Rc(x, mid-h, 3, 2*h), 1, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0xd0}))
		case k%2 == 1:
			p.RRect(geom.Rc(x, mid-h, 3, h), 1, paint.Solid(c))
		default:
			p.RRect(geom.Rc(x, mid, 3, h), 1, paint.Solid(c))
		}
	}
	p.RRect(geom.Rc(r.Min.X+4, mid, r.Size().W-8, 1), 0, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x20}))
	label := audioui.Shaped(fmt.Sprintf("every %g beats · left above, right below", beats), 10, false, false)
	label.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(scopeColor, 0.7))
}

// curveView draws a compressor's curve: the level out for each level in,
// bending at its threshold by its ratio, and a dot where the mix is
// turned down now.
type curveView struct {
	song *synth.Song
	db   float32
}

func (c *curveView) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return cs.Max
}

func (c *curveView) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	screen(p, r)
	if c.song == nil {
		return
	}
	th := orDefault(c.song.Mix.Threshold, -10)
	ratio := orDefault(c.song.Mix.Ratio, 2)
	// From -36 dB to 0, in to out.
	pt := func(in, out float64) geom.Point {
		return geom.Pt(r.Min.X+6+(r.Size().W-12)*float32((in+36)/36), r.Max.Y-6-(r.Size().H-12)*float32((out+36)/36))
	}
	var pts []geom.Point
	for i := range 49 {
		in := -36 + 36*float64(i)/48
		out := in
		if in > th {
			out = th + (in-th)/ratio
		}
		pts = append(pts, pt(in, out))
	}
	audioui.Segment(p, pt(-36, -36), pt(0, 0), 1, color.NRGBA{0xff, 0xff, 0xff, 0x20})
	trace(p, pts, scopeColor)
	// The level now, read back from how far it is turned down.
	if c.db > 0.05 {
		in := th + float64(c.db)*ratio/(ratio-1)
		out := in - float64(c.db)
		q := pt(min(in, 0), out)
		p.ShadowRRect(geom.Rc(q.X-4, q.Y-4, 8, 8), 4, paint.Solid(color.NRGBA{0xff, 0xc8, 0x57, 0xff}),
			paint.Shadow{Blur: 8, Color: color.NRGBA{0xff, 0xc8, 0x57, 0xc0}})
	}
	label := audioui.Shaped(fmt.Sprintf("%.1f dB down now", c.db), 10, false, false)
	label.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+5), withAlpha(scopeColor, 0.7))
}
