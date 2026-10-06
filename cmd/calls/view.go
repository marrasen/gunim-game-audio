package main

import (
	"image/color"
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-music/calls"
)

// The window's own tokens.
var (
	smallSize = theme.Length("calls.small", 12)
	problem   = theme.Color("calls.problem", color.NRGBA{0xff, 0x8a, 0x7a, 0xff})
	good      = theme.Color("calls.good", color.NRGBA{0x6f, 0xe0, 0x9a, 0xff})
	cardFill  = theme.Color("calls.card", color.NRGBA{0x1e, 0x22, 0x2e, 0xff})
	chosen    = theme.Color("calls.chosen", color.NRGBA{0x26, 0x30, 0x48, 0xff})
)

// maxLayers is how many layers a call's editor shows, and maxKnobs how
// many knobs a layer's.
const maxLayers, maxKnobs = 4, 24

// The calls' names as the window shows them.
var kindNames = map[string]string{calls.Hello: "Hello", calls.Cheer: "Cheer", calls.Oops: "Oops"}

func small(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Color = smallSize, widget.MenuHint
	return l
}

// view is the window, with handles on what updates change.
type view struct {
	*widget.Pad
	phone   *widget.Switch
	songs   *widget.Dropdown
	save    *widget.Button
	status  *widget.Label
	list    *widget.List
	name    *widget.Label
	about   *widget.Label
	cards   [3]*callCard
	editing *widget.Label
	callKs  []*knob
	master  []*knob
	spec    *spectrum
	layers  [maxLayers]*layerSlot
	add     *widget.MenuButton
	notes   *widget.TextArea
	gen     int
	sel     int
	models  []string
}

// root is the window's root: it hears the keys that play.
type root struct{ *view }

func buildView(s Lab) *root {
	v := &view{gen: -1, models: calls.ModelNames()}
	title := widget.NewLabel("Companion calls")
	title.Size = widget.HeadingSize
	v.phone = widget.NewSwitch("Phone speaker")
	v.phone.Tooltip = "Plays the calls as a phone's small speaker does: nothing under about 700 Hz, little over 8 kHz"
	v.phone.OnChange = func(on bool) gunim.Intent { return PhoneSet{On: on} }
	v.songs = widget.NewDropdown(s.Songs...)
	v.songs.Label = "Music under the calls"
	v.songs.OnChange = func(i int) gunim.Intent { return SongChosen{Song: i} }
	vol := widget.NewSlider(0, 1)
	vol.Set(s.MusicVolume)
	vol.OnChange = func(x float32) gunim.Intent { return MusicVolumeSet{Volume: x} }
	volBox := widget.NewSized(vol, 110, 0)
	v.save = widget.NewButton("Save")
	v.save.Icon, v.save.Kind, v.save.On = icon.Save, widget.ButtonPrimary, Saved{}
	revert := widget.NewButton("Revert")
	revert.Icon, revert.On = icon.Undo2, Reverted{}
	revert.Tooltip = "Reads the companion back from its file, dropping changes not saved"
	gap := widget.NewSpacer()
	top := widget.Row(title, gap, v.phone, v.songs, volBox, v.save, revert).Grow(gap, 1)
	top.Cross = widget.CrossCenter
	v.status = small(s.Status)

	v.list = widget.NewList()
	v.list.OnClick = func(k widget.Key) gunim.Intent { return CompanionChosen{ID: string(k)} }
	left := widget.NewSized(widget.NewScroll(v.list), 250, 0)

	v.name = widget.NewLabel("")
	v.name.Size = widget.HeadingSize
	v.about = small("")
	all := widget.NewButton("Play all three")
	all.Icon, all.On = icon.Play, PlayAll{}
	all.Tooltip = "Plays hello, cheer and oops one after another (A)"
	hgap := widget.NewSpacer()
	head := widget.Row(widget.Column(v.name, v.about), hgap, all).Grow(hgap, 1)
	head.Cross = widget.CrossCenter
	cardNodes := make([]gunim.Node, 3)
	for i := range v.cards {
		v.cards[i] = newCallCard(i)
		cardNodes[i] = v.cards[i]
	}
	cardRow := widget.Row(cardNodes...)
	for _, c := range cardNodes {
		cardRow.Grow(c, 1)
	}
	cardRow.Cross = widget.CrossStretch

	v.editing = widget.NewLabel("")
	v.editing.Size = widget.HeadingSize
	v.callKs = []*knob{{}, {}}
	v.master = []*knob{{}, {}, {}}
	var setKnobs []gunim.Node
	for _, k := range v.callKs {
		setKnobs = append(setKnobs, k)
	}
	var masterKnobs []gunim.Node
	for _, k := range v.master {
		masterKnobs = append(masterKnobs, k)
	}
	callPanel := panel("THIS CALL", widget.Row(setKnobs...))
	masterPanel := panel("EVERY CALL", widget.Row(masterKnobs...))
	v.spec = &spectrum{}
	specPanel := panel("SPECTRUM", v.spec)
	knobsRow := widget.Row(callPanel, masterPanel, specPanel).Grow(specPanel, 1)
	knobsRow.Cross = widget.CrossStretch
	layerNodes := []gunim.Node{}
	for i := range v.layers {
		v.layers[i] = newLayerSlot(v, i)
		layerNodes = append(layerNodes, v.layers[i].sw)
	}
	v.add = widget.NewMenuButton("Add a layer", v.models...)
	v.add.Icon = icon.Plus
	v.add.OnPick = func(i int) gunim.Intent { return LayerAdded{Model: v.models[i]} }
	v.notes = widget.NewTextArea()
	v.notes.Placeholder = "What should change in this call? Write it here and save; Claude reads it from the companion's file."
	v.notes.OnChange = func(t string) gunim.Intent { return NotesSet{Text: t} }
	notesPanel := panel("NOTES FOR CLAUDE", widget.NewSized(v.notes, 0, 90))
	addRow := widget.Row(v.add)
	editor := widget.Column(append([]gunim.Node{v.editing, knobsRow}, append(layerNodes, addRow, notesPanel)...)...)
	editor.Cross = widget.CrossStretch
	keys := small("Keys: Space plays the call open · N makes a new take · 1, 2, 3 open and play hello, cheer, oops · A plays all three. " +
		"Drag a knob up or down, Shift for fine steps; a double click sets it back. The call plays as you let go.")
	right := widget.Column(head, cardRow, editor, keys)
	right.Cross = widget.CrossStretch
	scroll := widget.NewScroll(widget.NewPad(right))
	body := widget.Row(left, scroll).Grow(scroll, 1)
	body.Cross = widget.CrossStretch
	col := widget.Column(top, v.status, body).Grow(body, 1)
	col.Cross = widget.CrossStretch
	v.Pad = widget.NewPad(col)
	v.Padding = widget.CardPadding
	return &root{v}
}

