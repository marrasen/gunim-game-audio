package main

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/calls"
	"github.com/marrasen/gunim-game-audio/internal/songpicker"
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
var kindNames = map[string]string{
	calls.Hello: "Hello", calls.Cheer: "Cheer", calls.Oops: "Oops",
	calls.Taunt: "Taunt", calls.Roar: "Roar", calls.Hurt: "Hurt", calls.Laugh: "Laugh",
	calls.Worried: "Worried", calls.Defeat: "Defeat", calls.Whimper: "Whimper",
}

// cardsPerRow is how many call cards stand side by side: a companion's
// three in one row, a boss's seven in two.
const cardsPerRow = 4

// kindName returns a call's name as the window shows it: its own, or an
// effect's name, capitalised.
func kindName(kind string) string {
	if n, ok := kindNames[kind]; ok {
		return n
	}
	if kind == "" {
		return ""
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

func small(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Color = smallSize, widget.MenuHint
	return l
}

// view is the window, with handles on what updates change.
type view struct {
	*widget.Pad
	keys    *widget.Label
	phone   *widget.Switch
	songs   *songpicker.Picker
	save    *widget.Button
	status  *widget.Label
	list    *widget.List
	name    *widget.Label
	about   *widget.Label
	cards   *cardGrid
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
	v.phone.OnChange = func(on bool, _ *gunim.UI) gunim.Intent { return PhoneSet{On: on} }
	v.songs = songpicker.New(s.Songs, s.Categories, func(i int, _ *gunim.UI) gunim.Intent { return SongChosen{Song: i} })
	v.songs.Label = "Music under the calls"
	vol := widget.NewSlider(0, 1)
	vol.SetValue(s.MusicVolume, nil)
	vol.OnChange = func(x float32, _ *gunim.UI) gunim.Intent { return MusicVolumeSet{Volume: x} }
	volBox := widget.NewSized(vol, 110, 0)
	v.save = widget.NewButton("Save")
	v.save.Icon, v.save.Kind, v.save.OnClick = icon.Save, widget.ButtonPrimary, widget.Sends(Saved{})
	revert := widget.NewButton("Revert")
	revert.Icon, revert.OnClick = icon.Undo2, widget.Sends(Reverted{})
	revert.Tooltip = "Reads the companion back from its file, dropping changes not saved"
	gap := widget.NewSpacer()
	top := widget.Row(title, gap, v.phone, v.songs, volBox, v.save, revert).Grow(gap, 1)
	top.Cross = widget.CrossCenter
	v.status = small(s.Status)

	v.list = widget.NewList()
	v.list.OnActivate = func(k widget.Key, _ *gunim.UI) gunim.Intent { return CompanionChosen{ID: string(k)} }
	left := widget.NewSized(widget.NewScroll(v.list), 250, 0)

	v.name = widget.NewLabel("")
	v.name.Size = widget.HeadingSize
	v.about = small("")
	all := widget.NewButton("Play all")
	all.Icon, all.OnClick = icon.Play, widget.Sends(PlayAll{})
	all.Tooltip = "Plays every call, one after another (A)"
	hgap := widget.NewSpacer()
	head := widget.Row(widget.Column(v.name, v.about), hgap, all).Grow(hgap, 1)
	head.Cross = widget.CrossCenter
	// The call cards, in rows: one for a companion's three, two for a
	// boss's seven.
	v.cards = newCardGrid()

	v.editing = widget.NewLabel("")
	v.editing.Size = widget.HeadingSize
	v.callKs = []*knob{{}, {}, {}, {}, {}}
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
	v.add = widget.NewMenuButton("Add a layer", widget.Labels(v.models...))
	v.add.Icon = icon.Plus
	v.add.OnPick = func(i int, _ *gunim.UI) gunim.Intent { return LayerAdded{Model: v.models[i]} }
	v.notes = widget.NewTextArea()
	v.notes.Placeholder = "What should change in this call? Write it here and save; Claude reads it from the companion's file."
	v.notes.OnChange = func(t string, _ *gunim.UI) gunim.Intent { return NotesSet{Text: t} }
	notesPanel := panel("NOTES FOR CLAUDE", widget.NewSized(v.notes, 0, 90))
	addRow := widget.Row(v.add)
	editor := widget.Column(append([]gunim.Node{v.editing, knobsRow}, append(layerNodes, addRow, notesPanel)...)...)
	editor.Cross = widget.CrossStretch
	v.keys = small("")
	right := widget.Column(head, v.cards, editor, v.keys)
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
	v.phone.SetChecked(s.Phone, u)
	v.songs.SetSongs(s.Songs, s.Categories)
	v.songs.Choose(s.Song, u)
	v.status.Text = s.Status
	v.save.Label = "Save"
	if s.Dirty {
		v.save.Label = "Save changes"
	}
	widget.Sync(v.list, u, s.Companions, func(c CompanionRow) widget.Key { return widget.Key(c.ID) }, newCompanionRow, (*companionRow).set)
	v.name.Text = s.Name
	v.about.Text = s.About
	v.sel = s.Selected
	v.cards.set(s.Calls, s.Selected)
	v.keys.Text = keysHint(s.Calls)
	e := s.Editor
	v.editing.Text = "Editing " + strings.ToLower(kindName(e.Kind))
	made := len(e.Layers) > 0
	v.callKs[0].show(-1, ParamView{Name: "vary", Label: "Vary", Lo: 0, Hi: 1, Def: 0.5, Value: e.Vary,
		About: "How far each take strays from the call as set"})
	v.callKs[1].show(-1, ParamView{Name: "room", Label: "Room", Lo: 0, Hi: 1, Def: 0.15, Value: e.Room,
		About: "How much of a small room is heard round the call"})
	v.callKs[2].show(-1, ParamView{Name: "cut", Label: "Ends by", Unit: "s", Lo: 0, Hi: 1.5, Def: 0, Value: e.Cut, Zero: "rings out",
		About: "Fades the call out to end by then; 0 lets it ring out"})
	v.callKs[3].show(-1, ParamView{Name: "presence", Label: "Presence", Unit: "dB", Lo: -6, Hi: 12, Def: 0, Value: e.Presence,
		About: "Lifts the call about 2.2 kHz, where a phone's speaker carries it"})
	v.callKs[4].show(-1, ParamView{Name: "loop", Label: "Loop", Unit: "s", Lo: 0, Hi: 6, Def: 0, Value: e.Loop, Zero: "no",
		About: "Makes the call a loop this long, played over and over without a seam, as a hum; Play again stops it"})
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
		v.notes.SetText(e.Notes, u)
		v.gen = s.Gen
	}
	u.Invalidate()
}

// CatchKey plays the calls by key, wherever the keyboard is, but for
// keys something focused took, as the notes' typing.
func (r *root) CatchKey(e input.Event, u *gunim.UI) bool {
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
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		i := int(p.Char - '1')
		if i >= r.cards.n {
			return false
		}
		u.Send(r, CallChosen{Call: i})
		u.Send(r, PlayCall{Call: i})
	default:
		return false
	}
	return true
}

