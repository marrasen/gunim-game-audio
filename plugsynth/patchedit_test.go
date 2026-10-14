package plugsynth

import (
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// showPage shows page name of pn, in the window, through a patch.
func showPage(t *testing.T, w *gunim.Window, run func(int), pn *panel, name string) {
	t.Helper()
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { pn.ShowPage(name, u) })
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(3)
}

// bounds returns where n stands in the window.
func bounds(t *testing.T, w *gunim.Window, run func(int), n gunim.Node) geom.Rect {
	t.Helper()
	var b geom.Rect
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { b, _ = u.Bounds(n) })
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return b
}

// card returns the editor's card named name, or nil.
func card(e *patchEditor, name string) *plugui.Section {
	for _, c := range e.flow.cards {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestEveryPageShowsEverySound shows each page of the panel for sounds
// of every kind, and the editor shows the cards each kind has.
func TestEveryPageShowsEverySound(t *testing.T) {
	p, h := newSynth(t)
	w, pn, run := stage(t, p, h)
	for _, c := range []struct {
		bank, group, name string
		cards             []string
	}{
		{Library, "Leads", "Jupiter brass lead", []string{"SOUND", "OSC 1", "OSC 2", "FILTER", "AMP ENVELOPE", "MOD WHEEL", "PRESSURE", "CHIP", "ARPEGGIO"}},
		{Songs, "Wire Cathedral", "whisper", []string{"VOICE · sings a vowel", "PLAYS"}},
		{GeneralMIDI, "Guitar", "Acoustic Guitar (nylon)", []string{"FILTER"}},
		{GeneralMIDI, GMDrums, "TR-808 kit", []string{"KIT"}},
	} {
		s, ok := Find(c.bank, c.group, c.name)
		if !ok {
			t.Fatalf("no sound %s", c.name)
		}
		if err := p.SetState(s.State()); err != nil {
			t.Fatal(err)
		}
		pn.FollowState(nil)
		for _, page := range PageNames {
			showPage(t, w, run, pn, page)
		}
		for _, name := range c.cards {
			if card(pn.pages.editor, name) == nil {
				t.Errorf("%s: the editor lacks a card %s", c.name, name)
			}
		}
	}
}

// TestAKnobOfTheEditorTurnsThePatch drags the editor's cutoff up: the
// turn is one edit of the state, the plugin plays the patch turned, and
// an undo, as the host sets the state back, turns the knob back.
func TestAKnobOfTheEditorTurnsThePatch(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Juno pad")
	w, pn, run := stage(t, p, h)
	pn.FollowState(nil)
	showPage(t, w, run, pn, "Patch")
	before := p.sound().Patch.Filter.Cutoff
	was := p.State()
	sec := card(pn.pages.editor, "FILTER")
	if sec == nil {
		t.Fatal("no FILTER card")
	}
	kn := sec.Knobs[0]
	sb, fb := bounds(t, w, run, sec), bounds(t, w, run, pn.pages.editor.flow)
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) {
		pn.pages.editor.Reveal(geom.Rect{Min: sb.Min.Sub(fb.Min), Max: sb.Max.Sub(fb.Min)}, u)
	})
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(40)
	b := bounds(t, w, run, kn)
	at := geom.Pt((b.Min.X+b.Max.X)/2, b.Min.Y+20)
	h.log = nil
	w.Input(input.PointerMove{Pos: at})
	run(1)
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	for dy := float32(4); dy <= 40; dy += 4 {
		w.Input(input.PointerMove{Pos: at.Add(geom.Pt(0, -dy))})
		run(1)
	}
	w.Input(input.PointerUp{Pos: at.Add(geom.Pt(0, -40)), Button: input.ButtonPrimary})
	run(2)
	after := p.sound().Patch.Filter.Cutoff
	if after <= or(before, 20000)*1.1 {
		t.Fatalf("dragged up, the cutoff goes from %.0f Hz to %.0f", before, after)
	}
	if h.log[0] != "begin Turn cutoff" || h.log[len(h.log)-1] != "end" || strings.Count(strings.Join(h.log, ","), "begin") != 1 {
		t.Fatalf("the turn tells the host %v", h.log)
	}
	// An undo: the host sets the state back.
	if err := p.SetState(was); err != nil {
		t.Fatal(err)
	}
	pn.FollowState(nil)
	if got := pn.pages.editor.pt.Filter.Cutoff; got != before {
		t.Fatalf("after the undo, the editor shows a cutoff of %.0f, not %.0f", got, before)
	}
}

