package main

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/synth"
)

// The choices the editors offer, as the song writes them.
var (
	waves      = []string{"saw", "pulse", "tri", "sine", "fm", "noise", "sawtri", "pulsetri", "pulsesaw", "nespulse", "nestri", "nesnoise", "nesmetal", "gbwave"}
	filters    = []string{"none", "lp", "lp24", "hp", "bp", "sidlp", "sidbp", "sidhp", "sidnotch"}
	lfoTargets = []string{"pitch", "cutoff", "amp", "width", "pan"}
	lfoWaves   = []string{"sine", "tri", "saw", "square", "random"}
	vowels     = []string{"off", "a", "e", "i", "o", "u"}
	// waveNames and filterNames are what the selectors print for each:
	// the waves of an analogue synth, then the chips'; the usual
	// filters, then the SID's.
	waveNames = []string{"saw", "pulse", "tri", "sine", "fm", "noise",
		"saw·tri", "pulse·tri", "pulse·saw", "nes pulse", "nes tri", "nes noise", "nes metal", "gb wave"}
	filterNames = []string{"off", "lp 12", "lp 24", "hp", "bp", "sid lp", "sid bp", "sid hp", "sid notch"}
)

// maxOsc and maxLFO are the oscillators and LFOs a patch editor shows.
const (
	maxOsc = 4
	maxLFO = 2
)

// patchPane edits a synth or pluck patch: its oscillators, each with a
// readout of its wave; its filter, with its response; its envelopes,
// drawn; its LFOs; and how it plays. Under them a note of it is drawn,
// whole and close up, the sound of a track playing it runs as on a
// scope, and a keyboard plays it.
type patchPane struct {
	*widget.Scroll
	base    string
	name    string
	names   []string
	song    *synth.Song
	picker  *widget.Dropdown
	about   *widget.Label
	save    *widget.IconButton
	load    *widget.IconButton
	copy    *widget.IconButton
	body    *switcher
	oscs    [maxOsc]*oscSlot
	oscRow  *widget.Flex
	lfos    [maxLFO]*lfoSlot
	pluckKs []*knob
	filter  *selector
	arpKs   []*knob
	arpOn   *widget.Button
	arpChrd *widget.Button
	arpStep *widget.TextField
	arpGen  int
	chipOn  *widget.Button
	chipKs  []*knob
	bendKs  []*knob
	resp    *response
	fKnobs  []*knob
	fenv    *envelope
	aenv    *envelope
	feKnobs []*knob
	aeKnobs []*knob
	vKnobs  []*knob
	vowel   *selector
	vocoder *widget.Button
	slide   *widget.Button
	lib     *libraryBar
	tree    *presetTree
	// track is the track whose strip shows, one that plays the patch,
	// and tracks those that play a synth or a pluck, to step through.
	track     string
	tracks    []string
	trackName *widget.Label
	strip     *channel
	stripSw   *switcher
	note      *wave
	cycle     *wave
	live      *wave
	keys      *keyboard
	// changed tells the root the patch chosen changed, and returns the
	// intent that says so.
	changed func(name string) gunim.Intent
}

