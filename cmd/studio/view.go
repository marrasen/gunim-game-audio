package main

import (
	"fmt"
	"image/color"
	"slices"
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

	"github.com/marrasen/gunim-game-audio/internal/songpicker"
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
	songs     *songpicker.Picker
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
	tabs      *widget.Tabs
	side      *side
	mixer     *mixerPane
	patch     *patchPane
	kit       *kitPane
	fx        *fxPane
	pattern   *patternPane
	beat      *beatPane
	// focused is the focus last sent, and editorGen the studio's
	// EditorGen last obeyed.
	focused   Focus
	editorGen int
	// gen is the studio's Gen the fields were last set for, and shown
	// the number of tiers the segmented control shows.
	gen, shown int
	stingNames []string
	s          Studio
	root       *root
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
	v := &view{gen: -1, shown: -1, editorGen: -1}
	r := &root{v}
	v.root = r
	title := widget.NewLabel("gunim music studio")
	title.Size = widget.HeadingSize
	v.songs = songpicker.New(s.Songs, s.Categories, func(i int, _ *gunim.UI) gunim.Intent { return SongChosen{Song: i} })
	v.songs.Label = "Song"
	v.play = widget.NewButton("Pause")
	v.play.Icon, v.play.Kind, v.play.OnClick = icon.Pause, widget.ButtonPrimary, widget.Sends(PlayToggled{})
	v.play.KeepFocus = true
	restart := widget.NewIconButton(icon.RotateCcw, "Play from the start")
	restart.OnClick = widget.Sends(Restarted{})
	v.tierLabel = small("Tier")
	v.tiers = widget.NewSegmented("1")
	v.tiers.KeepFocus = true
	v.tiers.OnChange = func(i int, _ *gunim.UI) gunim.Intent { return TierChosen{Tier: i + 1} }
	v.stings = widget.NewDropdown(widget.Labels("no stings"))
	v.stings.Label, v.stings.Disabled = "Sting", true
	v.sting = widget.NewButton("Sting")
	v.sting.Icon = icon.Trophy
	v.bpm = widget.NewNumberField(40, 240)
	v.bpm.Suffix = " BPM"
	v.bpm.OnChange = func(x float64, _ *gunim.UI) gunim.Intent { return BPMSet{BPM: x} }
	down := widget.NewIconButton(icon.Minus, "Down a semitone")
	down.OnClick = widget.Sends(Transposed{By: -1})
	up := widget.NewIconButton(icon.Plus, "Up a semitone")
	up.OnClick = widget.Sends(Transposed{By: 1})
	v.key = widget.NewLabel("")
	v.volume = widget.NewSlider(0, 1)
	v.volume.SetValue(s.Volume, nil)
	v.volume.OnChange = func(x float32, _ *gunim.UI) gunim.Intent { return VolumeSet{Volume: x} }
	save := widget.NewIconButton(icon.Save, "Save the song as a song.json")
	save.OnClick = widget.Sends(Saved{})
	gap := widget.NewSpacer()
	top := widget.Row(title, v.songs, v.play, restart, v.tierLabel, v.tiers, v.stings, v.sting, gap,
		widget.NewSized(v.bpm.TextField, 104, 0), down, v.key, up, widget.NewIcon(icon.Volume2, "Volume"),
		widget.NewSized(v.volume, 110, 0), save).Grow(gap, 1)
	top.Cross = widget.CrossCenter
	v.about = small("")

	v.stage = newStage()
	v.flow = newFlow()
	stagePage := widget.Column(v.stage, widget.NewSized(v.flow, 0, 250)).Grow(v.stage, 1)
	stagePage.Cross = widget.CrossStretch
	v.mixer = newMixerPane()
	v.patch = newPatchPane(func(string) gunim.Intent { return v.focus() })
	v.kit = newKitPane(func(string, string) gunim.Intent { return v.focus() })
	v.fx = newFxPane()
	v.pattern = newPatternPane(func(string) gunim.Intent { return v.focus() })
	v.beat = newBeatPane()
	v.tabs = widget.NewTabs(editorTabs, stagePage, v.mixer, v.patch, v.kit, v.fx, v.pattern, v.beat)
	v.tabs.Icons = []*icon.Icon{icon.Orbit, icon.SlidersHorizontal, icon.AudioWaveform, icon.Drum, icon.Waves, icon.Grid3x3, icon.Drumstick}
	v.tabs.OnChange = func(int, *gunim.UI) gunim.Intent { return v.focus() }
	left := v.tabs

	head := widget.NewLabel("Tracks")
	head.Size = widget.HeadingSize
	v.follow = widget.NewButton("Follow the tier")
	v.follow.Icon, v.follow.OnClick = icon.Layers, widget.Sends(TierFollowed{})
	hint := small("Type a pattern and hear it from the next bar. Click a card to start or stop its track.")
	headGap := widget.NewSpacer()
	headRow := widget.Row(head, headGap, v.follow).Grow(headGap, 1)
	headRow.Cross = widget.CrossCenter
	v.tracks = widget.NewList()
	v.tracks.SkipFocus = true
	v.tracks.OnActivate = func(k widget.Key, _ *gunim.UI) gunim.Intent { return PartClicked{Name: string(k)} }
	list := widget.NewScroll(v.tracks)
	right := widget.Column(headRow, hint, list).Grow(list, 1)
	right.Cross = widget.CrossStretch
	panel := widget.NewCard(right)
	panel.Fill = panelFill

	v.side = newSide(panel, 480)
	middle := widget.Row(left, v.side).Grow(left, 1)
	middle.Cross = widget.CrossStretch

	chordsLabel := widget.NewLabel("Chords")
	v.chords = widget.NewTextField()
	v.chords.Placeholder = "Am F C G, or vi IV I V"
	v.chords.Face = widget.MonoFont
	v.chords.OnChange = func(t string, _ *gunim.UI) gunim.Intent { return ChordsEdited{Text: t} }
	v.chordErr = small("")
	v.chordErr.Color = problem
	genButtons := make([]gunim.Node, 0, len(styles))
	for _, st := range styles {
		b := widget.NewButton(st)
		b.Icon, b.OnClick, b.Ghost, b.KeepFocus = icon.WandSparkles, widget.Sends(ChordsGenerated{Style: st}), true, true
		b.Tooltip = "Write a " + st + " progression"
		genButtons = append(genButtons, b)
	}
	v.keypad = small("")
	v.spectrum = newSpectrum()
	chordRow := widget.Row(append([]gunim.Node{chordsLabel, v.chords, small("Write:")}, genButtons...)...).Grow(v.chords, 1)
	chordRow.Cross = widget.CrossCenter
	bottomLeft := widget.Column(chordRow, v.chordErr, v.keypad)
	bottomLeft.Cross = widget.CrossStretch
	bottom := widget.Row(bottomLeft, widget.NewSized(v.spectrum, 380, 76)).Grow(bottomLeft, 1)
	bottom.Cross = widget.CrossStretch
	v.status = small("")

	col := widget.Column(top, v.about, middle, bottom, v.status).Grow(middle, 1)
	col.Cross = widget.CrossStretch
	v.Pad = widget.NewPad(col)
	return r
}

