package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-music/calls"
)

// harness runs the lab's two halves in a test: the window offscreen,
// and the sound mixed by hand, with no speaker, on a copy of the
// library in a folder of its own.
type harness struct {
	t   *testing.T
	w   *gunim.Window
	l   *lab
	v   *root
	dir string
	at  map[gunim.Node]geom.Rect
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	src, err := filepath.Glob("../../voices/*.json")
	if err != nil || len(src) == 0 {
		t.Fatal("no recipes in voices/")
	}
	// The versions to compare, each a folder of its own, come too.
	more, _ := filepath.Glob("../../voices/*/*.json")
	for _, f := range append(src, more...) {
		rel, _ := filepath.Rel("../../voices", f)
		b, _ := os.ReadFile(f)
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &harness{t: t, w: gunimtest.New(t, geom.Sz(1360, 940), widget.NewSurface()), dir: dir, at: map[gunim.Node]geom.Rect{}}
	h.w.RegisterTheme(widget.Dark())
	gunim.RegisterView(h.w, "lab", func(s Lab) *root {
		h.v = buildView(s)
		return h.v
	}, func(r *root, s Lab, u *gunim.UI) {
		r.update(s, u)
		for n := range h.at {
			h.at[n], _ = u.Bounds(n)
		}
	})
	l, err := newLab(h.w.Client(), audio.NewMixer(), dir)
	if err != nil {
		t.Fatal(err)
	}
	h.l = l
	if err := h.w.Client().Mount(gunim.Root, "lab", "lab", l.state()); err != nil {
		t.Fatal(err)
	}
	h.frame()
	return h
}

func (h *harness) frame() {
	_ = h.w.Client().Update("lab", h.l.state())
	h.w.Frame(time.Second / 60)
	h.w.Frame(time.Second / 60)
}

// do carries out intents, as the window sends them.
func (h *harness) do(vs ...gunim.Intent) {
	for _, v := range vs {
		h.l.handle(v)
	}
	h.frame()
}

// intent waits for the window to send an intent.
func (h *harness) intent() gunim.Intent {
	select {
	case ev := <-h.w.Client().Intents():
		return ev.Intent
	case <-time.After(time.Second):
		h.t.Fatal("the window sent no intent")
		return nil
	}
}

// centre returns the middle of n in the window.
func (h *harness) centre(n gunim.Node) geom.Point {
	h.at[n] = geom.Rect{}
	h.frame()
	r := h.at[n]
	if r.Empty() {
		h.t.Fatalf("%T is not on screen", n)
	}
	return geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

func TestTheLabShowsEachCompanionsCallsMeasured(t *testing.T) {
	h := newHarness(t)
	s := h.l.state()
	if len(s.Companions) != len(h.l.lib.Order) || s.Companions[0].ID != "groda" || !s.Companions[0].Open {
		t.Fatalf("the list shows %+v", s.Companions)
	}
	for _, c := range s.Calls {
		if !c.Made || len(c.Wave) == 0 || c.Stats == "" || len(c.Problems) > 0 {
			t.Errorf("the frog's %s shows %+v", c.Kind, c)
		}
	}
	h.do(CompanionChosen{ID: "raven"})
	if s = h.l.state(); s.Name != "Räven (fox who runs)" || !s.Calls[0].Made {
		t.Fatalf("the fox shows %q, its hello made %v", s.Name, s.Calls[0].Made)
	}
	// The last layer taken away unmakes the call, and a layer added
	// makes it again.
	h.do(LayerRemoved{Layer: 0})
	if s = h.l.state(); s.Calls[0].Made || len(s.Editor.Layers) != 0 {
		t.Error("the fox's hello is still made with no layer")
	}
	h.do(LayerAdded{Model: "yip"})
	if s = h.l.state(); !s.Calls[0].Made || len(s.Editor.Layers) != 1 {
		t.Error("a layer added made no hello")
	}
}

func TestAKnobTurnedChangesTheCallAndPlaysItAsItIsLetGo(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "uggla"})
	k := h.v.layers[0].ks[0]
	if k.p.Name != "pitch" {
		t.Fatalf("the first knob sets %s", k.p.Name)
	}
	c := h.centre(k)
	h.w.Input(input.PointerDown{Pos: c, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerMove{Pos: c.Add(geom.Pt(0, -30)), Time: time.Now()})
	got, ok := h.intent().(ParamSet)
	if !ok || got.Layer != 0 || got.Name != "pitch" || got.Value <= h.l.current().Layers[0].Get("pitch") || got.Done {
		t.Fatalf("a drag up the pitch knob sent %#v", got)
	}
	h.w.Input(input.PointerUp{Pos: c.Add(geom.Pt(0, -30)), Time: time.Now()})
	done := h.intent().(ParamSet)
	if !done.Done {
		t.Fatalf("letting go of the knob sent %#v", done)
	}
	before := h.l.takes[0].Stats.Presence
	h.do(got, done)
	if h.l.takes[0].Stats.Presence == before {
		t.Error("the pitch turned up left the hello as it was")
	}
	if i, _ := h.l.playing(); i != 0 {
		t.Error("the hello did not play as the knob was let go")
	}
	if !h.l.state().Dirty {
		t.Error("the change is not marked unsaved")
	}
}

func TestNotesAndChangesSaveToTheCompanionsFile(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "uggla"}, CallChosen{Call: 2},
		ParamSet{Layer: 0, Name: "question", Value: 7},
		NotesSet{Text: "More of a question, please"}, Saved{})
	if s := h.l.state(); s.Dirty || !strings.HasPrefix(s.Status, "Saved") {
		t.Fatalf("after saving the lab says %q, dirty %v", s.Status, s.Dirty)
	}
	lib, err := calls.LoadDir(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	oops := lib.Companion("uggla").Calls[calls.Oops]
	if oops.Notes != "More of a question, please" || oops.Layers[0].Params["question"] != 7 {
		t.Errorf("the file holds %q and question %v", oops.Notes, oops.Layers[0].Params["question"])
	}
	// A change reverted reads the file back.
	h.do(ParamSet{Layer: 0, Name: "question", Value: 1}, Reverted{})
	if v := h.l.current().Layers[0].Params["question"]; v != 7 {
		t.Errorf("reverted, the question is %v", v)
	}
}