func newPatchPane(changed func(string) gunim.Intent) *patchPane {
	pp := &patchPane{changed: changed}
	pp.picker = widget.NewDropdown(widget.Labels("–"))
	pp.picker.Label = "Patch"
	pp.picker.OnChange = func(i int, _ *gunim.UI) gunim.Intent {
		if i < len(pp.names) {
			pp.choose(pp.names[i])
		}
		return pp.changed(pp.name)
	}
	newSynth := widget.NewButton("New synth")
	newSynth.Icon, newSynth.OnClick, newSynth.Ghost = icon.Plus, widget.Sends(PatchNew{Kind: "synth"}), true
	newPluck := widget.NewButton("New pluck")
	newPluck.Icon, newPluck.OnClick, newPluck.Ghost = icon.Plus, widget.Sends(PatchNew{Kind: "pluck"}), true
	pp.copy = widget.NewIconButton(icon.Copy, "Copy the patch")
	pp.load = widget.NewIconButton(icon.FolderOpen, "Load a patch from a file in this one's place")
	pp.save = widget.NewIconButton(icon.Save, "Save the patch to a file")
	importP := widget.NewIconButton(icon.FileInput, "Load a patch from a file as a new one")
	importP.OnClick = widget.Sends(PatchLoad{})
	pp.about = small("")
	gap := widget.NewSpacer()
	head := widget.Row(pp.picker, pp.about, gap, newSynth, newPluck, pp.copy, importP, pp.load, pp.save).Grow(gap, 1)
	head.Cross = widget.CrossCenter

	oscRow := make([]gunim.Node, maxOsc)
	for i := range pp.oscs {
		pp.oscs[i] = newOscSlot(pp, i)
		oscRow[i] = pp.oscs[i].sw
	}
	oscs := widget.Row(oscRow...)
	for _, n := range oscRow {
		oscs.Grow(n, 1)
	}
	oscs.Cross = widget.CrossStretch
	pp.oscRow = oscs

	pb := &pp.base
	pp.pluckKs = []*knob{
		newKnob("Decay", pb, "/Pluck/Decay", 0.05, 4, 1).logScale().units("s").unsetIs(1),
		newKnob("Bright", pb, "/Pluck/Bright", 0, 1, 0.6).unsetIs(0.6),
		newKnob("Body", pb, "/Pluck/Body", 0, 1, 0.3),
	}
	pluck := panel("STRING · plucked, as Karplus and Strong pluck one", knobs(pp.pluckKs...))
	pp.body = newSwitcher(oscs, pluck)

	pp.filter = newSelector(filterNames, func(i int) gunim.Intent {
		return SetValue{Path: pp.base + "/Filter/Type", Str: filters[i], IsStr: true}
	}).group(5)
	pp.resp = &response{base: pb}
	pp.fKnobs = []*knob{
		newKnob("Cutoff", pb, "/Filter/Cutoff", 20, 20000, 2000).logScale().unsetIs(20000),
		newKnob("Res", pb, "/Filter/Res", 0, 1, 0.1),
		newKnob("Env", pb, "/Filter/Env", -4, 6, 0).center().units(" oct"),
		newKnob("Key", pb, "/Filter/Key", 0, 1, 0),
		newKnob("Vel", pb, "/Filter/Vel", 0, 3, 0),
	}
	filter := panelWith(small("FILTER"), pp.filter, sized(pp.resp, 0, 70), knobs(pp.fKnobs...))
	envKnobs := func(rel string) []*knob {
		return []*knob{
			newKnob("Attack", pb, rel+"/Attack", 0.001, 4, 0.01).logScale().units("s"),
			newKnob("Decay", pb, rel+"/Decay", 0.005, 4, 0.3).logScale().units("s"),
			newKnob("Sustain", pb, rel+"/Sustain", 0, 1, 0.7),
			newKnob("Release", pb, rel+"/Release", 0.005, 6, 0.2).logScale().units("s"),
		}
	}
	pp.fenv = &envelope{base: pb, rel: "/FilterEnv", label: "filter", color: color.NRGBA{0xff, 0xcf, 0x5c, 0xff}}
	pp.feKnobs = envKnobs("/FilterEnv")
	pp.aenv = &envelope{base: pb, rel: "/Amp", label: "amp"}
	pp.aeKnobs = envKnobs("/Amp")
	fenv := panel("FILTER ENVELOPE", sized(pp.fenv, 0, 70), knobs(pp.feKnobs...))
	aenv := panel("AMP ENVELOPE", sized(pp.aenv, 0, 70), knobs(pp.aeKnobs...))
	mid := widget.Row(filter, fenv, aenv).Grow(filter, 1.25).Grow(fenv, 1).Grow(aenv, 1)
	mid.Cross = widget.CrossStretch

	lfoRow := make([]gunim.Node, 0, maxLFO+1)
	for i := range pp.lfos {
		pp.lfos[i] = newLFOSlot(pp, i)
		lfoRow = append(lfoRow, pp.lfos[i].sw)
	}
	pp.vKnobs = []*knob{
		newKnob("Voices", pb, "/Poly", 1, 16, 8).steps(1).unsetIs(8),
		newKnob("Glide", pb, "/Glide", 0, 0.5, 0).units("s"),
		newKnob("Drive", pb, "/Drive", 0, 1, 0),
		newKnob("Noise", pb, "/Noise", 0, 1, 0),
		newKnob("Drift", pb, "/Drift", 0, 20, 0).units("c"),
		newKnob("Accent", pb, "/Accent", 0, 4, 0).units(" oct"),
		newKnob("Gain", pb, "/Gain", 0, 2, 1).unsetIs(1),
	}
	pp.vowel = newSelector(vowels, func(i int) gunim.Intent {
		v := vowels[i]
		if v == "off" {
			v = ""
		}
		return SetValue{Path: pp.base + "/Vowel", Str: v, IsStr: true}
	})
	pp.vocoder = widget.NewButton("Vocoder")
	pp.vocoder.KeepFocus, pp.vocoder.Tooltip = true, "Sing the vowel through a vocoder's ten bands, as a Roland VP-330's: a robot's voice"
	pp.slide = widget.NewButton("Slide")
	pp.slide.KeepFocus, pp.slide.Tooltip = true, "Glide only into a note tied to the one before, as a TB-303 slides; a note after a rest jumps"
	voice := panelWith(small("VOICE · sings a vowel"), pp.vowel, widget.Row(pp.vocoder, pp.slide), knobs(pp.vKnobs[:4]...), knobs(pp.vKnobs[4:]...))
	pp.arpOn = widget.NewButton("On")
	pp.arpOn.KeepFocus, pp.arpOn.Tooltip = true, "Arpeggiate: steps through notes fast, as a Commodore 64 plays a chord on one voice"
	pp.arpChrd = widget.NewButton("Chords")
	pp.arpChrd.KeepFocus, pp.arpChrd.Tooltip = true, "Play a track's chords, ch, as the arpeggio"
	pp.arpKs = []*knob{newKnob("Rate", pb, "/Arpeggio/Hz", 5, 100, 50).units("Hz").unsetIs(50).steps(1)}
	pp.arpStep = widget.NewTextField()
	pp.arpStep.Face = widget.MonoFont
	pp.arpStep.Placeholder = "0 4 7"
	pp.arpStep.OnChange = func(s string, _ *gunim.UI) gunim.Intent { return ArpSteps{Patch: pp.name, Steps: s} }
	arp := panel("CHIP ARPEGGIO", widget.Row(pp.arpOn, pp.arpChrd), knobs(pp.arpKs...), pp.arpStep)
	pp.chipOn = widget.NewButton("Stepped")
	pp.chipOn.KeepFocus, pp.chipOn.Tooltip = true, "Step the level as a console does: in 16 steps, set 60 times a second"
	pp.chipKs = []*knob{
		newKnob("Levels", pb, "/Chip/Levels", 2, 32, 16).steps(1).unsetIs(16),
		newKnob("Frames", pb, "/Chip/Hz", 25, 120, 60).steps(1).units("Hz").unsetIs(60),
	}
	pp.bendKs = []*knob{
		newKnob("Bend", pb, "/Bend/Semis", -24, 24, 0).center().steps(1).units("st"),
		newKnob("Slide", pb, "/Bend/Time", 0, 0.5, 0).units("s"),
	}
	chip := panel("CHIP · NES and Game Boy", pp.chipOn, knobs(pp.chipKs...), knobs(pp.bendKs...))
	// The LFOs in a row of their own, and how the patch plays in one
	// under them.
	lfos := widget.Row(lfoRow...)
	for _, n := range lfoRow {
		lfos.Grow(n, 1)
	}
	lfos.Cross = widget.CrossStretch
	plays := widget.Row(voice, arp, chip).Grow(voice, 1.4).Grow(arp, 1).Grow(chip, 1)
	plays.Cross = widget.CrossStretch
	low := widget.Column(lfos, plays)
	low.Cross = widget.CrossStretch

	pp.note = &wave{spans: true, label: "a note of C4, held and let go", fit: true}
	pp.cycle = &wave{label: "its cycles, close up", fit: true}
	pp.live = &wave{label: "", fit: true, color: color.NRGBA{0xff, 0xcf, 0x5c, 0xff}}
	pp.keys = &keyboard{low: 48, octaves: 3}
	pp.keys.play = func(pitch int, u *gunim.UI) { u.Send(pp.keys, Audition{Patch: pp.name, Pitch: pitch}) }
	readouts := widget.Row(pp.note, pp.cycle, pp.live).Grow(pp.note, 1.3).Grow(pp.cycle, 1).Grow(pp.live, 1)
	readouts.Cross = widget.CrossStretch
	out := panel("OUTPUT · play the keys to hear the patch over the song", sized(readouts, 0, 96), sized(pp.keys, 0, 70))

	pp.lib = newLibraryBar()
	col := widget.Column(head, pp.lib, pp.body, mid, low, out)
	col.Cross = widget.CrossStretch
	// The track playing the patch, its strip as the mixer's, and a step
	// to the track before or after.
	pp.strip = newChannel()
	pp.trackName = small("")
	pp.trackName.MaxLines = 1
	prev := widget.NewIconButton(icon.ChevronLeft, "The track before: edit its patch")
	prev.OnClick = func(*gunim.UI) gunim.Intent { return pp.stepTrack(-1) }
	next := widget.NewIconButton(icon.ChevronRight, "The next track: edit its patch")
	next.OnClick = func(*gunim.UI) gunim.Intent { return pp.stepTrack(1) }
	nav := widget.Row(prev, pp.trackName, next).Grow(pp.trackName, 1)
	nav.Cross = widget.CrossCenter
	pp.stripSw = newSwitcher(widget.NewSized(pp.strip, 0, 750), small("No track plays this patch."))
	left := widget.Column(small("TRACK"), nav, pp.stripSw)
	left.Cross = widget.CrossStretch
	pp.tree = newPresetTree(false)
	shelf := widget.Column(small("PATCH LIBRARY"), widget.NewSized(widget.NewScroll(pp.tree), 0, 720))
	shelf.Cross = widget.CrossStretch
	page := widget.Row(widget.NewSized(left, 124, 0), widget.NewSized(shelf, 196, 0), col).Grow(col, 1)
	page.Cross = widget.CrossStart
	pp.Scroll = widget.NewScroll(widget.NewPad(page))
	return pp
}