// keysHint says which keys play what, for the calls shown.
func keysHint(cs []CallView) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = strings.ToLower(kindName(c.Kind))
	}
	digits := "1"
	if n := len(cs); n > 1 {
		digits = fmt.Sprintf("1 to %d", n)
	}
	return "Keys: Space plays the call open · N makes a new take · " + digits + " open and play " +
		strings.Join(names, ", ") + " · A plays them all. " +
		"Drag a knob up or down, Shift for fine steps; a double click sets it back. The call plays as you let go."
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
	r.name.Text = c.Name
	r.doing.Text = c.Doing
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

// cardGrid holds a card for each call a character may make, and shows
// the first n, in rows of cardsPerRow: a companion's three in one row,
// a boss's seven in two. The cards it does not show take no room and
// are not drawn.
type cardGrid struct {
	cards []*callCard
	n     int
}

// cardGap is the room between two cards, across and down.
const cardGap = 10

func newCardGrid() *cardGrid {
	g := &cardGrid{}
	for i := range max(len(calls.Kinds), len(calls.BossKinds)) {
		g.cards = append(g.cards, newCallCard(i, ""))
	}
	return g
}

// Children implements [gunim.Parent].
func (g *cardGrid) Children() []gunim.Node {
	out := make([]gunim.Node, len(g.cards))
	for i, c := range g.cards {
		out[i] = c
	}
	return out
}

