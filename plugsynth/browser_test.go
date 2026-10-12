package plugsynth

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// stage mounts p's full view, made for h, offscreen, and returns the
// window, the view, and a way to run frames.
func stage(t *testing.T, p *plugin, h plug.Host) (*gunim.Window, *panel, func(int)) {
	t.Helper()
	pn, ok := p.View(plug.Full, h).(*panel)
	if !ok {
		t.Fatal("the full view is of another type")
	}
	w := gunimtest.New(t, geom.Sz(900, 440), nil)
	gunim.RegisterView(w, "synth", func(struct{}) gunim.Node { return pn }, nil)
	if err := w.Client().Mount(gunim.Root, "synth", "synth", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, pn, run
}

// where returns the middle of row i of a column, in the window.
func where(t *testing.T, w *gunim.Window, run func(int), n gunim.Node, r geom.Rect) geom.Point {
	t.Helper()
	var b geom.Rect
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { b, _ = u.Bounds(n) })
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return b.Min.Add(geom.Pt(40, (r.Min.Y+r.Max.Y)/2))
}

// click presses and lets go at p, with the pointer moved there in steps
// of 16 px from the left, as a hand does.
func click(w *gunim.Window, run func(int), p geom.Point) {
	for x := p.X - 48; x < p.X; x += 16 {
		w.Input(input.PointerMove{Pos: geom.Pt(x, p.Y)})
		run(1)
	}
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	run(1)
}

func TestTheBrowserPicksASoundAsOneEditAndPlaysIt(t *testing.T) {
	p, h := newSynth(t)
	w, pn, run := stage(t, p, h)
	b := pn.browser
	if b.bank != Library || b.group != "Pads & strings" || len(b.sounds.rows) != 7 {
		t.Fatalf("the browser opens on %s/%s with %d rows, not the Juno pad's group", b.bank, b.group, len(b.sounds.rows))
	}
	// The Bass group, from the left column.
	gi := b.groups.index(Library, "Bass")
	click(w, run, where(t, w, run, b.groups, b.groups.rowRect(gi)))
	if b.group != "Bass" || b.sounds.rows[1].s.Name != "Juno octave bass" {
		t.Fatalf("a click on Bass shows %s, its first sound %s", b.group, b.sounds.rows[1].label)
	}
	if len(h.log) != 0 {
		t.Errorf("showing a group edits: %q", h.log)
	}
	// The Moog bass.
	si := slices.IndexFunc(b.sounds.rows, func(r listRow) bool { return r.s.Name == "Moog bass" })
	click(w, run, where(t, w, run, b.sounds, b.sounds.rowRect(si)))
	if want := []string{"begin Pick Moog bass", "changed", "end"}; !slices.Equal(h.log, want) {
		t.Errorf("a pick tells the host %q, not %q", h.log, want)
	}
	if p.sound().Name != "Moog bass" || pn.st.Name != "Moog bass" {
		t.Errorf("after the pick the synth plays %s and the view shows %s", p.sound().Name, pn.st.Name)
	}
	if !slices.Equal(h.heard, []float64{45}) {
		t.Errorf("the pick auditions %v, not a low A", h.heard)
	}
	run(40)
	if v := b.sounds.pill.Value(); math.Abs(float64(v)-float64(si)) > 0.01 {
		t.Errorf("the pill settles on row %.2f, not %d", v, si)
	}
	// The Sub bass, five rows down: the pill slides there frame by
	// frame, never more than a row and a half at once, and settles.
	sub := slices.IndexFunc(b.sounds.rows, func(r listRow) bool { return r.s.Name == "Sub bass" })
	click(w, run, where(t, w, run, b.sounds, b.sounds.rowRect(sub)))
	h.heard = h.heard[:1]
	last, frames := b.sounds.pill.Value(), 0
	for range 40 {
		run(1)
		v := b.sounds.pill.Value()
		if math.Abs(float64(v-last)) > 1.5 {
			t.Fatalf("the pill jumps from row %.2f to %.2f in one frame", last, v)
		}
		if math.Abs(float64(v)-float64(sub)) > 0.05 {
			frames++
		}
		last = v
	}
	if math.Abs(float64(last)-float64(sub)) > 0.01 || frames < 3 {
		t.Errorf("the pill settles on row %.2f, not %d, after %d frames", last, sub, frames)
	}
	h.log = nil
	click(w, run, where(t, w, run, b.sounds, b.sounds.rowRect(si)))
	run(40)
	// Picking it again plays it again, and edits nothing.
	h.log, h.heard = nil, h.heard[:1]
	click(w, run, where(t, w, run, b.sounds, b.sounds.rowRect(si)))
	if len(h.log) != 0 || len(h.heard) != 2 {
		t.Errorf("a click on the sound playing edits %q, auditions %v", h.log, h.heard)
	}
}