// choose shows the patch named name.
func (pp *patchPane) choose(name string) {
	if name == pp.name {
		return
	}
	pp.name = name
	pp.base = "Patches/" + name
}

// update shows st.
func (pp *patchPane) update(st Studio) {
	song := st.Doc
	if song == nil {
		return
	}
	pp.song = song
	var names []string
	for _, n := range sortedNames(song.Patches) {
		if song.Patches[n].Kind != "drums" {
			names = append(names, n)
		}
	}
	if !slices.Equal(names, pp.names) {
		pp.names = names
		pp.picker.SetItems(widget.Labels(names...))
		if len(names) == 0 {
			pp.picker.SetItems(widget.Labels("no patches"))
		}
	}
	if _, ok := song.Patches[pp.name]; !ok && len(names) > 0 {
		pp.choose(names[0])
	}
	pp.picker.SetSelected(max(slices.Index(names, pp.name), 0), nil)
	p := song.Patches[pp.name]
	if p == nil {
		return
	}
	pp.lib.update(st, pp.name)
	pp.tree.update(st, pp.name)
	pp.showTrack(st)
	var users []string
	for _, t := range song.Tracks {
		if t.Patch == pp.name {
			users = append(users, t.Name)
		}
	}
	kind := "synth"
	if p.Kind == "pluck" {
		kind = "plucked string"
	}
	about := kind + " · played by nothing yet"
	if len(users) > 0 {
		about = fmt.Sprintf("%s · played by %s", kind, joinNames(users))
	}
	pp.about.Text = about
	pp.copy.OnClick, pp.load.OnClick, pp.save.OnClick = widget.Sends(PatchCopy{Name: pp.name}), widget.Sends(PatchLoad{Name: pp.name}), widget.Sends(PatchSave{Name: pp.name})
	pp.body.which = 0
	if p.Kind == "pluck" {
		pp.body.which = 1
	}
	for i, o := range pp.oscs {
		o.update(song, p)
		// An empty slot takes less room than an oscillator, and only the
		// first offers to add one.
		w := float32(1)
		switch {
		case i == len(p.Osc):
			w = 0.45
		case i > len(p.Osc):
			w = 0.001
		}
		pp.oscRow.Grow(o.sw, w)
	}
	for _, l := range pp.lfos {
		l.update(song, p)
	}
	showKnobs(song, pp.pluckKs...)
	showKnobs(song, pp.fKnobs...)
	showKnobs(song, pp.feKnobs...)
	showKnobs(song, pp.aeKnobs...)
	showKnobs(song, pp.vKnobs...)
	showKnobs(song, pp.arpKs...)
	showKnobs(song, pp.chipKs...)
	showKnobs(song, pp.bendKs...)
	pp.chipOn.Active = p.Chip != nil
	pp.chipOn.OnClick = widget.Sends(ToggleValue{Path: pp.base + "/Chip", Seed: pp.base + "/Chip/Levels", Num: 16})
	a := p.Arpeggio
	pp.arpOn.Active = a != nil
	pp.arpChrd.Active = a != nil && a.Chord
	pp.arpOn.OnClick = widget.Sends(ArpSet{Patch: pp.name})
	pp.arpChrd.OnClick = widget.Sends(ArpSet{Patch: pp.name, Chord: true})
	pp.arpChrd.Disabled = a == nil
	if pp.arpGen != st.Gen {
		pp.arpGen = st.Gen
		var steps []string
		if a != nil {
			for _, s := range a.Steps {
				steps = append(steps, strconv.Itoa(s))
			}
		}
		pp.arpStep.SetText(strings.Join(steps, " "), nil)
	}
	ft := p.Filter.Type
	if ft == "" {
		ft = "none"
	}
	pp.filter.SetSelected(segmentedIndex(filters, ft))
	vw := p.Vowel
	if vw == "" {
		vw = "off"
	}
	pp.vowel.SetSelected(segmentedIndex(vowels, vw))
	pp.vocoder.Active = p.Vocoder
	pp.vocoder.OnClick = widget.Sends(ToggleValue{Path: pp.base + "/Vocoder"})
	pp.slide.Active = p.Slide
	pp.slide.OnClick = widget.Sends(ToggleValue{Path: pp.base + "/Slide"})
	pp.resp.song, pp.fenv.song, pp.aenv.song = song, song, song
	if st.Preview.Patch == pp.name {
		pp.note.data, pp.cycle.data = st.Preview.Wave, st.Preview.Cycle
	}
	pp.live.data = st.Scope
	pp.live.label = "the track " + st.ScopeTrack + ", as it plays"
	if st.ScopeTrack == "" {
		pp.live.label = "no track plays it"
	}
	pp.keys.lit = pp.keys.lit[:0]
	heard := heardAt(st.Clock, st.Clock.At)
	for _, n := range st.Notes {
		if n.Drum || n.Track < 0 || n.Track >= len(st.Tracks) || st.Tracks[n.Track].Patch != pp.name {
			continue
		}
		if float64(n.Frame) <= heard && heard < float64(n.Frame+n.Len) {
			pp.keys.lit = append(pp.keys.lit, n.Pitch)
		}
	}
}