// panel is a card with a small title over what it holds.
func panel(title string, kids ...gunim.Node) *widget.Card {
	col := widget.Column(append([]gunim.Node{small(title)}, kids...)...)
	col.Cross = widget.CrossStretch
	c := widget.NewCard(col)
	c.Fill = cardFill
	return c
}

func (r *root) update(s Lab, u *gunim.UI) { r.view.update(s, u) }

func (v *view) update(s Lab, u *gunim.UI) {
	v.phone.On = s.Phone
	v.songs.Selected = s.Song
	v.status.SetText(s.Status)
	v.save.Label = "Save"
	if s.Dirty {
		v.save.Label = "Save changes"
	}
	widget.Sync(v.list, u, s.Companions, func(c CompanionRow) widget.Key { return widget.Key(c.ID) }, newCompanionRow, (*companionRow).set)
	v.name.SetText(s.Name)
	v.about.SetText(s.About)
	v.sel = s.Selected
	for i, c := range s.Calls {
		v.cards[i].set(c, i == s.Selected)
	}
	e := s.Editor
	v.editing.SetText("Editing " + strings.ToLower(kindNames[e.Kind]))
	made := len(e.Layers) > 0
	v.callKs[0].show(-1, ParamView{Name: "vary", Label: "Vary", Lo: 0, Hi: 1, Def: 0.5, Value: e.Vary,
		About: "How far each take strays from the call as set"})
	v.callKs[1].show(-1, ParamView{Name: "room", Label: "Room", Lo: 0, Hi: 1, Def: 0.15, Value: e.Room,
		About: "How much of a small room is heard round the call"})
	v.master[0].show(-2, ParamView{Name: "highpass", Label: "Low cut", Unit: "Hz", Lo: 100, Hi: 600, Def: 300, Value: s.HighPass, Log: true})
	v.master[1].show(-2, ParamView{Name: "loudness", Label: "Loudness", Unit: "", Lo: -24, Hi: -8, Def: -14, Value: s.Loudness})
	v.master[2].show(-2, ParamView{Name: "ceiling", Label: "Ceiling", Unit: "", Lo: -6, Hi: 0, Def: -1, Value: s.Ceiling})
	if s.Selected < len(s.Calls) {
		v.spec.data = s.Calls[s.Selected].Spectrum
	}
	for i, slot := range v.layers {
		if i < len(e.Layers) {
			slot.set(e.Layers[i])
		} else {
			slot.sw.which = -1
		}
	}
	v.add.Title = "Add a layer"
	if !made {
		v.add.Title = "Make this call: add a layer"
	}
	if s.Gen != v.gen {
		v.notes.SetText(e.Notes)
		v.gen = s.Gen
	}
	u.Invalidate()
}

// Handle plays the calls by key.
func (r *root) Handle(e input.Event, u *gunim.UI) bool {
	p, ok := e.(input.KeyPress)
	if !ok || p.Mods != 0 {
		return false
	}
	switch p.Char {
	case ' ':
		u.Send(r, PlayCall{Call: r.sel})
	case 'n', 'N':
		u.Send(r, NewTake{Call: r.sel})
	case 'a', 'A':
		u.Send(r, PlayAll{})
	case '1', '2', '3':
		i := int(p.Char - '1')
		u.Send(r, CallChosen{Call: i})
		u.Send(r, PlayCall{Call: i})
	default:
		return false
	}
	return true
}

