package plugsynth

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// The patch editor edits the patch the state holds: every field of
// it, on cards as the panel's sections are, each change an edit of the
// state for the host to keep and undo. A knob's turn is one edit; a
// click on a switch, a wave or a type is one.

// The choices the editor offers, as a patch writes them, and as it
// shows them.
var (
	waveIDs      = []string{"saw", "pulse", "tri", "sine", "fm", "noise", "glottal", "sawtri", "pulsetri", "pulsesaw", "nespulse", "nestri", "nesnoise", "nesmetal", "gbwave"}
	waveNames    = []string{"Saw", "Pulse", "Triangle", "Sine", "FM", "SID noise", "Voice", "Saw·tri", "Pulse·tri", "Pulse·saw", "NES pulse", "NES tri", "NES noise", "NES metal", "GB wave"}
	filterIDs    = []string{"none", "lp", "lp24", "hp", "bp", "sidlp", "sidbp", "sidhp", "sidnotch"}
	filterNames  = []string{"Off", "Low 12", "Low 24", "High", "Band", "SID low", "SID band", "SID high", "SID notch"}
	lfoTargetIDs = []string{"pitch", "cutoff", "amp", "width", "pan"}
	lfoTargets   = []string{"Pitch", "Cutoff", "Level", "Width", "Pan"}
	lfoWaveIDs   = []string{"sine", "tri", "saw", "square", "random"}
	lfoWaves     = []string{"Sine", "Triangle", "Saw", "Square", "Random"}
	singerIDs    = []string{"", "bass", "baritone", "tenor", "alto", "soprano"}
	singerNames  = []string{"Formants", "Bass", "Baritone", "Tenor", "Alto", "Soprano"}
	vowelIDs     = []string{"", "a", "e", "i", "o", "u"}
)

// The most oscillators and LFOs the editor gives a patch.
const (
	maxOscs = 4
	maxLFOs = 2
)

// patchEditor is the patch editor: a flow of cards, rebuilt as the
// patch's shape changes, its controls following its values otherwise.
type patchEditor struct {
	*widget.Scroll
	p    *plugin
	h    plug.Host
	flow *flow
	// pt is the patch shown, the editor's own copy, and st the state it
	// came from.
	pt *synth.Patch
	st State
	// shape names the patch's shape as the cards were built for it.
	shape string
	// follows set each control from pt, as an undo or a pick moves it.
	follows []func(u *gunim.UI)
	// held says a knob's turn is under way.
	held bool
	// edited tells the panel the state changed.
	edited func(u *gunim.UI)
}

func newPatchEditor(p *plugin, h plug.Host, edited func(*gunim.UI)) *patchEditor {
	e := &patchEditor{p: p, h: h, flow: &flow{}, edited: edited}
	e.Scroll = widget.NewScroll(e.flow)
	e.load(nil)
	return e
}

// load shows the patch the plugin plays: the cards built again where
// its shape changed, or each control set to its value.
func (e *patchEditor) load(u *gunim.UI) {
	e.st = e.p.sound()
	e.pt = clonePatch(e.st.Patch)
	if s := shapeOf(e.pt); s != e.shape || e.flow.cards == nil {
		e.shape = s
		e.build()
		if u != nil {
			u.Invalidate()
		}
		return
	}
	if u == nil {
		return
	}
	for _, f := range e.follows {
		f(u)
	}
	u.Invalidate()
}

// FollowState follows the state set elsewhere, as an undo, unless a
// knob's turn is under way.
func (e *patchEditor) FollowState(u *gunim.UI) {
	if !e.held {
		e.load(u)
	}
}

// shapeOf names what the cards show of pt: its kind, its oscillators'
// waves, its LFOs, the switches that bring in cards, and its drums.
func shapeOf(pt *synth.Patch) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%d|%d|%t|%t|", pt.Kind, len(pt.Osc), len(pt.LFO), pt.Chip != nil, pt.Arpeggio != nil)
	for _, o := range pt.Osc {
		b.WriteString(o.Wave + ",")
	}
	b.WriteString("|")
	for _, n := range kitNames(pt) {
		b.WriteString(n + ",")
	}
	return b.String()
}

// begin starts an edit named name, as a knob's turn starts.
func (e *patchEditor) begin(name string, u *gunim.UI) {
	e.held = true
	e.h.BeginEdit(name, u)
}

// change changes the patch by fn, and has the plugin play it, telling
// the host. A patch the plugin turns away, as one that would fault, is
// left as the plugin plays.
func (e *patchEditor) change(fn func(pt *synth.Patch), u *gunim.UI) {
	fn(e.pt)
	st := State{Bank: e.st.Bank, Group: e.st.Group, Name: e.st.Name, Patch: e.pt}
	if err := e.p.SetState(st.Encode()); err != nil {
		e.pt = clonePatch(e.p.sound().Patch)
		return
	}
	e.h.Changed(u)
	if e.edited != nil {
		e.edited(u)
	}
}