// oscSlot is one of a patch editor's oscillators: its wave, drawn, and
// its knobs, or a button to add one.
type oscSlot struct {
	pp     *patchPane
	i      int
	base   string
	sw     *switcher
	wave   *selector
	shape  *wave
	ks     []*knob
	extra  *switcher
	width  []*knob
	fm     []*knob
	remove *widget.IconButton
	add    *widget.Button
	sync   *widget.Button
	ring   *widget.Button
	table  *tableEdit
	look   *switcher
}

func newOscSlot(pp *patchPane, i int) *oscSlot {
	o := &oscSlot{pp: pp, i: i}
	b := &o.base
	o.wave = newSelector(waveNames, func(k int) gunim.Intent {
		return SetValue{Path: o.base + "/Wave", Str: waves[k], IsStr: true}
	}).group(6)
	o.remove = widget.NewIconButton(icon.X, "Take this oscillator out")
	o.shape = &wave{}
	// A click on the wave drawn turns to the next wave.
	o.shape.click = func() gunim.Intent {
		return SetValue{Path: o.base + "/Wave", Str: waves[(o.wave.Selected()+1)%len(waves)], IsStr: true}
	}
	o.ks = []*knob{
		newKnob("Octave", b, "/Octave", -3, 3, 0).steps(1).center(),
		newKnob("Semi", b, "/Semi", -12, 12, 0).steps(1).center(),
		newKnob("Fine", b, "/Detune", -50, 50, 0).center().units("c"),
		newKnob("Level", b, "/Level", 0, 1, 0.7).unsetIs(1),
	}
	unison := []*knob{
		newKnob("Voices", b, "/Unison", 1, 9, 1).steps(1).unsetIs(1),
		newKnob("Spread", b, "/Spread", 0, 60, 0).units("c"),
	}
	o.width = []*knob{newKnob("Width", b, "/Width", 0.05, 0.95, 0.5).unsetIs(0.5)}
	o.fm = []*knob{
		newKnob("Ratio", b, "/Ratio", 0.5, 8, 1).steps(0.01).unsetIs(1),
		newKnob("Index", b, "/Index", 0, 8, 1),
		newKnob("Fall", b, "/Decay", 0.005, 2, 0.2).logScale().units("s"),
	}
	o.ks = append(o.ks, unison...)
	o.extra = newSwitcher(knobs(o.width...), knobs(o.fm...))
	o.sync = widget.NewButton("Sync")
	o.sync.KeepFocus, o.sync.Ghost, o.sync.Tooltip = true, true, "Restart with each cycle of the oscillator before, as the SID's hard sync"
	o.ring = widget.NewButton("Ring")
	o.ring.KeepFocus, o.ring.Ghost, o.ring.Tooltip = true, true, "Turn over with each half cycle of the oscillator before, as the SID's ring modulation"
	head := titled(fmt.Sprintf("OSC %d", i+1), o.remove)
	links := widget.Row(o.sync, o.ring)
	o.table = &tableEdit{}
	o.table.set = func(steps []int) gunim.Intent { return SetInts{Path: o.base + "/Table", Values: steps} }
	o.look = newSwitcher(o.shape, o.table)
	editor := panelWith(head, o.wave, sized(o.look, 0, 48), links, knobs(o.ks[:4]...), widget.Row(knobs(unison...), o.extra))
	o.add = widget.NewButton("Add")
	o.add.Tooltip = "Add an oscillator"
	o.add.Icon, o.add.Ghost = icon.Plus, true
	empty := widget.NewCard(widget.Column(small(fmt.Sprintf("OSC %d", i+1)), o.add))
	empty.Fill = panelFill
	// Past the first empty slot, a slot shows nothing.
	o.sw = newSwitcher(editor, empty, widget.NewSpacer())
	return o
}