func TestTheSearchFindsSoundsInEveryBank(t *testing.T) {
	p, h := newSynth(t)
	w, pn, run := stage(t, p, h)
	b := pn.browser
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, q string, u *gunim.UI) {
		b.field.SetText(q, u)
		b.field.OnChange(q, u)
	})
	if err := w.Client().Patch("synth", "piano"); err != nil {
		t.Fatal(err)
	}
	run(2)
	banks := map[string]bool{}
	n := 0
	for _, r := range b.sounds.rows {
		if r.header {
			continue
		}
		n++
		banks[r.s.Bank] = true
		if !strings.Contains(strings.ToLower(r.s.Name+r.s.Group+r.s.About), "piano") {
			t.Errorf("the search for piano finds %s", r.s.Name)
		}
	}
	if n < 8 || !banks[Library] || !banks[GeneralMIDI] || !banks[Songs] {
		t.Errorf("the search for piano finds %d sounds, in banks %v", n, banks)
	}
	if i := b.groups.index(GeneralMIDI, "Piano"); b.groups.rows[i].count != 8 {
		t.Errorf("General MIDI's pianos count %d matches", b.groups.rows[i].count)
	}
	// A group clicked during a search scrolls to its sounds.
	click(w, run, where(t, w, run, b.groups, b.groups.rowRect(b.groups.index(GeneralMIDI, "Piano"))))
	run(40)
	if b.query != "piano" || b.group != "Pads & strings" {
		t.Errorf("a group clicked during a search shows %s, searching %q", b.group, b.query)
	}
	if b.ss.Offset() <= 0 {
		t.Error("a group clicked during a search leaves its sounds where they were")
	}
	if err := w.Client().Patch("synth", "zzzz"); err != nil {
		t.Fatal(err)
	}
	run(1)
	if len(b.sounds.rows) != 0 {
		t.Errorf("a search for nothing finds %d rows", len(b.sounds.rows))
	}
}

func TestTheStepsWalkTheGroupRound(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Aah choir")
	w, pn, run := stage(t, p, h)
	_ = w
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, by int, u *gunim.UI) { pn.browser.step(by, u) })
	for _, c := range []struct {
		by   int
		want string
	}{{1, "String machine"}, {1, "Juno pad"}, {-1, "String machine"}, {-1, "Aah choir"}} {
		if err := w.Client().Patch("synth", c.by); err != nil {
			t.Fatal(err)
		}
		run(1)
		if p.sound().Name != c.want {
			t.Errorf("a step by %d plays %s, not %s", c.by, p.sound().Name, c.want)
		}
	}
}

func TestTheViewsFollowAStateSetElsewhere(t *testing.T) {
	p, h := newSynth(t)
	w, pn, run := stage(t, p, h)
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, name string, u *gunim.UI) {
		s, _ := Named(name)
		_ = p.SetState(s.State())
		pn.FollowState(u)
	})
	if err := w.Client().Patch("synth", "TR-808"); err != nil {
		t.Fatal(err)
	}
	// The name turns over: the old one fades as the new one comes in,
	// each frame on from the one before.
	last := float32(-1)
	for range 30 {
		run(1)
		v := pn.turn.Value()
		if v < last-0.05 {
			t.Fatalf("the name turns back, from %.2f to %.2f", last, v)
		}
		last = v
	}
	b := pn.browser
	if pn.st.Name != "TR-808" || b.group != "Drum kits" || b.sounds.index(p.sound()) < 0 || pn.keys.low != 35 {
		t.Errorf("after the state is set elsewhere, the view shows %s, its browser %s, its keys from %d", pn.st.Name, b.group, pn.keys.low)
	}
	if !knob(pn, AttackID).Disabled {
		t.Error("the drums' envelope knobs stay lit")
	}
}