// end ends the edit.
func (e *patchEditor) end(u *gunim.UI) {
	e.held = false
	e.h.EndEdit(u)
}

// edit makes fn one edit named name, and builds the cards again where
// it changed the patch's shape.
func (e *patchEditor) edit(name string, fn func(pt *synth.Patch), u *gunim.UI) {
	e.begin(name, u)
	e.change(fn, u)
	e.end(u)
	e.load(u)
}

// knobSpec is a knob of a field of the patch: what it shows, and how it
// reads and writes the field.
type knobSpec struct {
	prm plug.Param
	get func(pt *synth.Patch) float64
	set func(pt *synth.Patch, v float64)
}

// k describes a knob named name from lo to hi, its default def.
func k(name string, lo, hi, def float64, get func(*synth.Patch) float64, set func(*synth.Patch, float64)) knobSpec {
	return knobSpec{prm: plug.Param{Name: name, Min: lo, Max: hi, Default: def}, get: get, set: set}
}

// logScale runs the knob by ratios.
func (s knobSpec) logScale() knobSpec { s.prm.Log = true; return s }

// steps runs the knob in n steps.
func (s knobSpec) steps(n int) knobSpec { s.prm.Steps = n; return s }

// shows writes the knob's value with f.
func (s knobSpec) shows(f func(float64) string) knobSpec { s.prm.Format = f; return s }

// The ways a knob writes its value.
func secs(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.0f ms", v*1000)
	}
	return fmt.Sprintf("%.2f s", v)
}

func hertz(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.1f kHz", v/1000)
	}
	return fmt.Sprintf("%.0f Hz", v)
}

func plain(v float64) string    { return fmt.Sprintf("%.2f", v) }
func whole(v float64) string    { return strconv.Itoa(int(math.Round(v))) }
func cents(v float64) string    { return fmt.Sprintf("%+.0f c", v) }
func octaves(v float64) string  { return fmt.Sprintf("%+.1f oct", v) }
func semis(v float64) string    { return fmt.Sprintf("%+.0f st", v) }
func decibels(v float64) string { return fmt.Sprintf("%+.1f dB", v) }
func lfoHz(v float64) string    { return fmt.Sprintf("%.2f Hz", v) }

// or returns v, or def where v is zero, as a patch leaves a field unset.
func or(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// knob returns a knob of s, each turn one edit.
func (e *patchEditor) knob(s knobSpec) *plugui.Knob {
	if s.prm.Format == nil {
		s.prm.Format = plain
	}
	kn := plugui.NewKnobFunc(s.prm, s.prm.Clamp(s.get(e.pt)),
		func(u *gunim.UI) { e.begin("Turn "+strings.ToLower(s.prm.Name), u) },
		func(v float64, u *gunim.UI) { e.change(func(pt *synth.Patch) { s.set(pt, v) }, u) },
		func(u *gunim.UI) { e.end(u) })
	kn.Compact = true
	e.follows = append(e.follows, func(u *gunim.UI) { kn.Follow(s.prm.Clamp(s.get(e.pt)), u) })
	return kn
}

// card returns a card named name of knobs.
func (e *patchEditor) card(name string, specs ...knobSpec) *plugui.Section {
	sec := &plugui.Section{Name: name}
	for _, s := range specs {
		sec.Knobs = append(sec.Knobs, e.knob(s))
	}
	return sec
}

// pick returns a drop-down of names, the one get gives chosen, which
// set changes to, as one edit named name.
func (e *patchEditor) pick(name string, names []string, get func(*synth.Patch) int, set func(*synth.Patch, int)) *widget.Dropdown {
	d := widget.NewDropdown(widget.Labels(names...))
	d.Label = name
	d.SetSelected(max(get(e.pt), 0), nil)
	d.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		e.edit("Pick "+strings.ToLower(name), func(pt *synth.Patch) { set(pt, i) }, u)
		return nil
	}
	e.follows = append(e.follows, func(u *gunim.UI) { d.SetSelected(max(get(e.pt), 0), u) })
	return d
}

// toggle returns a button that turns a switch of the patch on and off,
// as one edit.
func (e *patchEditor) toggle(label, tip string, get func(*synth.Patch) bool, set func(*synth.Patch, bool)) *widget.Button {
	b := widget.NewButton(label)
	b.KeepFocus, b.Tooltip, b.Active = true, tip, get(e.pt)
	b.OnClick = func(u *gunim.UI) gunim.Intent {
		on := !get(e.pt)
		e.edit(label, func(pt *synth.Patch) { set(pt, on) }, u)
		return nil
	}
	e.follows = append(e.follows, func(*gunim.UI) { b.Active = get(e.pt) })
	return b
}

// button returns a ghost button of icon ic that makes one edit.
func (e *patchEditor) button(ic *icon.Icon, tip string, fn func(pt *synth.Patch)) *widget.IconButton {
	b := widget.NewIconButton(ic, tip)
	b.OnClick = func(u *gunim.UI) gunim.Intent {
		e.edit(tip, fn, u)
		return nil
	}
	return b
}