func (o *oscSlot) update(song *synth.Song, p *synth.Patch) {
	o.base = fmt.Sprintf("%s/Osc/%d", o.pp.base, o.i)
	o.remove.OnClick = widget.Sends(RemoveItem{Path: o.pp.base + "/Osc", Index: o.i})
	o.add.OnClick = widget.Sends(AddItem{Path: o.pp.base + "/Osc"})
	switch {
	case o.i < len(p.Osc):
		o.sw.which = 0
	default:
		// Only the first empty slot offers to add one.
		o.sw.which = 1
		if o.i > len(p.Osc) {
			o.sw.which = 2
		}
		return
	}
	osc := p.Osc[o.i]
	w := osc.Wave
	if w == "square" {
		w = "pulse"
	}
	if w == "" {
		w = "saw"
	}
	o.wave.SetSelected(segmentedIndex(waves, w))
	o.shape.data = synth.OscCycle(osc, 96)
	o.look.which = 0
	if osc.Wave == "gbwave" {
		o.look.which = 1
		o.table.show(osc.Table)
	}
	o.sync.Active, o.ring.Active = osc.Sync, osc.Ring
	o.sync.OnClick = widget.Sends(SetValue{Path: o.base + "/Sync", Num: boolNum(!osc.Sync)})
	o.ring.OnClick = widget.Sends(SetValue{Path: o.base + "/Ring", Num: boolNum(!osc.Ring)})
	o.sync.Disabled, o.ring.Disabled = o.i == 0, o.i == 0
	o.shape.label = w
	if osc.Unison > 1 {
		o.shape.label = fmt.Sprintf("%s ×%d", w, osc.Unison)
	}
	o.extra.which = 0
	if w == "fm" {
		o.extra.which = 1
	}
	showKnobs(song, o.ks...)
	showKnobs(song, o.width...)
	showKnobs(song, o.fm...)
}

