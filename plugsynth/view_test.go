package plugsynth

import (
	"fmt"
	"slices"
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

func TestTheStepsWalkTheGroupRound(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Aah choir")
	w, pn, run := stage(t, p, h)
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, by int, u *gunim.UI) { pn.step(by, u) })
	for _, c := range []struct {
		by   int
		want string
	}{{1, "String machine"}, {1, "Juno pad"}, {-1, "String machine"}, {-1, "Aah choir"}} {
		h.log, h.heard = nil, nil
		if err := w.Client().Patch("synth", c.by); err != nil {
			t.Fatal(err)
		}
		run(1)
		if p.sound().Name != c.want || pn.st.Name != c.want {
			t.Errorf("a step by %d plays %s, shows %s, not %s", c.by, p.sound().Name, pn.st.Name, c.want)
		}
		if want := []string{"begin Pick " + c.want, "changed", "end"}; !slices.Equal(h.log, want) {
			t.Errorf("a step tells the host %q, not %q", h.log, want)
		}
		if !slices.Equal(h.heard, []float64{60}) {
			t.Errorf("a step to %s auditions %v, not middle C", c.want, h.heard)
		}
	}
	// A bass auditions a low A, and the steps keep to the Bass group.
	play(t, p, "Moog bass")
	h.heard = nil
	if err := w.Client().Patch("synth", 1); err != nil {
		t.Fatal(err)
	}
	run(1)
	if st := p.sound(); st.Group != "Bass" || st.Name == "Moog bass" || !slices.Equal(h.heard, []float64{45}) {
		t.Errorf("a step from the Moog bass plays %s/%s, auditions %v", st.Group, st.Name, h.heard)
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
	if pn.st.Name != "TR-808" || pn.st.Group != "Drum kits" || pn.keys.low != 35 {
		t.Errorf("after the state is set elsewhere, the view shows %s/%s, its keys from %d", pn.st.Group, pn.st.Name, pn.keys.low)
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
	if !slices.Equal(hh.notes, []string{"hold 60 60"}) || k.lit[60].Value() < 0.99 {
		t.Fatalf("C4 held for half a second plays %v, lit %.2f", hh.notes, k.lit[60].Value())
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
	if v := k.lit[48].Value(); v > 0.05 {
		t.Errorf("a key let go a second ago is lit at %.2f", v)
	}
}