// text returns a field of words the patch holds, as its vowels, each
// change one edit; a change that fails check is kept back until it
// passes.
func (e *patchEditor) text(name, hint string, get func(*synth.Patch) string, set func(*synth.Patch, string), check func(string) bool) *widget.TextField {
	f := widget.NewTextField()
	f.Placeholder = hint
	f.Face = widget.MonoFont
	f.SetText(get(e.pt), nil)
	f.OnChange = func(s string, u *gunim.UI) gunim.Intent {
		if check(s) {
			e.begin("Type "+strings.ToLower(name), u)
			e.change(func(pt *synth.Patch) { set(pt, s) }, u)
			e.end(u)
		}
		return nil
	}
	e.follows = append(e.follows, func(u *gunim.UI) {
		if get(e.pt) != f.Text() && check(f.Text()) {
			f.SetText(get(e.pt), u)
		}
	})
	return f
}

// column stacks nodes in a column, as one control of a card.
func column(kids ...gunim.Node) gunim.Node {
	c := widget.Column(kids...)
	c.Cross = widget.CrossStretch
	return c
}

// build makes the cards for the patch's shape.
func (e *patchEditor) build() {
	e.follows = nil
	var cards []*plugui.Section
	switch e.pt.Kind {
	case "drums":
		cards = e.kitCards()
	case "pluck":
		cards = append(cards, e.kindCard(), e.stringCard())
	default:
		cards = append(cards, e.kindCard())
		cards = append(cards, e.oscCards()...)
	}
	if e.pt.Kind != "drums" {
		cards = append(cards, e.filterCards()...)
		cards = append(cards, e.envCard("AMP ENVELOPE", func(pt *synth.Patch) *synth.Env { return &pt.Amp }))
		cards = append(cards, e.lfoCards()...)
		cards = append(cards, e.voiceCard(), e.playCard(), e.answerCard("MOD WHEEL", false), e.answerCard("PRESSURE", true),
			e.chipCard(), e.arpCard())
	}
	e.flow.cards = cards
}

// kindCard picks what makes the sound: oscillators or a plucked string;
// the patch's level and voices stand beside it.
func (e *patchEditor) kindCard() *plugui.Section {
	sec := e.card("SOUND",
		k("Gain", 0, 2, 1, func(pt *synth.Patch) float64 { return or(pt.Gain, 1) }, func(pt *synth.Patch, v float64) { pt.Gain = v }),
		k("Voices", 1, 16, 8, func(pt *synth.Patch) float64 { return or(float64(pt.Poly), 8) },
			func(pt *synth.Patch, v float64) { pt.Poly = int(math.Round(v)) }).steps(15).shows(whole))
	kinds := plugui.NewSelector("Made of", []string{"Oscillators", "String"},
		[]string{"Oscillators, layered, through a filter", "A plucked string, as Karplus and Strong pluck one"})
	get := func(pt *synth.Patch) int {
		if pt.Kind == "pluck" {
			return 1
		}
		return 0
	}
	kinds.SetSelected(get(e.pt), nil)
	kinds.OnPick = func(i int, u *gunim.UI) {
		e.edit("Pick what it is made of", func(pt *synth.Patch) {
			pt.Kind = []string{"synth", "pluck"}[i]
			if i == 0 && len(pt.Osc) == 0 && pt.Noise == 0 {
				pt.Osc = []synth.Osc{{Wave: "saw"}}
			}
		}, u)
	}
	return sec.Add(kinds, 64)
}

// oscCards are a card for each oscillator, and one to add one.
func (e *patchEditor) oscCards() []*plugui.Section {
	var out []*plugui.Section
	for i := range e.pt.Osc {
		out = append(out, e.oscCard(i))
	}
	if len(e.pt.Osc) < maxOscs {
		add := e.button(icon.Plus, "Add an oscillator", func(pt *synth.Patch) {
			pt.Osc = append(pt.Osc, synth.Osc{Wave: "saw"})
		})
		out = append(out, (&plugui.Section{Name: "OSCILLATOR"}).Add(add, 40))
	}
	return out
}