// lfoSlot is one of a patch editor's LFOs, or a button to add one.
type lfoSlot struct {
	pp     *patchPane
	i      int
	base   string
	sw     *switcher
	target *selector
	wave   *selector
	ks     []*knob
	remove *widget.IconButton
	add    *widget.Button
}

func newLFOSlot(pp *patchPane, i int) *lfoSlot {
	l := &lfoSlot{pp: pp, i: i}
	b := &l.base
	l.target = newSelector(lfoTargets, func(k int) gunim.Intent {
		return SetValue{Path: l.base + "/To", Str: lfoTargets[k], IsStr: true}
	})
	l.wave = newSelector(lfoWaves, func(k int) gunim.Intent {
		return SetValue{Path: l.base + "/Wave", Str: lfoWaves[k], IsStr: true}
	})
	l.remove = widget.NewIconButton(icon.X, "Take this LFO out")
	l.ks = []*knob{
		newKnob("Rate", b, "/Hz", 0.05, 20, 2).logScale().units("Hz"),
		newKnob("Beats", b, "/Beats", 0, 16, 0).steps(0.25),
		newKnob("Depth", b, "/Depth", 0, 4, 0.5),
		newKnob("Delay", b, "/Delay", 0, 2, 0).units("s"),
	}
	head := titled(fmt.Sprintf("LFO %d", i+1), l.remove)
	moves := widget.Column(small("MOVES"), l.target)
	shape := widget.Column(small("WAVE"), l.wave)
	pick := widget.Row(moves, shape).Grow(moves, 1).Grow(shape, 1)
	pick.Cross = widget.CrossStart
	editor := panelWith(head, pick, knobs(l.ks...))
	l.add = widget.NewButton("Add an LFO")
	l.add.Icon, l.add.Ghost = icon.Plus, true
	empty := widget.NewCard(widget.Column(small(fmt.Sprintf("LFO %d", i+1)), l.add))
	empty.Fill = panelFill
	l.sw = newSwitcher(editor, empty)
	return l
}