func (r *root) update(s Studio, u *gunim.UI) { r.view.update(s, u) }

// Overhear tells the wheel's guard of each move and turn of the wheel,
// wherever it lands.
func (r *root) Overhear(e input.Event, _ *gunim.UI) { wheel.overhear(e) }

// update shows s.
func (v *view) update(s Studio, u *gunim.UI) {
	v.s = s
	v.songs.SetSongs(s.Songs, s.Categories)
	v.songs.Choose(s.Song, u)
	v.about.Text = s.About
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
			v.tiers.Items = labels
			v.shown = s.Tiers
		}
		v.tierLabel.Text = "Tier"
		if v.tiers.Selected() != s.Tier-1 {
			v.tiers.SetSelected(s.Tier-1, u)
		}
	} else if v.shown != 0 {
		v.tiers.Items = []string{"wanders"}
		v.tierLabel.Text = "Parts"
		v.shown = 0
		// The one label is the one chosen: the last song's tier would
		// leave the highlight where its button was, over the stings.
		v.tiers.SetSelected(0, u)
	}
	if fmt.Sprint(s.Stings) != fmt.Sprint(v.stingNames) {
		v.stingNames = s.Stings
		if len(s.Stings) == 0 {
			v.stings.SetItems(widget.Labels("no stings"))
			v.stings.Disabled = true
		} else {
			v.stings.SetItems(widget.Labels(s.Stings...))
			v.stings.Disabled = false
		}
		v.stings.SetSelected(0, u)
	}
	v.sting.Disabled = len(s.Stings) == 0
	if len(s.Stings) > 0 {
		v.sting.OnClick = widget.Sends(StingPlayed{Name: s.Stings[min(v.stings.Selected(), len(s.Stings)-1)]})
	}
	v.key.Text = s.Key
	v.follow.Disabled = false
	if v.gen != s.Gen {
		v.gen = s.Gen
		v.chords.SetText(s.ChordText, u)
		v.chords.Select(0, 0)
		v.bpm.SetText(strconv.FormatFloat(s.BPM, 'f', -1, 64), u)
	}
	v.chordErr.Text = s.ChordErr
	if s.Keypad {
		v.keypad.Text = "Keypad: the digit keys play the key's pentatonic over the song, so typing always fits. F1 to F4 set the tier; Space plays and pauses."
	} else {
		v.keypad.Text = "F1 to F4 set the tier; Space plays and pauses."
	}
	v.status.Text = s.Status
	v.stage.s = s
	v.flow.s = s
	if s.EditorGen != v.editorGen {
		v.editorGen = s.EditorGen
		v.open(s, u)
	}
	// The mixer and the patch page, which shows the track's strip and steps
	// through the tracks, take the window's width.
	v.side.open = v.tabs.Selected() != tabMixer && v.tabs.Selected() != tabPatch
	v.mixer.update(s, u)
	v.patch.update(s)
	v.kit.update(s)
	v.fx.update(s)
	v.pattern.update(s, u)
	v.beat.update(s, u)
	if f := v.focus(); f != v.focused {
		v.focused = f
		u.Send(v.root, f)
	}
	v.spectrum.target = s.Spectrum
	widget.Sync(v.tracks, u, s.Tracks, func(t TrackRow) widget.Key { return widget.Key(t.Name) }, newCard,
		func(c *card, t TrackRow, u *gunim.UI) { c.set(t, u); c.patches(s) })
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
	patch                       *widget.Dropdown
	patchNames                  []string
	patchName                   string
}