func TestTheKeysAuditionTheirNotes(t *testing.T) {
	p, h := newSynth(t)
	w, pn, run := stage(t, p, h)
	if pn.keys == nil || !pn.shown {
		t.Fatal("a host that auditions gets no keys")
	}
	// Middle C is the 13th key from C3; C#4 the 14th, a black key.
	for _, c := range []struct {
		i    int
		want float64
	}{{12, 60}, {13, 61}, {0, 48}} {
		r := pn.keys.rect(c.i)
		at := where(t, w, run, pn.keys, r)
		at.X += (r.Min.X+r.Max.X)/2 - 40
		at.Y = at.Y - (r.Min.Y+r.Max.Y)/2 + r.Min.Y + r.Size().H*0.3
		h.heard = nil
		click(w, run, at)
		if !slices.Equal(h.heard, []float64{c.want}) {
			t.Errorf("key %d auditions %v, not %v", c.i, h.heard, c.want)
		}
	}
}

// holdHost is a host for tests that holds notes, as a [plug.Holder],
// and logs each note held and let go.
type holdHost struct {
	*host
	notes []string
	// down counts the notes held now.
	down int
}

// Hold implements [plug.Holder].
func (h *holdHost) Hold(key int, pitch, _ float64, _ *gunim.UI) {
	h.notes = append(h.notes, fmt.Sprintf("hold %d %g", key, pitch))
	h.down++
}

// Release implements [plug.Holder].
func (h *holdHost) Release(key int, _ *gunim.UI) {
	h.notes = append(h.notes, fmt.Sprintf("release %d", key))
	h.down--
}

func TestTheKeysHoldTheirNotesAndGlide(t *testing.T) {
	p, h := newSynth(t)
	hh := &holdHost{host: h}
	w, pn, run := stage(t, p, hh)
	k := pn.keys
	var kb geom.Rect
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { kb, _ = u.Bounds(k) })
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	// A white key's middle, low on it, under the black keys.
	white := func(i int) geom.Point {
		r := k.rect(i)
		return kb.Min.Add(geom.Pt((r.Min.X+r.Max.X)/2, r.Max.Y-8))
	}
	// frame runs a frame, and checks one note at most sounds.
	frame := func() {
		run(1)
		if hh.down < 0 || hh.down > 1 {
			t.Fatalf("%d notes are held at once: %v", hh.down, hh.notes)
		}
	}
	moveTo := func(to geom.Point, from geom.Point) {
		for x := from.X; x != to.X; {
			x = min(x+16, to.X)
			w.Input(input.PointerMove{Pos: geom.Pt(x, to.Y)})
			frame()
		}
	}
	// C4, held: it sounds until the button comes up.
	at := white(12)
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	for range 30 {
		frame()
	}
	if !slices.Equal(hh.notes, []string{"hold 60 60"}) || k.lit[12].Value() < 0.99 {
		t.Fatalf("C4 held for half a second plays %v, lit %.2f", hh.notes, k.lit[12].Value())
	}
	// Dragged to E4, it plays D4 and E4 on the way, each letting the
	// one before go.
	moveTo(white(16), at)
	want := []string{"hold 60 60", "release 60", "hold 62 62", "release 62", "hold 64 64"}
	if !slices.Equal(hh.notes, want) {
		t.Fatalf("a drag from C4 to E4 plays %v, not %v", hh.notes, want)
	}
	w.Input(input.PointerUp{Pos: white(16), Button: input.ButtonPrimary})
	frame()
	if hh.down != 0 || hh.notes[len(hh.notes)-1] != "release 64" {
		t.Fatalf("the button up leaves %v", hh.notes)
	}
	// Off the keyboard, the note lets go; a second button ends the
	// press, and the first coming up after sends nothing more.
	hh.notes = nil
	at = white(0)
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	frame()
	w.Input(input.PointerMove{Pos: geom.Pt(at.X, kb.Min.Y-30)})
	frame()
	if hh.down != 0 {
		t.Fatalf("dragged off the keys, %v", hh.notes)
	}
	w.Input(input.PointerMove{Pos: at})
	frame()
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonSecondary, Clicks: 1})
	frame()
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonSecondary})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	frame()
	want = []string{"hold 48 48", "release 48", "hold 48 48", "release 48"}
	if !slices.Equal(hh.notes, want) || hh.down != 0 {
		t.Fatalf("off the keys and back, then a second button, plays %v, not %v", hh.notes, want)
	}
	// The key lit fades once let go.
	for range 60 {
		frame()
	}
	if v := k.lit[0].Value(); v > 0.05 {
		t.Errorf("a key let go a second ago is lit at %.2f", v)
	}
}