// oscCard is oscillator i's card: its wave, drawn, and its knobs.
func (e *patchEditor) oscCard(i int) *plugui.Section {
	o := func(pt *synth.Patch) *synth.Osc { return &pt.Osc[i] }
	f := func(name string, lo, hi, def float64, at func(*synth.Osc) *float64) knobSpec {
		return k(name, lo, hi, def, func(pt *synth.Patch) float64 { return or(*at(o(pt)), def) },
			func(pt *synth.Patch, v float64) { *at(o(pt)) = v })
	}
	n := func(name string, lo, hi, def float64, at func(*synth.Osc) *int) knobSpec {
		return k(name, lo, hi, def, func(pt *synth.Patch) float64 { return or(float64(*at(o(pt))), def) },
			func(pt *synth.Patch, v float64) { *at(o(pt)) = int(math.Round(v)) }).steps(int(hi - lo)).shows(whole)
	}
	specs := []knobSpec{
		n("Octave", -3, 3, 0, func(x *synth.Osc) *int { return &x.Octave }),
		f("Semi", -12, 12, 0, func(x *synth.Osc) *float64 { return &x.Semi }).steps(24).shows(semis),
		f("Fine", -50, 50, 0, func(x *synth.Osc) *float64 { return &x.Detune }).shows(cents),
		f("Level", 0, 1, 1, func(x *synth.Osc) *float64 { return &x.Level }),
		n("Copies", 1, 9, 1, func(x *synth.Osc) *int { return &x.Unison }),
		f("Spread", 0, 60, 0, func(x *synth.Osc) *float64 { return &x.Spread }).shows(func(v float64) string { return fmt.Sprintf("%.0f c", v) }),
	}
	wave := e.pt.Osc[i].Wave
	switch wave {
	case "fm":
		specs = append(specs,
			f("Ratio", 0.5, 8, 1, func(x *synth.Osc) *float64 { return &x.Ratio }),
			f("Index", 0, 8, 0, func(x *synth.Osc) *float64 { return &x.Index }),
			f("Fall", 0.005, 2, 0.2, func(x *synth.Osc) *float64 { return &x.Decay }).logScale().shows(secs),
			f("Held", 0, 1, 0, func(x *synth.Osc) *float64 { return &x.Sustain }))
	case "pulse", "square", "nespulse", "sawtri", "pulsetri", "pulsesaw":
		specs = append(specs, f("Width", 0.05, 0.95, 0.5, func(x *synth.Osc) *float64 { return &x.Width }))
	}
	sec := e.card(fmt.Sprintf("OSC %d", i+1), specs...)
	picked := func(pt *synth.Patch) int {
		w := pt.Osc[i].Wave
		switch w {
		case "":
			w = "saw"
		case "square":
			w = "pulse"
		}
		return slices.Index(waveIDs, w)
	}
	dd := e.pick("Wave", waveNames, picked, func(pt *synth.Patch, k int) { pt.Osc[i].Wave = waveIDs[k] })
	shape := &waveShape{e: e, i: i}
	var look gunim.Node = shape
	if wave == "gbwave" {
		look = &tableEdit{e: e, i: i}
	}
	sec.Add(column(dd, widget.NewSized(look, 0, 40)), 132)
	var side []gunim.Node
	if i > 0 {
		side = append(side,
			e.toggle("Sync", "Restart with each cycle of the oscillator before, as the SID's hard sync",
				func(pt *synth.Patch) bool { return pt.Osc[i].Sync }, func(pt *synth.Patch, v bool) { pt.Osc[i].Sync = v }),
			e.toggle("Ring", "Turn over with each half cycle of the oscillator before, as the SID's ring modulation",
				func(pt *synth.Patch) bool { return pt.Osc[i].Ring }, func(pt *synth.Patch, v bool) { pt.Osc[i].Ring = v }))
	}
	if len(e.pt.Osc) > 1 || e.pt.Noise > 0 {
		side = append(side, e.button(icon.X, "Take this oscillator out", func(pt *synth.Patch) {
			pt.Osc = slices.Delete(pt.Osc, i, i+1)
			if len(pt.Osc) > 0 {
				pt.Osc[0].Sync, pt.Osc[0].Ring = false, false
			}
		}))
	}
	if len(side) > 0 {
		sec.Add(column(side...), 64)
	}
	return sec
}

// stringCard is a plucked string's card.
func (e *patchEditor) stringCard() *plugui.Section {
	return e.card("STRING",
		k("Decay", 0.05, 4, 1, func(pt *synth.Patch) float64 { return or(pt.Pluck.Decay, 1) },
			func(pt *synth.Patch, v float64) { pt.Pluck.Decay = v }).logScale().shows(secs),
		k("Bright", 0.01, 1, 0.6, func(pt *synth.Patch) float64 { return or(pt.Pluck.Bright, 0.6) },
			func(pt *synth.Patch, v float64) { pt.Pluck.Bright = v }),
		k("Body", 0, 1, 0, func(pt *synth.Patch) float64 { return pt.Pluck.Body },
			func(pt *synth.Patch, v float64) { pt.Pluck.Body = v }))
}

