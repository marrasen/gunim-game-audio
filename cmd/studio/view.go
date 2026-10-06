package main

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The studio's own tokens.
var (
	smallSize = theme.Length("studio.small", 12)
	problem   = theme.Color("studio.problem", color.NRGBA{0xff, 0x6b, 0x5f, 0xff})
	panelFill = theme.Color("studio.panel", color.NRGBA{0x17, 0x1a, 0x24, 0xff})
	cardFill  = theme.Color("studio.card", color.NRGBA{0x1e, 0x22, 0x2e, 0xff})
	playing   = theme.Color("studio.playing", color.NRGBA{0x25, 0x2b, 0x3c, 0xff})
)

// view is the studio's window, with handles on what updates change.
type view struct {
	*widget.Pad
	songs     *widget.Dropdown
	about     *widget.Label
	play      *widget.Button
	tiers     *widget.Segmented
	tierLabel *widget.Label
	stings    *widget.Dropdown
	sting     *widget.Button
	bpm       *widget.NumberField
	key       *widget.Label
	volume    *widget.Slider
	stage     *stage
	flow      *flow
	tracks    *widget.List
	follow    *widget.Button
	chords    *widget.TextField
	chordErr  *widget.Label
	keypad    *widget.Label
	status    *widget.Label
	spectrum  *spectrum
	// gen is the studio's Gen the fields were last set for, and shown
	// the number of tiers the segmented control shows.
	gen, shown int
	stingNames []string
	s          Studio
}

// root is the view's root: it catches the keys, for the keypad, the
// tiers and the transport.
type root struct{ *view }

func small(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Color = smallSize, widget.MenuHint
	return l
}