// companionRow is a companion in the list.
type companionRow struct {
	*widget.Card
	name, doing *widget.Label
}

func newCompanionRow(c CompanionRow) *companionRow {
	r := &companionRow{name: widget.NewLabel(""), doing: small("")}
	col := widget.Column(r.name, r.doing)
	r.Card = widget.NewCard(col)
	r.set(c, nil)
	return r
}

func (r *companionRow) set(c CompanionRow, _ *gunim.UI) {
	r.name.SetText(c.Name)
	r.doing.SetText(c.Doing)
	r.doing.Color = widget.MenuHint
	if c.Made {
		r.doing.Color = good
	}
	r.Fill = widget.CardFill
	if c.Open {
		r.Fill = chosen
	}
}

// callCard is a call: its take drawn, how it measures, and buttons to
// play it.
type callCard struct {
	*widget.Card
	title, take, stats, problems *widget.Label
	wave                         *wave
	edit                         *widget.Button
}

func newCallCard(i int) *callCard {
	c := &callCard{title: widget.NewLabel(kindNames[calls.Kinds[i]]), take: small(""), stats: small(""), problems: small("")}
	c.title.Size = theme.Length("calls.cardtitle", 17)
	c.problems.Color = problem
	c.wave = &wave{h: 84, click: PlayCall{Call: i}, playhead: -1}
	play := widget.NewButton("Play")
	play.Icon, play.On = icon.Play, PlayCall{Call: i}
	again := widget.NewButton("New take")
	again.Icon, again.On = icon.Shuffle, NewTake{Call: i}
	again.Tooltip = "Makes another take, as the game does each time, and plays it"
	asSet := widget.NewButton("As set")
	asSet.Icon, asSet.On = icon.RotateCcw, AsSet{Call: i}
	asSet.Tooltip = "Plays the call exactly as its knobs set it, the take the files are made from"
	c.edit = widget.NewButton("Edit")
	c.edit.On = CallChosen{Call: i}
	gap := widget.NewSpacer()
	head := widget.Row(c.title, gap, c.take).Grow(gap, 1)
	head.Cross = widget.CrossCenter
	btns := widget.NewWrap(play, again, asSet, c.edit)
	col := widget.Column(head, c.wave, c.stats, c.problems, btns)
	col.Cross = widget.CrossStretch
	c.Card = widget.NewCard(col)
	c.Fill = cardFill
	return c
}

func (c *callCard) set(v CallView, selected bool) {
	c.wave.data = v.Wave
	c.wave.playhead = v.Playhead
	c.take.SetText(v.Take)
	c.stats.SetText(v.Stats)
	switch {
	case !v.Made:
		c.problems.SetText("")
		c.stats.SetText("Not made yet.")
	case len(v.Problems) == 0:
		c.problems.SetText("Meets the brief.")
		c.problems.Color = good
	default:
		c.problems.SetText("Short of the brief: " + strings.Join(v.Problems, "; ") + ".")
		c.problems.Color = problem
	}
	c.Fill = cardFill
	c.edit.Label, c.edit.Active, c.edit.Disabled = "Edit", false, false
	if selected {
		c.Fill = chosen
		c.edit.Label, c.edit.Active = "Editing", true
	}
}

// layerSlot is a layer's editor: its model, and a knob for each of its
// numbers.
type layerSlot struct {
	i      int
	sw     *switcher
	model  *widget.Dropdown
	about  *widget.Label
	knobs  *grid
	ks     []*knob
	shown  string
	models []string
}

func newLayerSlot(v *view, i int) *layerSlot {
	s := &layerSlot{i: i, models: v.models}
	s.model = widget.NewDropdown(v.models...)
	s.model.Label = "Model"
	s.model.OnChange = func(m int) gunim.Intent { return ModelChosen{Layer: i, Model: s.models[m]} }
	s.about = small("")
	remove := widget.NewButton("Remove")
	remove.Icon, remove.On = icon.Trash2, LayerRemoved{Layer: i}
	gap := widget.NewSpacer()
	head := widget.Row(small("LAYER"), s.model, s.about, gap, remove).Grow(s.about, 1)
	head.Cross = widget.CrossCenter
	kids := make([]gunim.Node, maxKnobs)
	for j := range kids {
		k := &knob{}
		s.ks = append(s.ks, k)
		kids[j] = k
	}
	s.knobs = &grid{kids: kids, cell: geom.Sz(knobW+4, knobH+4)}
	col := widget.Column(head, s.knobs)
	col.Cross = widget.CrossStretch
	card := widget.NewCard(col)
	card.Fill = cardFill
	s.sw = &switcher{kids: []gunim.Node{card}, which: 0}
	return s
}

func (s *layerSlot) set(l LayerView) {
	s.sw.which = 0
	s.model.Selected = max(slices.Index(s.models, l.Model), 0)
	s.about.SetText(l.About)
	n := min(len(l.Params), maxKnobs)
	s.knobs.n = n
	for j := range n {
		s.ks[j].show(s.i, l.Params[j])
	}
}
