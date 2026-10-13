package plugsynth

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// keysHost is a host for tests that holds notes and says which keys
// sound, as a [plug.KeysHost].
type keysHost struct {
	holdHost
	sounding plug.Keys
}

// Keys implements [plug.KeysHost].
func (h *keysHost) Keys() plug.Keys { return h.sounding }

// keysOf returns the set of notes ns.
func keysOf(ns ...int) plug.Keys {
	var k plug.Keys
	for _, n := range ns {
		k = k.With(n)
	}
	return k
}

// nodeBounds returns where n stands in the window.
func nodeBounds(t *testing.T, w *gunim.Window, run func(int), n gunim.Node) geom.Rect {
	t.Helper()
	var b geom.Rect
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) { b, _ = u.Bounds(n) })
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return b
}

func TestTheKeysLightAsTheSoundPlaysThem(t *testing.T) {
	p, h := newSynth(t)
	kh := &keysHost{holdHost: holdHost{host: h}}
	_, pn, run := stage(t, p, kh)
	k := pn.keys
	// C4 and E4 sound, from the song or a MIDI keyboard: both light at
	// once, and the others stay dark.
	kh.sounding = keysOf(60, 64)
	run(1)
	for n := range 128 {
		want := n == 60 || n == 64
		if lit := k.lit[n].Value() > 0.99; lit != want {
			t.Fatalf("with C4 and E4 sounding, key %d is lit %.2f", n, k.lit[n].Value())
		}
	}
	// E4 lets go: it fades, frame by frame, never brighter than the
	// frame before, while C4 stays lit.
	kh.sounding = keysOf(60)
	last := float32(1)
	for range 60 {
		run(1)
		v := k.lit[64].Value()
		if v > last+1e-4 {
			t.Fatalf("E4 brightens as it fades, from %.3f to %.3f", last, v)
		}
		last = v
		if k.lit[60].Value() < 0.99 {
			t.Fatal("C4 dims while it sounds")
		}
	}
	if last > 0.05 {
		t.Errorf("E4, let go a second ago, is lit %.2f", last)
	}
	// A key the sound plays below the keys shown lights too, for when
	// the keys slide to it.
	kh.sounding = keysOf(60, 30)
	run(1)
	if k.lit[30].Value() < 0.99 {
		t.Error("a key off the keys shown stays dark")
	}
}

func TestTheKeysShiftAnOctaveByButtonOrKey(t *testing.T) {
	p, h := newSynth(t)
	kh := &keysHost{holdHost: holdHost{host: h}}
	w, pn, run := stage(t, p, kh)
	k := pn.keys
	kb := nodeBounds(t, w, run, k)
	up := kb.Min.Add(geom.Pt(k.size.W-shiftW/2, k.size.H/2))
	// The up button: the keys slide an octave up, a little each frame,
	// never back, and settle.
	click(w, run, up)
	if k.low != 60 {
		t.Fatalf("the up button leaves the keys from %d", k.low)
	}
	last := k.slide.Value()
	for range 40 {
		run(1)
		v := k.slide.Value()
		if v < last-1e-3 || v-last > 3 {
			t.Fatalf("the keys slide from white key %.2f to %.2f in a frame", last, v)
		}
		last = v
	}
	if last != float32(whiteOf(60)) {
		t.Fatalf("the keys settle at white key %.2f, not %d", last, whiteOf(60))
	}
	// A key pressed now plays its note an octave up.
	white := func(i int) geom.Point {
		r := k.rect(i)
		return kb.Min.Add(geom.Pt((r.Min.X+r.Max.X)/2, r.Max.Y-8))
	}
	click(w, run, white(0))
	if !slices.Equal(kh.notes, []string{"hold 60 60", "release 60"}) {
		t.Fatalf("the lowest key, an octave up, plays %v", kh.notes)
	}
	// The click gave the panel the keyboard: Z goes down, X up, and
	// Ctrl+Z is the host's.
	key := func(k input.Key, m input.Mods) {
		w.Input(input.KeyPress{Key: k, Mods: m})
		run(1)
	}
	key(input.KeyZ, 0)
	key(input.KeyZ, 0)
	if k.low != 36 {
		t.Fatalf("Z twice leaves the keys from %d, not 36", k.low)
	}
	key(input.KeyZ, input.ModControl)
	if k.low != 36 {
		t.Fatalf("Ctrl+Z shifts the keys, to %d", k.low)
	}
	key(input.KeyX, 0)
	if k.low != 48 {
		t.Fatalf("X leaves the keys from %d, not 48", k.low)
	}
	// At the top, X does nothing, and the up button greys.
	for range 10 {
		key(input.KeyX, 0)
	}
	if k.low != 96 || !k.up.Disabled || k.down.Disabled {
		t.Fatalf("X ten times leaves the keys from %d, up greyed %v, down %v", k.low, k.up.Disabled, k.down.Disabled)
	}
	for range 20 {
		key(input.KeyZ, 0)
	}
	if k.low != 0 || !k.down.Disabled {
		t.Fatalf("Z twenty times leaves the keys from %d, down greyed %v", k.low, k.down.Disabled)
	}
}

func TestAShiftLetsGoOfTheKeyHeld(t *testing.T) {
	p, h := newSynth(t)
	kh := &keysHost{holdHost: holdHost{host: h}}
	w, pn, run := stage(t, p, kh)
	k := pn.keys
	kb := nodeBounds(t, w, run, k)
	r := k.rect(12)
	at := kb.Min.Add(geom.Pt((r.Min.X+r.Max.X)/2, r.Max.Y-8))
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyX})
	run(1)
	if kh.down != 0 || k.held != -1 || k.low != 60 {
		t.Fatalf("X with C4 held leaves %d notes held, key %d, keys from %d: %v", kh.down, k.held, k.low, kh.notes)
	}
	// The press goes on: a move holds the key now under the pointer.
	at.X += 1
	w.Input(input.PointerMove{Pos: at})
	run(1)
	if kh.down != 1 || k.held < 60 {
		t.Fatalf("a move after the shift holds key %d: %v", k.held, kh.notes)
	}
	// A second button ends the press.
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonSecondary, Clicks: 1})
	run(1)
	if kh.down != 0 || k.pressing {
		t.Fatalf("a second button leaves %d notes held, pressing %v", kh.down, k.pressing)
	}
}

func TestAKitStartsItsKeysAtItsOwnOctave(t *testing.T) {
	p, h := newSynth(t)
	_, pn, run := stage(t, p, &keysHost{holdHost: holdHost{host: h}})
	k := pn.keys
	k.shift, k.low = 2, 72
	k.follow("drums")
	run(1)
	if k.low != 35 || k.shift != 0 || k.slide.Value() != float32(whiteOf(35)) {
		t.Errorf("a kit's keys start from %d, shifted %d, at white key %.1f", k.low, k.shift, k.slide.Value())
	}
}