func (l *lfoSlot) update(song *synth.Song, p *synth.Patch) {
	l.base = fmt.Sprintf("%s/LFO/%d", l.pp.base, l.i)
	l.remove.OnClick = widget.Sends(RemoveItem{Path: l.pp.base + "/LFO", Index: l.i})
	l.add.OnClick = widget.Sends(AddItem{Path: l.pp.base + "/LFO"})
	if l.i >= len(p.LFO) {
		l.sw.which = 1
		l.add.Disabled = l.i > len(p.LFO)
		return
	}
	l.sw.which = 0
	lf := p.LFO[l.i]
	l.target.SetSelected(segmentedIndex(lfoTargets, lf.To))
	w := lf.Wave
	if w == "rand" {
		w = "random"
	}
	l.wave.SetSelected(segmentedIndex(lfoWaves, w))
	showKnobs(song, l.ks...)
}

// keyboard is a piano's keys, octaves of them from low, which play a
// note as they are pressed, and light as a track plays them.
type keyboard struct {
	low, octaves int
	play         func(pitch int, u *gunim.UI)
	lit          []int
	down         int
	size         geom.Size
}

func (k *keyboard) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	k.size = c.Max
	return c.Max
}

var blackKey = [12]bool{1: true, 3: true, 6: true, 8: true, 10: true}

// keyAt returns the key at x along the keyboard, at y down it.
func (k *keyboard) keyAt(p geom.Point) int {
	whites := 7 * k.octaves
	ww := k.size.W / float32(whites)
	if p.Y < k.size.H*0.6 {
		for n := k.low; n < k.low+12*k.octaves; n++ {
			if blackKey[n%12] && k.keyRect(n, ww).Contains(p) {
				return n
			}
		}
	}
	wi := int(p.X / ww)
	n := k.low
	for i := 0; ; n++ {
		if !blackKey[n%12] {
			if i == wi {
				return n
			}
			i++
		}
		if n > k.low+12*k.octaves {
			return -1
		}
	}
}