func buildView(s Studio) *root {
	v := &view{gen: -1, shown: -1}
	title := widget.NewLabel("gunim music studio")
	title.Size = widget.HeadingSize
	v.songs = widget.NewDropdown(s.Songs...)
	v.songs.Label = "Song"
	v.songs.OnChange = func(i int) gunim.Intent { return SongChosen{Song: i} }
	v.play = widget.NewButton("Pause")
	v.play.Icon, v.play.Kind, v.play.On = icon.Pause, widget.ButtonPrimary, PlayToggled{}
	v.play.KeepFocus = true
	restart := widget.NewIconButton(icon.RotateCcw, "Play from the start")
	restart.On = Restarted{}
	v.tierLabel = small("Tier")
	v.tiers = widget.NewSegmented("1")
	v.tiers.KeepFocus = true
	v.tiers.OnChange = func(i int) gunim.Intent { return TierChosen{Tier: i + 1} }
	v.stings = widget.NewDropdown("no stings")
	v.stings.Label, v.stings.Disabled = "Sting", true
	v.sting = widget.NewButton("Sting")
	v.sting.Icon = icon.Trophy
	v.bpm = widget.NewNumberField(40, 240)
	v.bpm.Suffix = " BPM"
	v.bpm.OnChange = func(x float64) gunim.Intent { return BPMSet{BPM: x} }
	down := widget.NewIconButton(icon.Minus, "Down a semitone")
	down.On = Transposed{By: -1}
	up := widget.NewIconButton(icon.Plus, "Up a semitone")
	up.On = Transposed{By: 1}
	v.key = widget.NewLabel("")
	v.volume = widget.NewSlider(0, 1)
	v.volume.Set(s.Volume)
	v.volume.OnChange = func(x float32) gunim.Intent { return VolumeSet{Volume: x} }
	save := widget.NewIconButton(icon.Save, "Save the song as a song.json")
	save.On = Saved{}
	gap := widget.NewSpacer()
	top := widget.Row(title, v.songs, v.play, restart, v.tierLabel, v.tiers, v.stings, v.sting, gap,
		widget.NewSized(v.bpm.TextField, 104, 0), down, v.key, up, widget.NewIcon(icon.Volume2, "Volume"),
		widget.NewSized(v.volume, 110, 0), save).Grow(gap, 1)
	top.Cross = widget.CrossCenter
	v.about = small("")

	v.stage = newStage()
	v.flow = newFlow()
	left := widget.Column(v.stage, widget.NewSized(v.flow, 0, 250)).Grow(v.stage, 1)
	left.Cross = widget.CrossStretch

	head := widget.NewLabel("Tracks")
	head.Size = widget.HeadingSize
	v.follow = widget.NewButton("Follow the tier")
	v.follow.Icon, v.follow.On = icon.Layers, TierFollowed{}
	hint := small("Type a pattern and hear it from the next bar. Click a card to start or stop its track.")
	headGap := widget.NewSpacer()
	headRow := widget.Row(head, headGap, v.follow).Grow(headGap, 1)
	headRow.Cross = widget.CrossCenter
	v.tracks = widget.NewList()
	v.tracks.NoFocus = true
	v.tracks.OnClick = func(k widget.Key) gunim.Intent { return PartClicked{Name: string(k)} }
	list := widget.NewScroll(v.tracks)
	right := widget.Column(headRow, hint, list).Grow(list, 1)
	right.Cross = widget.CrossStretch
	panel := widget.NewCard(right)
	panel.Fill = panelFill

	middle := widget.Row(left, widget.NewSized(panel, 480, 0)).Grow(left, 1)
	middle.Cross = widget.CrossStretch

	chordsLabel := widget.NewLabel("Chords")
	v.chords = widget.NewTextField()
	v.chords.Placeholder = "Am F C G, or vi IV I V"
	v.chords.Face = widget.MonoFont
	v.chords.OnChange = func(t string) gunim.Intent { return ChordsEdited{Text: t} }
	v.chordErr = small("")
	v.chordErr.Color = problem
	genButtons := make([]gunim.Node, 0, len(styles))
	for _, st := range styles {
		b := widget.NewButton(st)
		b.Icon, b.On, b.Ghost, b.KeepFocus = icon.WandSparkles, ChordsGenerated{Style: st}, true, true
		b.Tooltip = "Write a " + st + " progression"
		genButtons = append(genButtons, b)
	}
	v.keypad = small("")
	v.spectrum = newSpectrum()
	chordCol := widget.Column(widget.Row(chordsLabel, v.chords).Grow(v.chords, 1), v.chordErr)
	chordCol.Cross = widget.CrossStretch
	genRow := widget.Row(append([]gunim.Node{small("Write:")}, genButtons...)...)
	genRow.Cross = widget.CrossCenter
	bottomLeft := widget.Column(chordCol, genRow, v.keypad)
	bottomLeft.Cross = widget.CrossStretch
	bottom := widget.Row(bottomLeft, widget.NewSized(v.spectrum, 380, 96)).Grow(bottomLeft, 1)
	bottom.Cross = widget.CrossStretch
	v.status = small("")

	col := widget.Column(top, v.about, middle, bottom, v.status).Grow(middle, 1)
	col.Cross = widget.CrossStretch
	v.Pad = widget.NewPad(col)
	return &root{v}
}

func (r *root) update(s Studio, u *gunim.UI) { r.view.update(s, u) }