// filterCards are the filter's card, its response drawn, and its
// envelope's.
func (e *patchEditor) filterCards() []*plugui.Section {
	fl := func(name string, lo, hi float64, at func(*synth.Filter) *float64) knobSpec {
		return k(name, lo, hi, 0, func(pt *synth.Patch) float64 { return *at(&pt.Filter) },
			func(pt *synth.Patch, v float64) { *at(&pt.Filter) = v })
	}
	sec := e.card("FILTER",
		k("Cutoff", 20, 20000, 20000, func(pt *synth.Patch) float64 { return or(pt.Filter.Cutoff, 20000) },
			func(pt *synth.Patch, v float64) { pt.Filter.Cutoff = v }).logScale().shows(hertz),
		fl("Res", 0, 0.98, func(f *synth.Filter) *float64 { return &f.Res }),
		fl("Env", -4, 6, func(f *synth.Filter) *float64 { return &f.Env }).shows(octaves),
		fl("Key", 0, 1, func(f *synth.Filter) *float64 { return &f.Key }),
		fl("Velocity", 0, 3, func(f *synth.Filter) *float64 { return &f.Vel }).shows(func(v float64) string { return fmt.Sprintf("%.1f oct", v) }))
	dd := e.pick("Filter", filterNames, func(pt *synth.Patch) int {
		return max(slices.Index(filterIDs, or2(pt.Filter.Type, "none")), 0)
	}, func(pt *synth.Patch, i int) { pt.Filter.Type = filterIDs[i] })
	sec.Add(column(dd, widget.NewSized(&response{e: e}, 0, 40)), 132)
	return []*plugui.Section{sec, e.envCard("FILTER ENVELOPE", func(pt *synth.Patch) *synth.Env { return &pt.FilterEnv })}
}