func newCard(t TrackRow) *card {
	c := &card{name: widget.NewLabel(""), doing: small(""), err: small(""), swatch: &swatch{}, meter: &meter{}, gen: -1, track: t.Name}
	c.err.Color = problem
	c.field = widget.NewTextField()
	c.field.Face = widget.MonoFont
	c.field.Placeholder = "a pattern, as bd ~ sn ~ or x*8"
	name := t.Name
	c.field.OnChange = func(s string, _ *gunim.UI) gunim.Intent { return PatternEdited{Track: name, Text: s} }
	knob := func(lo, hi float32, k string) *widget.Slider {
		s := widget.NewSlider(lo, hi)
		s.OnChange = func(x float32, _ *gunim.UI) gunim.Intent { return KnobSet{Track: name, Knob: k, Value: x} }
		return s
	}
	c.gain, c.filter, c.reverb, c.delay = knob(-36, 6, "gain"), knob(0, 1, "filter"), knob(0, 1, "reverb"), knob(0, 1, "delay")
	c.patch = widget.NewDropdown(widget.Labels("–"))
	c.patch.Label = "Patch"
	c.patch.MaxWidth = 110
	c.patch.OnChange = func(i int, _ *gunim.UI) gunim.Intent {
		if i < len(c.patchNames) {
			return TrackPatch{Track: name, Patch: c.patchNames[i]}
		}
		return TrackPatch{Track: name, Patch: c.patchName}
	}
	editPattern := widget.NewIconButton(icon.Grid3x3, "Edit the pattern")
	editPattern.OnClick, editPattern.KeepFocus = widget.Sends(OpenEditor{Editor: "Pattern", Track: name}), true
	editPatch := widget.NewIconButton(icon.AudioWaveform, "Edit the patch")
	editPatch.OnClick, editPatch.KeepFocus = widget.Sends(OpenEditor{Editor: "Patch", Track: name}), true
	head := widget.Row(widget.NewSized(c.swatch, 12, 12), c.name, c.doing, widget.NewSpacer(), widget.NewSized(c.meter, 60, 8), c.patch, editPatch, editPattern)
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

func (c *card) set(t TrackRow, u *gunim.UI) {
	label := t.Name
	switch {
	case t.Tier > 0:
		label = fmt.Sprintf("%s · tier %d", t.Name, t.Tier)
	case t.Core:
		label = t.Name + " · always"
	}
	c.name.Text = label
	c.doing.Text = t.Doing
	c.doing.Color = widget.MenuHint
	c.Fill = cardFill
	if t.Playing {
		c.doing.Color = widget.Accent
		c.Fill = playing
	}
	c.err.Text = t.PatternErr
	c.patchName = t.Patch
	c.swatch.c = hexColor(t.Color, 0)
	c.swatch.on = t.Playing
	c.meter.c = hexColor(t.Color, 0)
	c.meter.target = t.Level
	if c.gen != t.Gen {
		c.gen = t.Gen
		c.field.SetText(t.Pattern, u)
		c.field.Select(0, 0)
		c.gain.SetValue(t.Gain, nil)
		c.filter.SetValue(t.Filter, nil)
		c.reverb.SetValue(t.Reverb, nil)
		c.delay.SetValue(t.Delay, nil)
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

// The editors the middle of the window shows, by their tabs.
var editorTabs = []string{"Stage", "Mixer", "Patch", "Kit", "Effects", "Pattern", "Beat"}

const (
	tabStage = iota
	tabMixer
	tabPatch
	tabKit
	tabEffects
	tabPattern
	tabBeat
)

// focus says what the editors show: the patch and the drum to render,
// and the track whose sound the scope shows, for the tab chosen.
func (v *view) focus() Focus {
	f := Focus{Patch: v.patch.name, Kit: v.kit.name, Drum: v.kit.drum}
	song := v.s.Doc
	firstPlaying := func(patch string) string {
		if song == nil {
			return ""
		}
		for _, t := range song.Tracks {
			if t.Patch == patch {
				return t.Name
			}
		}
		return ""
	}
	switch v.tabs.Selected() {
	case tabPatch:
		f.Track = firstPlaying(v.patch.name)
		if song != nil {
			if t := track(song, v.patch.track); t != nil && t.Patch == v.patch.name {
				f.Track = t.Name
			}
		}
	case tabKit:
		f.Track = firstPlaying(v.kit.name)
	case tabPattern:
		f.Track = v.pattern.track
	}
	return f
}

// open shows the editor the studio asks for, on its track.
func (v *view) open(s Studio, u *gunim.UI) {
	song := s.Doc
	t := track(song, s.EditorTrack)
	tab := slices.Index(editorTabs, s.Editor)
	if tab < 0 {
		return
	}
	if t != nil {
		switch tab {
		case tabPattern:
			v.pattern.track = t.Name
			v.pattern.gen, v.pattern.sel = -1, ""
		case tabKit:
			if p := song.Patches[t.Patch]; p != nil && p.Kind == "drums" {
				v.kit.choose(t.Patch)
			}
		case tabPatch:
			if p := song.Patches[t.Patch]; p != nil && p.Kind == "drums" {
				tab = tabKit
				v.kit.choose(t.Patch)
			} else {
				v.patch.choose(t.Patch)
				v.patch.track = t.Name
			}
		}
	}
	v.tabs.SetSelected(tab, u)
}

// patches offers the song's patches for the card's track to play.
func (c *card) patches(s Studio) {
	if s.Doc == nil {
		return
	}
	names := sortedNames(s.Doc.Patches)
	if !slices.Equal(names, c.patchNames) {
		c.patchNames = names
		c.patch.SetItems(widget.Labels(names...))
	}
	c.patch.SetSelected(max(slices.Index(names, c.patchName), 0), nil)
}