// TestTheEditorAddsAndTakesOutOscillators adds an oscillator, which is
// one edit and a card more, and takes one out.
func TestTheEditorAddsAndTakesOutOscillators(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Juno pad")
	e := newPatchEditor(p, h, nil)
	n := len(e.pt.Osc)
	e.edit("Add an oscillator", func(pt *synth.Patch) { pt.Osc = append(pt.Osc, synth.Osc{Wave: "sine"}) }, nil)
	if got := len(p.sound().Patch.Osc); got != n+1 {
		t.Fatalf("the plugin plays %d oscillators, not %d", got, n+1)
	}
	if card(e, "OSC "+string(rune('1'+n))) == nil {
		t.Fatal("the new oscillator has no card")
	}
	e.edit("Take it out", func(pt *synth.Patch) { pt.Osc = slices.Delete(pt.Osc, n, n+1) }, nil)
	if card(e, "OSC "+string(rune('1'+n))) != nil || len(p.sound().Patch.Osc) != n {
		t.Fatal("the oscillator taken out stays")
	}
}

// TestTheEditorShowsTheCardsOfASoundPicked shows the editor, then picks
// a sound of another shape: the cards built for it are the ones it
// shows, each in a place of its own, and none of the old ones stays.
func TestTheEditorShowsTheCardsOfASoundPicked(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Juno pad")
	w, pn, run := stage(t, p, h)
	pn.FollowState(nil)
	showPage(t, w, run, pn, "Patch")
	old := slices.Clone(pn.pages.editor.flow.cards)
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) {
		play(t, p, "Robot girl")
		pn.FollowState(u)
	})
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(40)
	cards := pn.pages.editor.flow.cards
	if slices.Equal(cards, old) {
		t.Fatal("the editor did not build the cards again")
	}
	rs := make([]geom.Rect, 0, len(cards))
	for _, c := range cards {
		var r geom.Rect
		var ok bool
		gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { r, ok = u.Bounds(c) })
		if err := w.Client().Patch("synth", struct{}{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if !ok {
			t.Fatalf("the card %s is not shown", c.Name)
		}
		for _, o := range rs {
			if r.Min.X < o.Max.X && o.Min.X < r.Max.X && r.Min.Y < o.Max.Y && o.Min.Y < r.Max.Y {
				t.Fatalf("the card %s at %v stands over another at %v", c.Name, r, o)
			}
		}
		rs = append(rs, r)
	}
	for _, c := range old {
		gone := false
		gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { _, shown := u.Bounds(c); gone = !shown })
		if err := w.Client().Patch("synth", struct{}{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if !gone && !slices.Contains(cards, c) {
			t.Fatalf("the old card %s is still shown", c.Name)
		}
	}
}

// TestDrawingTheWaveEndsOnASecondButton draws a Game Boy wave, and a
// second button pressed mid-drag ends the edit where it got to.
func TestDrawingTheWaveEndsOnASecondButton(t *testing.T) {
	p, h := newSynth(t)
	pt := &synth.Patch{Osc: []synth.Osc{{Wave: "gbwave"}}, Amp: synth.Env{Sustain: 1}}
	if err := p.SetState(State{Name: "GB", Patch: pt}.Encode()); err != nil {
		t.Fatal(err)
	}
	e := newPatchEditor(p, h, nil)
	te := &tableEdit{e: e, size: geom.Sz(160, 40)}
	h.log = nil
	te.Handle(input.PointerDown{Pos: geom.Pt(6, 6), Button: input.ButtonPrimary}, nil)
	te.Handle(input.PointerMove{Pos: geom.Pt(20, 30)}, nil)
	if !te.held || !e.held {
		t.Fatal("a drag on the table holds nothing")
	}
	te.Handle(input.PointerDown{Pos: geom.Pt(20, 30), Button: input.ButtonSecondary}, nil)
	if te.held || e.held || h.log[len(h.log)-1] != "end" {
		t.Fatalf("a second button leaves the drag held, the host told %v", h.log)
	}
	if got := p.sound().Patch.Osc[0].Table; len(got) != 32 || got[0] != 14 {
		t.Fatalf("the wave drawn is %v", got)
	}
}