func TestANewTakeStraysAndAsSetComesBack(t *testing.T) {
	h := newHarness(t)
	set := h.l.takes[1].Samples
	h.do(NewTake{Call: 1})
	if h.l.takes[1].Seed == 0 || len(h.l.takes[1].Samples) == len(set) && h.l.takes[1].Samples[1000] == set[1000] {
		t.Error("a new take is the call as set")
	}
	if !strings.HasPrefix(h.l.state().Calls[1].Take, "Take ") {
		t.Errorf("the card says %q", h.l.state().Calls[1].Take)
	}
	h.do(AsSet{Call: 1})
	if h.l.takes[1].Seed != 0 || h.l.state().Calls[1].Take != "As set" {
		t.Error("as set did not come back")
	}
}

func TestPlayAllPlaysTheThreeCallsInTurn(t *testing.T) {
	h := newHarness(t)
	h.do(PlayAll{})
	var heard []int
	buf := make([]float32, 2*1024)
	for range 1000 {
		if i, _ := h.l.playing(); i >= 0 && (len(heard) == 0 || heard[len(heard)-1] != i) {
			heard = append(heard, i)
		}
		h.l.mix.Mix(buf)
		h.l.tick()
		if len(heard) == 3 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(heard) != 3 || heard[0] != 0 || heard[1] != 1 || heard[2] != 2 {
		t.Errorf("heard the calls %v", heard)
	}
}

func TestTheTopKnobStepsThroughTheOddHarmonicsByTheWheel(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "robo-ninja"}, CallChosen{Call: 2})
	var k *knob
	for _, kk := range h.v.layers[0].ks {
		if kk.p.Name == "top" {
			k = kk
		}
	}
	if k == nil {
		t.Fatal("the glitch shows no top knob")
	}
	if k.format() != "all" {
		t.Errorf("a top of 0 reads %q, not all", k.format())
	}
	c := h.centre(k)
	var got []float64
	for range 3 {
		h.w.Input(input.Scroll{Pos: c, Delta: geom.Pt(0, -40)})
		in, ok := h.intent().(ParamSet)
		if !ok || in.Name != "top" {
			t.Fatalf("the wheel sent %#v", in)
		}
		got = append(got, in.Value)
		h.do(in)
	}
	if got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Errorf("the wheel stepped the top through %v, not 1, 3, 5", got)
	}
}

func TestABossShowsItsSevenCallsAndACompanionItsThree(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "boss-stor"})
	if h.v.cards.n != len(calls.BossKinds) {
		t.Fatalf("a boss shows %d calls, want %d", h.v.cards.n, len(calls.BossKinds))
	}
	if got := h.v.cards.cards[6].title.Text; got != "Whimper" {
		t.Errorf("the boss's seventh card is %q, want Whimper", got)
	}
	h.do(CompanionChosen{ID: "uggla"})
	if h.v.cards.n != len(calls.Kinds) {
		t.Fatalf("a companion shows %d calls, want %d", h.v.cards.n, len(calls.Kinds))
	}
}

func TestACallTheCharacterOpenDoesNotMakeIsLeftAlone(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "boss-stor"}, CompanionChosen{ID: "uggla"})
	// The window still showed the boss as these were sent.
	for _, in := range []gunim.Intent{PlayCall{Call: 6}, NewTake{Call: 5}, AsSet{Call: 4}, CallChosen{Call: 6}, PlayCall{Call: -1}} {
		h.do(in)
	}
	if s := h.l.state(); s.Selected != 0 || len(s.Calls) != 3 {
		t.Errorf("the owl shows call %d of %d", s.Selected, len(s.Calls))
	}
}