// update shows s.
func (v *view) update(s Studio, u *gunim.UI) {
	v.s = s
	v.songs.Selected = s.Song
	v.about.SetText(s.About)
	if s.Playing {
		v.play.Label, v.play.Icon = "Pause", icon.Pause
	} else {
		v.play.Label, v.play.Icon = "Play", icon.Play
	}
	if s.Tiers > 0 {
		if v.shown != s.Tiers {
			labels := make([]string, s.Tiers)
			for i := range labels {
				labels[i] = strconv.Itoa(i + 1)
			}
			v.tiers.Labels = labels
			v.shown = s.Tiers
		}
		v.tierLabel.SetText("Tier")
		if v.tiers.Selected() != s.Tier-1 {
			v.tiers.SetSelected(s.Tier-1, u)
		}
	} else if v.shown != 0 {
		v.tiers.Labels = []string{"wanders"}
		v.tierLabel.SetText("Parts")
		v.shown = 0
	}
	if fmt.Sprint(s.Stings) != fmt.Sprint(v.stingNames) {
		v.stingNames = s.Stings
		if len(s.Stings) == 0 {
			v.stings.Items, v.stings.Disabled = []string{"no stings"}, true
		} else {
			v.stings.Items, v.stings.Disabled = s.Stings, false
		}
		v.stings.Selected = 0
	}
	v.sting.Disabled = len(s.Stings) == 0
	if len(s.Stings) > 0 {
		v.sting.On = StingPlayed{Name: s.Stings[min(v.stings.Selected, len(s.Stings)-1)]}
	}
	v.key.SetText(s.Key)
	v.follow.Disabled = false
	if v.gen != s.Gen {
		v.gen = s.Gen
		v.chords.SetText(s.ChordText)
		v.chords.Select(0, 0)
		v.bpm.SetText(strconv.FormatFloat(s.BPM, 'f', -1, 64))
	}
	v.chordErr.SetText(s.ChordErr)
	if s.Keypad {
		v.keypad.SetText("Keypad: the digit keys play the key's pentatonic over the song, so typing always fits. F1 to F4 set the tier; Space plays and pauses.")
	} else {
		v.keypad.SetText("F1 to F4 set the tier; Space plays and pauses.")
	}
	v.status.SetText(s.Status)
	v.stage.s = s
	v.flow.s = s
	v.spectrum.target = s.Spectrum
	widget.Sync(v.tracks, u, s.Tracks, func(t TrackRow) widget.Key { return widget.Key(t.Name) }, newCard, (*card).set)
	u.Invalidate()
}

// CatchKey plays the keypad for the digits, sets the tier for F1 to F4,
// and plays or pauses for Space, where nothing focused, such as a field
// being typed in, took the key.
func (r *root) CatchKey(e input.Event, u *gunim.UI) bool {
	p, ok := e.(input.KeyPress)
	if !ok || p.Mods != 0 {
		return false
	}
	switch {
	case p.Char >= '0' && p.Char <= '9' && r.s.Keypad:
		u.Send(r, KeyPlayed{Digit: int(p.Char - '0')})
		return true
	case p.Key >= input.KeyF1 && p.Key <= input.KeyF1+3 && r.s.Tiers > 0:
		u.Send(r, TierChosen{Tier: min(int(p.Key-input.KeyF1)+1, r.s.Tiers)})
		return true
	case p.Key == input.KeySpace && !p.Repeat:
		u.Send(r, PlayToggled{})
		return true
	}
	return false
}

// card is a track's card: its name and what it plays, its level, its
// pattern to edit, and its sliders.
type card struct {
	*widget.Card
	name, doing, err            *widget.Label
	swatch                      *swatch
	meter                       *meter
	field                       *widget.TextField
	gain, filter, reverb, delay *widget.Slider
	gen                         int
	track                       string
}

func newCard(t TrackRow) *card {
	c := &card{name: widget.NewLabel(""), doing: small(""), err: small(""), swatch: &swatch{}, meter: &meter{}, gen: -1, track: t.Name}
	c.err.Color = problem
	c.field = widget.NewTextField()
	c.field.Face = widget.MonoFont
	c.field.Placeholder = "a pattern, as bd ~ sn ~ or x*8"
	name := t.Name
	c.field.OnChange = func(s string) gunim.Intent { return PatternEdited{Track: name, Text: s} }
	knob := func(lo, hi float32, k string) *widget.Slider {
		s := widget.NewSlider(lo, hi)
		s.OnChange = func(x float32) gunim.Intent { return KnobSet{Track: name, Knob: k, Value: x} }
		return s
	}
	c.gain, c.filter, c.reverb, c.delay = knob(-36, 6, "gain"), knob(0, 1, "filter"), knob(0, 1, "reverb"), knob(0, 1, "delay")
	head := widget.Row(widget.NewSized(c.swatch, 12, 12), c.name, c.doing, widget.NewSpacer(), widget.NewSized(c.meter, 90, 8))
	head.Cross = widget.CrossCenter
	head.Grow(head.Children()[3], 1)
	pair := func(label string, s *widget.Slider) gunim.Node {
		col := widget.Column(small(label), s)
		col.Cross = widget.CrossStretch
		return col
	}
	knobs := widget.Row(pair("Level", c.gain), pair("Filter", c.filter), pair("Reverb", c.reverb), pair("Delay", c.delay))
	for _, k := range knobs.Children() {
		knobs.Grow(k, 1)
	}
	col := widget.Column(head, c.field, c.err, knobs)
	col.Cross = widget.CrossStretch
	c.Card = widget.NewCard(col)
	c.set(t, nil)
	return c
}