// Layout implements [gunim.Node]: each row's cards side by side, each
// as wide as the row allows and as tall as the row's tallest.
func (g *cardGrid) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	w, y := c.Max.W, float32(0)
	for from := 0; from < g.n; from += cardsPerRow {
		k := min(cardsPerRow, g.n-from)
		cw := (w - cardGap*float32(k-1)) / float32(k)
		h := float32(0)
		for j := range k {
			h = max(h, kids.At(from+j).Layout(gunim.Constraints{Min: geom.Sz(cw, 0), Max: geom.Sz(cw, c.Max.H)}).H)
		}
		for j := range k {
			kids.At(from + j).Layout(gunim.Tight(geom.Sz(cw, h)))
			kids.At(from + j).Place(geom.Pt(float32(j)*(cw+cardGap), y))
		}
		y += h + cardGap
	}
	return geom.Sz(w, max(0, y-cardGap))
}

// Paint implements [gunim.Node].
func (g *cardGrid) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for i := range min(g.n, kids.Len()) {
		kids.At(i).Paint(p)
	}
}

// set shows calls on the cards, the chosen one marked.
func (g *cardGrid) set(views []CallView, selected int) {
	g.n = min(len(views), len(g.cards))
	for i := range g.n {
		g.cards[i].set(views[i], i == selected)
	}
}

func newCallCard(i int, kind string) *callCard {
	c := &callCard{title: widget.NewLabel(kindName(kind)), take: small(""), stats: small(""), problems: small("")}
	c.title.Size = theme.Length("calls.cardtitle", 17)
	c.problems.Color = problem
	c.wave = &wave{h: 84, click: PlayCall{Call: i}, playhead: -1}
	play := widget.NewButton("Play")
	play.Icon, play.OnClick = icon.Play, widget.Sends(PlayCall{Call: i})
	again := widget.NewButton("New take")
	again.Icon, again.OnClick = icon.Shuffle, widget.Sends(NewTake{Call: i})
	again.Tooltip = "Makes another take, as the game does each time, and plays it"
	asSet := widget.NewButton("As set")
	asSet.Icon, asSet.OnClick = icon.RotateCcw, widget.Sends(AsSet{Call: i})
	asSet.Tooltip = "Plays the call exactly as its knobs set it, the take the files are made from"
	c.edit = widget.NewButton("Edit")
	c.edit.OnClick = widget.Sends(CallChosen{Call: i})
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
	c.title.Text = kindName(v.Kind)
	c.wave.data = v.Wave
	c.wave.playhead = v.Playhead
	c.take.Text = v.Take
	c.stats.Text = v.Stats
	switch {
	case !v.Made:
		c.problems.Text = ""
		c.stats.Text = "Not made yet."
	case len(v.Problems) == 0:
		c.problems.Text = "Passes the checks."
		c.problems.Color = good
	default:
		c.problems.Text = "Fails the checks: " + strings.Join(v.Problems, "; ") + "."
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
	s.model = widget.NewDropdown(widget.Labels(v.models...))
	s.model.Label = "Model"
	s.model.OnChange = func(m int, _ *gunim.UI) gunim.Intent { return ModelChosen{Layer: i, Model: s.models[m]} }
	s.about = small("")
	remove := widget.NewButton("Remove")
	remove.Icon, remove.OnClick = icon.Trash2, widget.Sends(LayerRemoved{Layer: i})
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
	s.model.SetSelected(max(slices.Index(s.models, l.Model), 0), nil)
	s.about.Text = l.About
	n := min(len(l.Params), maxKnobs)
	s.knobs.n = n
	for j := range n {
		s.ks[j].show(s.i, l.Params[j])
	}
}