// or2 returns s, or def where s is empty.
func or2(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// envCard is the card of the envelope at gives, drawn.
func (e *patchEditor) envCard(name string, at func(*synth.Patch) *synth.Env) *plugui.Section {
	ev := func(n string, lo, hi float64, field func(*synth.Env) *float64) knobSpec {
		s := k(n, lo, hi, lo, func(pt *synth.Patch) float64 { return *field(at(pt)) },
			func(pt *synth.Patch, v float64) { *field(at(pt)) = v })
		if n != "Sustain" {
			s = s.logScale().shows(secs)
		}
		return s
	}
	sec := e.card(name,
		ev("Attack", 0.001, 4, func(x *synth.Env) *float64 { return &x.Attack }),
		ev("Decay", 0.005, 4, func(x *synth.Env) *float64 { return &x.Decay }),
		ev("Sustain", 0, 1, func(x *synth.Env) *float64 { return &x.Sustain }),
		ev("Release", 0.005, 6, func(x *synth.Env) *float64 { return &x.Release }))
	return sec.Add(&envShape{e: e, at: at}, 110)
}

// lfoCards are a card for each LFO, and one to add one.
func (e *patchEditor) lfoCards() []*plugui.Section {
	var out []*plugui.Section
	for i := range e.pt.LFO {
		l := func(pt *synth.Patch) *synth.LFO { return &pt.LFO[i] }
		f := func(name string, lo, hi, def float64, field func(*synth.LFO) *float64) knobSpec {
			return k(name, lo, hi, def, func(pt *synth.Patch) float64 { return *field(l(pt)) },
				func(pt *synth.Patch, v float64) { *field(l(pt)) = v })
		}
		sec := e.card(fmt.Sprintf("LFO %d", i+1),
			f("Rate", 0.05, 20, 2, func(x *synth.LFO) *float64 { return &x.Hz }).logScale().shows(lfoHz),
			f("Beats", 0, 16, 0, func(x *synth.LFO) *float64 { return &x.Beats }).steps(64).shows(func(v float64) string {
				if v == 0 {
					return "Off"
				}
				return fmt.Sprintf("%.2f", v)
			}),
			f("Depth", 0, 4, 0.5, func(x *synth.LFO) *float64 { return &x.Depth }),
			f("Delay", 0, 2, 0, func(x *synth.LFO) *float64 { return &x.Delay }).shows(secs))
		to := e.pick("Moves", lfoTargets, func(pt *synth.Patch) int { return slices.Index(lfoTargetIDs, pt.LFO[i].To) },
			func(pt *synth.Patch, k int) { pt.LFO[i].To = lfoTargetIDs[k] })
		wave := e.pick("Wave", lfoWaves, func(pt *synth.Patch) int {
			w := pt.LFO[i].Wave
			if w == "rand" {
				w = "random"
			}
			return max(slices.Index(lfoWaveIDs, w), 0)
		}, func(pt *synth.Patch, k int) { pt.LFO[i].Wave = lfoWaveIDs[k] })
		sec.Add(column(to, wave), 104)
		sec.Add(e.button(icon.X, "Take this LFO out", func(pt *synth.Patch) { pt.LFO = slices.Delete(pt.LFO, i, i+1) }), 28)
		out = append(out, sec)
	}
	if len(e.pt.LFO) < maxLFOs {
		add := e.button(icon.Plus, "Add an LFO", func(pt *synth.Patch) {
			pt.LFO = append(pt.LFO, synth.LFO{To: "pitch", Wave: "sine", Hz: 5, Depth: 0.2})
		})
		out = append(out, (&plugui.Section{Name: "LFO"}).Add(add, 40))
	}
	return out
}

// voiceCard is how the patch sings: its vowel, the vowels its notes
// sing in turn, and the voice it sings through.
func (e *patchEditor) voiceCard() *plugui.Section {
	sec := &plugui.Section{Name: "VOICE · sings a vowel"}
	vowel := plugui.NewSelector("Vowel", []string{"off", "a", "e", "i", "o", "u"},
		[]string{"Sing no vowel", "Sing a", "Sing e", "Sing i", "Sing o", "Sing u"})
	vowel.SetSelected(max(slices.Index(vowelIDs, e.pt.Vowel), 0), nil)
	vowel.OnPick = func(i int, u *gunim.UI) {
		e.edit("Pick the vowel", func(pt *synth.Patch) { pt.Vowel = vowelIDs[i] }, u)
	}
	e.follows = append(e.follows, func(u *gunim.UI) { vowel.SetSelected(max(slices.Index(vowelIDs, e.pt.Vowel), 0), u) })
	singer := e.pick("Sings through", singerNames, func(pt *synth.Patch) int { return slices.Index(singerIDs, pt.Singer) },
		func(pt *synth.Patch, i int) { pt.Singer = singerIDs[i] })
	vocoder := e.toggle("Vocoder", "Sing the vowel through a vocoder's ten bands, as a Roland VP-330's: a robot's voice",
		func(pt *synth.Patch) bool { return pt.Vocoder }, func(pt *synth.Patch, v bool) { pt.Vocoder = v })
	turns := e.text("Vowels", "vowels in turn, as a i o", func(pt *synth.Patch) string { return pt.Vowels },
		func(pt *synth.Patch, s string) { pt.Vowels = strings.Join(strings.Fields(s), " ") }, vowelsOK)
	turns.Tooltip = "Vowels the notes sing in turn, a note each, as a song's track does"
	sec.Add(vowel, 104)
	sec.Add(column(singer, vocoder), 120)
	sec.Add(column(widget.NewLabel("In turn"), turns), 150)
	return sec
}

// vowelsOK reports whether s is vowels written apart by spaces.
func vowelsOK(s string) bool {
	for _, w := range strings.Fields(s) {
		if len(w) != 1 || !strings.Contains(Vowels, w) {
			return false
		}
	}
	return true
}

// playCard is how the notes play: glide, drive, noise, drift, accent.
func (e *patchEditor) playCard() *plugui.Section {
	sec := e.card("PLAYS",
		k("Glide", 0, 0.5, 0, func(pt *synth.Patch) float64 { return pt.Glide }, func(pt *synth.Patch, v float64) { pt.Glide = v }).shows(secs),
		k("Drive", 0, 1, 0, func(pt *synth.Patch) float64 { return pt.Drive }, func(pt *synth.Patch, v float64) { pt.Drive = v }),
		k("Noise", 0, 1, 0, func(pt *synth.Patch) float64 { return pt.Noise }, func(pt *synth.Patch, v float64) { pt.Noise = v }),
		k("Drift", 0, 20, 0, func(pt *synth.Patch) float64 { return pt.Drift }, func(pt *synth.Patch, v float64) { pt.Drift = v }).
			shows(func(v float64) string { return fmt.Sprintf("%.0f c", v) }),
		k("Accent", 0, 4, 0, func(pt *synth.Patch) float64 { return pt.Accent }, func(pt *synth.Patch, v float64) { pt.Accent = v }).
			shows(func(v float64) string { return fmt.Sprintf("%.1f oct", v) }))
	slide := e.toggle("Slide", "Glide only into a note tied to the one before, as a TB-303 slides; a note after a rest jumps",
		func(pt *synth.Patch) bool { return pt.Slide }, func(pt *synth.Patch, v bool) { pt.Slide = v })
	return sec.Add(slide, 64)
}

// answerCard is how the patch answers the mod wheel, or the key's
// pressure: as its kind does until a knob is turned, which writes all
// four.
func (e *patchEditor) answerCard(name string, pressure bool) *plugui.Section {
	at := func(pt *synth.Patch) *synth.Response {
		mod, press := pt.Responses()
		if pressure {
			if pt.Pressure == nil {
				pt.Pressure = &press
			}
			return pt.Pressure
		}
		if pt.Mod == nil {
			pt.Mod = &mod
		}
		return pt.Mod
	}
	read := func(pt *synth.Patch) synth.Response {
		mod, press := pt.Responses()
		if pressure {
			return press
		}
		return mod
	}
	f := func(n string, lo, hi float64, field func(*synth.Response) *float64) knobSpec {
		return k(n, lo, hi, 0, func(pt *synth.Patch) float64 { r := read(pt); return *field(&r) },
			func(pt *synth.Patch, v float64) { *field(at(pt)) = v })
	}
	return e.card(name,
		f("Vibrato", 0, 2, func(r *synth.Response) *float64 { return &r.Vibrato }).shows(func(v float64) string { return fmt.Sprintf("%.2f st", v) }),
		f("Bright", -4, 4, func(r *synth.Response) *float64 { return &r.Bright }).shows(octaves),
		f("Level", -24, 12, func(r *synth.Response) *float64 { return &r.Level }).shows(decibels),
		f("Vowel", -4, 4, func(r *synth.Response) *float64 { return &r.Vowel }).steps(8).shows(func(v float64) string { return fmt.Sprintf("%+.0f", v) }))
}

// chipCard steps the level as a console does, and slides each note in.
func (e *patchEditor) chipCard() *plugui.Section {
	chip := func(pt *synth.Patch) *synth.Chip {
		if pt.Chip == nil {
			pt.Chip = &synth.Chip{}
		}
		return pt.Chip
	}
	bend := func(pt *synth.Patch) *synth.Bend {
		if pt.Bend == nil {
			pt.Bend = &synth.Bend{}
		}
		return pt.Bend
	}
	sec := e.card("CHIP",
		k("Levels", 2, 32, 16, func(pt *synth.Patch) float64 {
			if pt.Chip == nil {
				return 16
			}
			return or(float64(pt.Chip.Levels), 16)
		}, func(pt *synth.Patch, v float64) { chip(pt).Levels = int(math.Round(v)) }).steps(30).shows(whole),
		k("Frames", 25, 120, 60, func(pt *synth.Patch) float64 {
			if pt.Chip == nil {
				return 60
			}
			return or(pt.Chip.Hz, 60)
		}, func(pt *synth.Patch, v float64) { chip(pt).Hz = math.Round(v) }).steps(95).shows(func(v float64) string { return fmt.Sprintf("%.0f Hz", v) }),
		k("Bend", -24, 24, 0, func(pt *synth.Patch) float64 {
			if pt.Bend == nil {
				return 0
			}
			return pt.Bend.Semis
		}, func(pt *synth.Patch, v float64) { bend(pt).Semis = v }).steps(48).shows(semis),
		k("Bend time", 0, 0.5, 0, func(pt *synth.Patch) float64 {
			if pt.Bend == nil {
				return 0
			}
			return pt.Bend.Time
		}, func(pt *synth.Patch, v float64) { bend(pt).Time = v }).shows(secs))
	// The first two knobs step the level; the switch brings it in.
	sec.Switch = plugui.NewPower("stepped")
	sec.Switch.SetChecked(e.pt.Chip != nil, nil)
	sec.Switch.OnChange = func(on bool, u *gunim.UI) gunim.Intent {
		e.edit("Step the level", func(pt *synth.Patch) {
			pt.Chip = nil
			if on {
				pt.Chip = &synth.Chip{Levels: 16, Hz: 60}
			}
		}, u)
		return nil
	}
	for _, kn := range sec.Knobs[:2] {
		kn.Disabled = e.pt.Chip == nil
	}
	return sec
}

// arpCard is the patch's fast arpeggio, as a Commodore 64 plays a chord
// on one voice.
func (e *patchEditor) arpCard() *plugui.Section {
	sec := e.card("ARPEGGIO",
		k("Rate", 5, 100, 50, func(pt *synth.Patch) float64 {
			if pt.Arpeggio == nil {
				return 50
			}
			return or(pt.Arpeggio.Hz, 50)
		}, func(pt *synth.Patch, v float64) {
			if pt.Arpeggio != nil {
				pt.Arpeggio.Hz = math.Round(v)
			}
		}).steps(95).shows(func(v float64) string { return fmt.Sprintf("%.0f Hz", v) }))
	sec.Switch = plugui.NewPower("arpeggio")
	sec.Switch.SetChecked(e.pt.Arpeggio != nil, nil)
	sec.Switch.OnChange = func(on bool, u *gunim.UI) gunim.Intent {
		e.edit("Arpeggiate", func(pt *synth.Patch) {
			pt.Arpeggio = nil
			if on {
				pt.Arpeggio = &synth.Arpeggio{Steps: []int{0, 4, 7}, Hz: 50}
			}
		}, u)
		return nil
	}
	if e.pt.Arpeggio == nil {
		return sec
	}
	chords := e.toggle("Chords", "Play a track's chords, ch, as the arpeggio",
		func(pt *synth.Patch) bool { return pt.Arpeggio != nil && pt.Arpeggio.Chord },
		func(pt *synth.Patch, v bool) { pt.Arpeggio.Chord = v })
	steps := e.text("Steps", "0 4 7", func(pt *synth.Patch) string {
		var out []string
		if pt.Arpeggio != nil {
			for _, s := range pt.Arpeggio.Steps {
				out = append(out, strconv.Itoa(s))
			}
		}
		return strings.Join(out, " ")
	}, func(pt *synth.Patch, s string) {
		pt.Arpeggio.Steps = pt.Arpeggio.Steps[:0]
		for _, w := range strings.Fields(s) {
			n, _ := strconv.Atoi(w)
			pt.Arpeggio.Steps = append(pt.Arpeggio.Steps, n)
		}
	}, func(s string) bool {
		if len(strings.Fields(s)) == 0 {
			return false
		}
		for _, w := range strings.Fields(s) {
			if n, err := strconv.Atoi(w); err != nil || n < -24 || n > 36 {
				return false
			}
		}
		return true
	})
	steps.Tooltip = "The semitones above the note it steps through"
	return sec.Add(column(chords, steps), 120)
}

// kitDefaults are the drums a kit with none of its own lists shows.
var kitDefaults = []string{"bd", "sn", "cp", "hh", "oh", "lt", "mt", "ht", "cr", "rd"}

// kitNames are the drums the kit's cards show: those it lists, by the
// note they play on, or else the usual ten.
func kitNames(pt *synth.Patch) []string {
	if pt.Kind != "drums" {
		return nil
	}
	if len(pt.Kit) == 0 {
		return kitDefaults
	}
	names := slices.Collect(maps.Keys(pt.Kit))
	slices.SortFunc(names, func(a, b string) int {
		if ka, kb := drumKey(a), drumKey(b); ka != kb {
			return ka - kb
		}
		return strings.Compare(a, b)
	})
	return names
}

// drumKey is the note a drum named name plays on, for sorting.
func drumKey(name string) int {
	if n, err := strconv.Atoi(name); err == nil {
		return n
	}
	if i := slices.Index(kitDefaults, name); i >= 0 {
		return 35 + i
	}
	return 200
}

// kitCards are the patch's level, and a card for each drum.
func (e *patchEditor) kitCards() []*plugui.Section {
	names := kitNames(e.pt)
	out := make([]*plugui.Section, 0, len(names)+1)
	out = append(out, e.card("KIT",
		k("Gain", 0, 2, 1, func(pt *synth.Patch) float64 { return or(pt.Gain, 1) }, func(pt *synth.Patch, v float64) { pt.Gain = v })))
	for _, name := range names {
		d := func(pt *synth.Patch) synth.Drum { return pt.Kit[name] }
		set := func(fn func(*synth.Drum)) func(pt *synth.Patch, v float64) {
			return func(pt *synth.Patch, v float64) {
				if pt.Kit == nil {
					pt.Kit = map[string]synth.Drum{}
				}
				dr := pt.Kit[name]
				fn(&dr)
				_ = v
				pt.Kit[name] = dr
			}
		}
		title := strings.ToUpper(name)
		if t := e.pt.Kit[name].Type; t != "" {
			title += " · " + t
		}
		out = append(out, e.card(title,
			knobSpec{prm: plug.Param{Name: "Tune", Min: -24, Max: 24, Format: semis}, get: func(pt *synth.Patch) float64 { return d(pt).Tune },
				set: func(pt *synth.Patch, v float64) { set(func(x *synth.Drum) { x.Tune = v })(pt, v) }},
			knobSpec{prm: plug.Param{Name: "Decay", Min: 0.25, Max: 4, Default: 1, Log: true, Format: times}, get: func(pt *synth.Patch) float64 { return or(d(pt).Decay, 1) },
				set: func(pt *synth.Patch, v float64) { set(func(x *synth.Drum) { x.Decay = v })(pt, v) }},
			knobSpec{prm: plug.Param{Name: "Tone", Max: 1}, get: func(pt *synth.Patch) float64 { return d(pt).Tone },
				set: func(pt *synth.Patch, v float64) { set(func(x *synth.Drum) { x.Tone = v })(pt, v) }},
			knobSpec{prm: plug.Param{Name: "Gain", Max: 2, Default: 1}, get: func(pt *synth.Patch) float64 { return or(d(pt).Gain, 1) },
				set: func(pt *synth.Patch, v float64) { set(func(x *synth.Drum) { x.Gain = v })(pt, v) }},
			knobSpec{prm: plug.Param{Name: "Pan", Min: -1, Max: 1, Format: pan}, get: func(pt *synth.Patch) float64 { return d(pt).Pan },
				set: func(pt *synth.Patch, v float64) { set(func(x *synth.Drum) { x.Pan = v })(pt, v) }}))
	}
	return out
}

// flow lays cards out in rows from the top, as many to a row as fit,
// each row centred, as a panel's sections stand at its foot.
type flow struct {
	cards []*plugui.Section
}

// Children implements [gunim.Composite].
func (f *flow) Children() []gunim.Node {
	out := make([]gunim.Node, len(f.cards))
	for i, c := range f.cards {
		out[i] = c
	}
	return out
}

// Layout implements [gunim.Node]: as wide as it is given, and as tall
// as its rows.
func (f *flow) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	w := c.Max.W
	// Placed at the foot of a view of no height, the rows' top tells how
	// tall they stand.
	h := panelInset - plugui.PlaceSections(geom.Sz(w, 0), panelInset, f.cards, func(int, geom.Rect) {})
	plugui.PlaceSections(geom.Sz(w, h), panelInset, f.cards, func(i int, r geom.Rect) {
		// Cards built again since the frame took the children wait for
		// the next frame, which Invalidate asked for.
		if i >= kids.Len() {
			return
		}
		kids.At(i).Layout(gunim.Tight(r.Size()))
		kids.At(i).Place(r.Min)
	})
	return geom.Sz(w, h)
}

// Paint implements [gunim.Node].
func (f *flow) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}