func (c *card) set(t TrackRow, _ *gunim.UI) {
	label := t.Name
	switch {
	case t.Tier > 0:
		label = fmt.Sprintf("%s · tier %d", t.Name, t.Tier)
	case t.Core:
		label = t.Name + " · always"
	}
	c.name.SetText(label)
	c.doing.SetText(t.Doing)
	c.doing.Color = widget.MenuHint
	c.Fill = cardFill
	if t.Playing {
		c.doing.Color = widget.Accent
		c.Fill = playing
	}
	c.err.SetText(t.PatternErr)
	c.swatch.c = hexColor(t.Color, 0)
	c.swatch.on = t.Playing
	c.meter.c = hexColor(t.Color, 0)
	c.meter.target = t.Level
	if c.gen != t.Gen {
		c.gen = t.Gen
		c.field.SetText(t.Pattern)
		c.field.Select(0, 0)
		c.gain.Set(t.Gain)
		c.filter.Set(t.Filter)
		c.reverb.Set(t.Reverb)
		c.delay.Set(t.Delay)
	}
}

// swatch is a track's colour, a dot, bright while it plays.
type swatch struct {
	c  color.NRGBA
	on bool
}

func (s *swatch) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

func (s *swatch) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	c := s.c
	if !s.on {
		c = withAlpha(grey(c, 0.6), 0.6)
	}
	d := min(box.W, box.H)
	r := geom.Rc((box.W-d)/2, (box.H-d)/2, d, d)
	if s.on {
		p.ShadowRRect(r, d/2, paint.Solid(c), paint.Shadow{Blur: 6, Color: withAlpha(s.c, 0.8)})
		return
	}
	p.RRect(r, d/2, paint.Solid(c))
}

// meter is a track's level, a bar that rises at once and falls slowly.
type meter struct {
	c      color.NRGBA
	target float32
	shown  float32
}

func (m *meter) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

func (m *meter) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, box.H/2, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x12}))
	w := box.W * min(m.shown*2.2, 1)
	if w > 1 {
		p.RRect(geom.Rc(0, 0, w, box.H), box.H/2, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, 0), To: geom.Pt(box.W, 0),
			Start: withAlpha(m.c, 0.6), End: lighten(m.c, 0.5)}})
	}
}

func (m *meter) Step(dt time.Duration) bool {
	t := float32(dt.Seconds())
	if m.target > m.shown {
		m.shown += (m.target - m.shown) * min(1, t*20)
	} else {
		m.shown += (m.target - m.shown) * min(1, t*4)
	}
	return m.shown > 1e-3 || m.target > 0
}

// spectrum is the sound's spectrum, as heard.
type spectrum struct {
	sp     *audioui.Spectrum
	target []float32
}

func newSpectrum() *spectrum { return &spectrum{sp: audioui.NewSpectrum(spectrumPoints)} }

func (s *spectrum) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (s *spectrum) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, 12, paint.Solid(audioui.Ground.Get(f.Theme)))
	s.sp.Paint(p, f.Theme, geom.Rc(8, 6, box.W-16, box.H-12))
}

func (s *spectrum) Step(dt time.Duration) bool {
	if len(s.target) != spectrumPoints {
		return false
	}
	s.sp.Step(dt, s.target, nil)
	return true
}