func TestTheDigitKeysOpenEachCallTheCharacterMakes(t *testing.T) {
	h := newHarness(t)
	h.do(CompanionChosen{ID: "boss-mellan"})
	if !strings.Contains(h.v.keys.Text, "1 to 7") || !strings.Contains(h.v.keys.Text, "whimper") {
		t.Errorf("a boss's keys say %q", h.v.keys.Text)
	}
	h.w.Input(input.KeyPress{Key: input.Key7, Char: '7', Time: time.Now()})
	if in, ok := h.intent().(CallChosen); !ok || in.Call != 6 {
		t.Fatalf("7 sent %#v, not the seventh call", in)
	}
	h.intent() // and plays it
	h.do(CompanionChosen{ID: "uggla"})
	if !strings.Contains(h.v.keys.Text, "1 to 3 open and play hello, cheer, oops") {
		t.Errorf("a companion's keys say %q", h.v.keys.Text)
	}
	h.w.Input(input.KeyPress{Key: input.Key7, Char: '7', Time: time.Now()})
	select {
	case ev := <-h.w.Client().Intents():
		t.Errorf("7 sent %#v for a companion of three calls", ev.Intent)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestKeysTypedInTheNotesPlayNothing(t *testing.T) {
	h := newHarness(t)
	// The notes lie below the window's edge: scroll down to them, over
	// the heading, where no knob takes the wheel.
	over := h.centre(h.v.name)
	for range 10 {
		h.w.Input(input.Scroll{Pos: over, Delta: geom.Pt(0, -120), Time: time.Now()})
		for range 10 {
			h.w.Frame(time.Second / 60)
		}
	}
	at := h.centre(h.v.notes)
	if at.Y > 900 {
		t.Fatalf("the notes still lie off the window, at %v", at)
	}
	h.w.Input(input.PointerDown{Pos: at, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: at, Time: time.Now()})
	h.frame()
	h.w.Input(input.KeyPress{Key: input.KeyA, Char: 'a', Typed: true, Time: time.Now()})
	h.w.Input(input.TextInput{Text: "a"})
	h.w.Input(input.KeyPress{Key: input.KeySpace, Char: ' ', Typed: true, Time: time.Now()})
	h.w.Input(input.TextInput{Text: " "})
	h.frame()
	for {
		select {
		case ev := <-h.w.Client().Intents():
			switch ev.Intent.(type) {
			case NotesSet:
			default:
				t.Errorf("typing in the notes sent %#v", ev.Intent)
			}
		case <-time.After(200 * time.Millisecond):
			if got := h.v.notes.Text(); got != "a " {
				t.Errorf("the notes read %q, not \"a \"", got)
			}
			return
		}
	}
}

func TestBHearsTheCallAgainInTheOtherVersion(t *testing.T) {
	h := newHarness(t)
	s := h.l.state()
	if len(s.Versions) < 2 || s.Version != 0 {
		t.Fatalf("the lab offers the versions %v, %d heard", s.Versions, s.Version)
	}
	h.do(CompanionChosen{ID: "uggla"}, CallChosen{Call: 2})
	fuller := h.l.takes[2].Samples
	h.w.Input(input.KeyPress{Key: input.KeyB, Char: 'b', Time: time.Now()})
	in, ok := h.intent().(VersionChosen)
	if !ok || in.Version != 1 {
		t.Fatalf("B sent %#v", in)
	}
	h.do(in)
	s = h.l.state()
	if s.Version != 1 || h.l.comp.ID != "uggla" || s.Selected != 2 {
		t.Fatalf("after B the lab hears version %d, %s's call %d", s.Version, h.l.comp.ID, s.Selected)
	}
	if i, _ := h.l.playing(); i != 2 {
		t.Error("B did not play the oops again")
	}
	if slices.Equal(h.l.takes[2].Samples, fuller) {
		t.Error("the owl's oops sounds the same in both versions")
	}
	// Saved, a version writes to its own folder.
	h.do(NotesSet{Text: "the brief's"}, Saved{})
	b, err := os.ReadFile(filepath.Join(h.l.versions[1].dir, "uggla.json"))
	if err != nil || !strings.Contains(string(b), "the brief's") {
		t.Errorf("the brief's version saved to %s: %v", h.l.versions[1].dir, err)
	}
	h.w.Input(input.KeyPress{Key: input.KeyB, Char: 'b', Time: time.Now()})
	if in, ok := h.intent().(VersionChosen); !ok || in.Version != 0 {
		t.Errorf("B again sent %#v", in)
	}
}