// keyRect returns where key n lies, the white keys ww wide.
func (k *keyboard) keyRect(n int, ww float32) geom.Rect {
	wi := 0
	for m := k.low; m < n; m++ {
		if !blackKey[m%12] {
			wi++
		}
	}
	if blackKey[n%12] {
		return geom.Rc(float32(wi)*ww-ww*0.3, 0, ww*0.6, k.size.H*0.6)
	}
	return geom.Rc(float32(wi)*ww, 0, ww, k.size.H)
}

func (k *keyboard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ww := box.W / float32(7*k.octaves)
	accent := widget.Accent.Get(f.Theme)
	for pass := range 2 {
		for n := k.low; n < k.low+12*k.octaves; n++ {
			black := blackKey[n%12]
			if black != (pass == 1) {
				continue
			}
			r := k.keyRect(n, ww)
			on := n == k.down || slices.Contains(k.lit, n)
			if black {
				c := color.NRGBA{0x1a, 0x1c, 0x22, 0xff}
				if on {
					c = accent
				}
				p.ShadowRRect(geom.Rc(r.Min.X, r.Min.Y, r.Size().W, r.Size().H), 3, paint.Solid(c),
					paint.Shadow{Offset: geom.Pt(0, 2), Blur: 3, Color: color.NRGBA{A: 0x90}})
				continue
			}
			c := color.NRGBA{0xe8, 0xea, 0xf0, 0xff}
			if on {
				c = lighten(accent, 0.3)
			}
			p.RRect(geom.Rc(r.Min.X+1, r.Min.Y, r.Size().W-2, r.Size().H), 4, paint.Solid(c))
			if n%12 == 0 {
				run := audioui.Shaped("C"+strconv.Itoa(n/12-1), 9.5, false, false)
				run.Paint(p, geom.Pt(r.Min.X+(r.Size().W-run.Advance)/2, r.Max.Y-run.Height()-3), color.NRGBA{0x50, 0x55, 0x60, 0xff})
			}
		}
	}
}

func (k *keyboard) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if n := k.keyAt(e.Pos); n >= 0 && k.play != nil {
			k.down = n
			k.play(n, u)
			u.Invalidate()
		}
		return true
	case input.PointerMove:
		if k.down > 0 {
			if n := k.keyAt(e.Pos); n >= 0 && n != k.down {
				k.down = n
				k.play(n, u)
				u.Invalidate()
			}
			return true
		}
	case input.PointerUp:
		k.down = 0
		u.Invalidate()
		return true
	}
	return false
}

// DragsTouch says a finger slides along the keys rather than scrolling.
func (k *keyboard) DragsTouch() bool { return true }

// sortedNames returns a map's keys, in order.
func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// joinNames writes names as a list a person reads: a, b and c.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// stepTrack moves to the track by places from the one shown, of those
// playing a synth or a pluck, round from the last to the first, and
// shows its patch.
func (pp *patchPane) stepTrack(by int) gunim.Intent {
	n := len(pp.tracks)
	if n == 0 || pp.song == nil {
		return nil
	}
	i := slices.Index(pp.tracks, pp.track)
	if i < 0 {
		i = 0
		if by < 0 {
			i = 1
		}
	}
	pp.track = pp.tracks[((i+by)%n+n)%n]
	if t := track(pp.song, pp.track); t != nil {
		pp.choose(t.Patch)
	}
	return pp.changed(pp.name)
}

// showTrack shows the strip of the track playing the patch: the one
// stepped to, or else the first that plays it.
func (pp *patchPane) showTrack(st Studio) {
	song := st.Doc
	pp.tracks = pp.tracks[:0]
	first := ""
	for _, t := range song.Tracks {
		if p := song.Patches[t.Patch]; p != nil && p.Kind != "drums" {
			pp.tracks = append(pp.tracks, t.Name)
		}
		if t.Patch == pp.name && first == "" {
			first = t.Name
		}
	}
	if t := track(song, pp.track); t == nil || t.Patch != pp.name {
		pp.track = first
	}
	pp.trackName.Text = pp.track
	pp.stripSw.which = 1
	for i, r := range st.Tracks {
		if r.Name == pp.track {
			pp.strip.update(st, r, i, nil)
			pp.stripSw.which = 0
		}
	}
	if pp.track == "" {
		pp.trackName.Text = "none"
	}
}
