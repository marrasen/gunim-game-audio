package plugfx

import (
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

// host is a host for tests, which keeps the parameters it is sent and
// notes the gestures.
type host struct {
	d      *plug.Descriptor
	values map[uint32]float64
	log    []string
}

func newHost(d *plug.Descriptor) *host {
	h := &host{d: d, values: map[uint32]float64{}}
	for _, p := range d.Params {
		h.values[p.ID] = p.Default
	}
	return h
}

func (h *host) Param(id uint32) float64 { return h.values[id] }
func (h *host) Begin(id uint32, _ *gunim.UI) {
	h.log = append(h.log, "begin "+h.name(id))
}
func (h *host) Set(id uint32, v float64, _ *gunim.UI) {
	h.values[id] = v
	h.log = append(h.log, "set "+h.name(id))
}
func (h *host) End(id uint32, _ *gunim.UI)           { h.log = append(h.log, "end "+h.name(id)) }
func (h *host) BeginEdit(name string, _ *gunim.UI)   { h.log = append(h.log, "edit "+name) }
func (h *host) Changed(*gunim.UI)                    { h.log = append(h.log, "changed") }
func (h *host) EndEdit(*gunim.UI)                    { h.log = append(h.log, "end edit") }
func (h *host) Sound() (in, out plug.Sound)          { return nil, nil }
func (h *host) Traced(uint32, int64) (float32, bool) { return 0, false }

func (h *host) name(id uint32) string {
	p, _ := h.d.Param(id)
	return p.Name
}

// gestures returns the log without its sets, which a drag sends many of.
func (h *host) gestures() []string {
	return slices.DeleteFunc(slices.Clone(h.log), func(s string) bool { return strings.HasPrefix(s, "set ") })
}

// stage mounts v in an offscreen window of size, and returns the window
// and a call that runs n frames of it.
func stage(t *testing.T, size geom.Size, v gunim.Node) (w *gunim.Window, run func(int)) {
	t.Helper()
	w = gunimtest.New(t, size, nil)
	gunim.RegisterView(w, "fx", func(struct{}) gunim.Node { return v }, nil)
	if err := w.Client().Mount(gunim.Root, "fx", "fx", nil); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, run
}

// withUI runs fn on the window's goroutine, with its UI.
func withUI(t *testing.T, w *gunim.Window, run func(int), fn func(u *gunim.UI)) {
	t.Helper()
	gunim.RegisterPatch(w, "fx", func(_ gunim.Node, _ struct{}, u *gunim.UI) { fn(u) })
	if err := w.Client().Patch("fx", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
}

// boundsIn returns where n is in w.
func boundsIn(t *testing.T, w *gunim.Window, run func(int), n gunim.Node) geom.Rect {
	t.Helper()
	var b geom.Rect
	withUI(t, w, run, func(u *gunim.UI) { b, _ = u.Bounds(n) })
	return b
}

// click clicks at pos.
func click(w *gunim.Window, run func(int), pos geom.Point) {
	w.Input(input.PointerMove{Pos: pos})
	w.Input(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: pos, Button: input.ButtonPrimary})
	run(1)
}

var panelSize = geom.Sz(960, 460)

// as returns v as a T, and fails t where it is another type.
func as[T any](t *testing.T, v any) T {
	t.Helper()
	x, ok := v.(T)
	if !ok {
		t.Fatalf("%T is no %T", v, x)
	}
	return x
}

func TestEveryViewShowsAtEachSize(t *testing.T) {
	for _, d := range []*plug.Descriptor{&ReverbDescriptor, &DelayDescriptor, &ColourDescriptor} {
		h := newHost(d)
		p := as[plug.Viewer](t, d.New(h))
		for _, s := range []plug.Size{plug.Chip, plug.Panel, plug.Full} {
			v := p.View(s, h)
			if v == nil {
				t.Fatalf("%s has no view at size %d", d.Name, s)
			}
			size := panelSize
			if s == plug.Chip {
				size = geom.Sz(160, 24)
			}
			w, run := stage(t, size, v)
			run(10)
			pn, ok := v.(*panel)
			if !ok {
				continue
			}
			for _, sec := range pn.sections {
				r := boundsIn(t, w, run, sec)
				if r.Empty() || r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > size.W || r.Max.Y > size.H {
					t.Errorf("%s at size %d: section %s is at %v, outside the view", d.Name, s, sec.Name, r)
				}
			}
		}
	}
}

func TestTheReverbsDecayHandleDragsAsOneGesture(t *testing.T) {
	h := newHost(&ReverbDescriptor)
	pn := as[*panel](t, as[plug.Viewer](t, ReverbDescriptor.New(h)).View(plug.Panel, h))
	g := as[*decayGraph](t, pn.disp)
	w, run := stage(t, panelSize, pn)
	run(40)
	at := boundsIn(t, w, run, g).Min.Add(g.handleAt(handleDecay))
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	was := h.values[DecayID]
	last := was
	// In small steps, as a hand moves: the decay grows each step, and
	// the handle stays under the pointer as the span glides.
	for k := 1; k <= 10; k++ {
		pos := at.Add(geom.Pt(float32(k)*16, 0))
		w.Input(input.PointerMove{Pos: pos})
		run(1)
		if v := h.values[DecayID]; v < last {
			t.Fatalf("step %d: the decay falls to %g from %g, the pointer moving right", k, v, last)
		}
		last = h.values[DecayID]
	}
	w.Input(input.PointerUp{Pos: at.Add(geom.Pt(160, 0)), Button: input.ButtonPrimary})
	run(1)
	if last <= was {
		t.Errorf("dragged right, the decay is %g, from %g", last, was)
	}
	if got := h.gestures(); !slices.Equal(got, []string{"begin Decay", "end Decay"}) {
		t.Errorf("the drag sent %v", got)
	}
	// The span glides out to fit the longer decay.
	run(60)
	if g.span.Value() < float32(g.v[DecayID]) {
		t.Errorf("the graph spans %g s, short of the decay of %g s", g.span.Value(), g.v[DecayID])
	}
}

// secondButtonEnds presses the primary button on what is at at, moves
// it by by, presses the secondary, and moves on: the drag ends with the
// second press, and moving on changes nothing.
func secondButtonEnds(t *testing.T, w *gunim.Window, run func(int), h *host, at, by geom.Point, id uint32) {
	t.Helper()
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerMove{Pos: at.Add(by)})
	run(1)
	w.Input(input.PointerDown{Pos: at.Add(by), Button: input.ButtonSecondary, Clicks: 1})
	run(1)
	v := h.values[id]
	w.Input(input.PointerMove{Pos: at.Add(by).Add(by)})
	run(1)
	if h.values[id] != v {
		t.Errorf("after the second button, the drag moves the value from %g to %g", v, h.values[id])
	}
	if g := h.gestures(); len(g) < 2 || !strings.HasPrefix(g[len(g)-1], "end ") {
		t.Errorf("the second button leaves the gestures at %v, unended", g)
	}
	w.Input(input.PointerUp{Pos: at.Add(by), Button: input.ButtonSecondary})
	w.Input(input.PointerUp{Pos: at.Add(by), Button: input.ButtonPrimary})
	run(1)
}

func TestASecondButtonEndsADisplaysDrag(t *testing.T) {
	h := newHost(&ReverbDescriptor)
	pn := as[*panel](t, as[plug.Viewer](t, ReverbDescriptor.New(h)).View(plug.Panel, h))
	g := as[*decayGraph](t, pn.disp)
	w, run := stage(t, panelSize, pn)
	run(40)
	secondButtonEnds(t, w, run, h, boundsIn(t, w, run, g).Min.Add(g.handleAt(handlePre)), geom.Pt(40, 0), PreDelayID)
	if g.grip.held != 0 {
		t.Error("the graph still holds a handle")
	}

	h = newHost(&DelayDescriptor)
	pn = as[*panel](t, as[plug.Viewer](t, DelayDescriptor.New(h)).View(plug.Panel, h))
	tp := as[*taps](t, pn.disp)
	w, run = stage(t, panelSize, pn)
	run(40)
	secondButtonEnds(t, w, run, h, boundsIn(t, w, run, tp).Min.Add(tp.handleAt(handleFeedback)), geom.Pt(0, -30),
		FeedbackID)
	if tp.grip.held != 0 {
		t.Error("the echoes still hold a handle")
	}
}

func TestTheDelaysTimeHandleSnapsToNotes(t *testing.T) {
	h := newHost(&DelayDescriptor)
	pn := as[*panel](t, as[plug.Viewer](t, DelayDescriptor.New(h)).View(plug.Panel, h))
	tp := as[*taps](t, pn.disp)
	w, run := stage(t, panelSize, pn)
	run(40)
	origin := boundsIn(t, w, run, tp).Min
	at := origin.Add(tp.handleAt(handleTime))
	// To where a quarter note stands: a beat.
	to := origin.Add(geom.Pt(tp.xOf(1), at.Y-origin.Y))
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	for k := 1; k <= 8; k++ {
		w.Input(input.PointerMove{Pos: geom.Pt(at.X+(to.X-at.X)*float32(k)/8, at.Y)})
		run(1)
	}
	w.Input(input.PointerUp{Pos: to, Button: input.ButtonPrimary})
	run(1)
	if note, feel := h.values[NoteID], h.values[FeelID]; note != 3 || feel != 0 {
		t.Errorf("dragged to a beat, the note is %s %s", NoteNames[int(note)], FeelNames[int(feel)])
	}
	if got := h.gestures(); !slices.Equal(got, []string{"begin Note", "begin Feel", "end Note", "end Feel"}) {
		t.Errorf("the drag sent %v", got)
	}
}

// chainStage stages Colour's panel on units.
func chainStage(t *testing.T, units []Unit) (*colourPlugin, *host, *chainPanel, *gunim.Window, func(int)) {
	t.Helper()
	h := newHost(&ColourDescriptor)
	p := as[*colourPlugin](t, ColourDescriptor.New(h))
	p.setUnits(units)
	pn := as[*chainPanel](t, p.View(plug.Panel, h))
	w, run := stage(t, geom.Sz(1200, 460), pn)
	run(40)
	return p, h, pn, w, run
}

func TestDraggingACardReordersTheChain(t *testing.T) {
	units := []Unit{{Kind: Chorus}, {Kind: Phaser, Slot: 1}, {Kind: Distortion, Slot: 2}}
	p, h, pn, w, run := chainStage(t, units)
	first, last := pn.cards[0], pn.cards[2]
	from := boundsIn(t, w, run, first)
	to := boundsIn(t, w, run, last)
	grab := geom.Pt(from.Min.X+30, from.Min.Y+10)
	w.Input(input.PointerMove{Pos: grab})
	w.Input(input.PointerDown{Pos: grab, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	// Across in steps of 16: the card stays under the pointer, and the
	// others never jump, each frame.
	others := []*unitCard{pn.cards[1], pn.cards[2]}
	was := []float32{others[0].x.Value(), others[1].x.Value()}
	dx := to.Max.X - 20 - grab.X
	steps := int(dx / 16)
	for k := 1; k <= steps; k++ {
		pos := grab.Add(geom.Pt(float32(k)*16, 0))
		w.Input(input.PointerMove{Pos: pos})
		run(1)
		if got := first.x.Value(); got != pos.X-30 {
			t.Fatalf("step %d: the card is at %g, the pointer at %g", k, got, pos.X)
		}
		for i, o := range others {
			if d := o.x.Value() - was[i]; d > 1 || d < -60 {
				t.Fatalf("step %d: card %d moves %g in a frame", k, i+2, d)
			}
			was[i] = o.x.Value()
		}
	}
	w.Input(input.PointerUp{Pos: grab.Add(geom.Pt(float32(steps)*16, 0)), Button: input.ButtonPrimary})
	run(1)
	want := []Unit{{Kind: Phaser, Slot: 1}, {Kind: Distortion, Slot: 2}, {Kind: Chorus}}
	if got := p.Units(); !slices.Equal(got, want) {
		t.Errorf("dropped last, the chain is %v", got)
	}
	if got := h.gestures(); !slices.Equal(got, []string{"edit Move chorus", "changed", "end edit"}) {
		t.Errorf("the move sent %v", got)
	}
	// The card dropped glides from where it was let go to its place.
	at := first.x.Value()
	run(60)
	if first.x.Value() == at && at != first.to.X {
		t.Error("the card dropped stays where it was let go")
	}
	if first.x.Value() != first.to.X {
		t.Errorf("the card dropped settles at %g, its place at %g", first.x.Value(), first.to.X)
	}
}

func TestASecondButtonDropsTheCard(t *testing.T) {
	units := []Unit{{Kind: Chorus}, {Kind: Phaser, Slot: 1}}
	p, _, pn, w, run := chainStage(t, units)
	c := pn.cards[0]
	b := boundsIn(t, w, run, c)
	grab := geom.Pt(b.Min.X+30, b.Min.Y+10)
	w.Input(input.PointerMove{Pos: grab})
	w.Input(input.PointerDown{Pos: grab, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	far := grab.Add(geom.Pt(b.Size().W+cardGap+40, 0))
	for x := grab.X; x < far.X; x += 16 {
		w.Input(input.PointerMove{Pos: geom.Pt(x, grab.Y)})
		run(1)
	}
	w.Input(input.PointerDown{Pos: far, Button: input.ButtonSecondary, Clicks: 1})
	run(1)
	if pn.dragged != nil {
		t.Fatal("the second button leaves the card dragged")
	}
	want := []Unit{{Kind: Phaser, Slot: 1}, {Kind: Chorus}}
	if got := p.Units(); !slices.Equal(got, want) {
		t.Errorf("dropped by the second button, the chain is %v", got)
	}
	// Moving on, the card settles in its place, away from the pointer.
	w.Input(input.PointerMove{Pos: far.Add(geom.Pt(-300, 0))})
	run(60)
	if c.x.Value() != c.to.X {
		t.Errorf("after the second button, the card stands at %g, not its place at %g", c.x.Value(), c.to.X)
	}
	w.Input(input.PointerUp{Pos: far, Button: input.ButtonSecondary})
	w.Input(input.PointerUp{Pos: far, Button: input.ButtonPrimary})
	run(1)
}

func TestTheAddAndRemoveButtonsEditTheChain(t *testing.T) {
	p, h, pn, w, run := chainStage(t, []Unit{{Kind: Chorus}})
	click(w, run, boundsIn(t, w, run, pn.add.buttons[1]).Center())
	want := []Unit{{Kind: Chorus}, {Kind: Phaser, Slot: 1}}
	if got := p.Units(); !slices.Equal(got, want) {
		t.Fatalf("a phaser added makes %v", got)
	}
	if got := h.gestures(); !slices.Equal(got, []string{"edit Add phaser", "changed", "end edit"}) {
		t.Errorf("the add sent %v", got)
	}
	// The new card grows in, frame by frame.
	c := pn.cards[1]
	last := c.appear.Value()
	for k := range 30 {
		run(1)
		// A spring may overshoot a little, and settle back.
		if v := c.appear.Value(); v < min(last, 1)-1e-3 {
			t.Fatalf("frame %d: the new card shrinks from %g to %g", k, last, v)
		}
		last = c.appear.Value()
	}
	if last < 0.95 {
		t.Errorf("the new card has grown to %g", last)
	}
	h.log = nil
	click(w, run, boundsIn(t, w, run, pn.cards[0].remove).Center())
	if got := p.Units(); !slices.Equal(got, []Unit{{Kind: Phaser, Slot: 1}}) {
		t.Fatalf("the chorus removed leaves %v", got)
	}
	if got := h.gestures(); !slices.Equal(got, []string{"edit Remove chorus", "changed", "end edit"}) {
		t.Errorf("the remove sent %v", got)
	}
	// The phaser slides into the chorus's place, without a jump.
	pc := pn.cards[0]
	lastX := pc.x.Value()
	for k := range 40 {
		run(1)
		if d := pc.x.Value() - lastX; d > 1 || d < -80 {
			t.Fatalf("frame %d: the phaser's card moves %g", k, d)
		}
		lastX = pc.x.Value()
	}
	if pc.x.Value() != pc.to.X {
		t.Errorf("the phaser's card settles at %g, its place at %g", pc.x.Value(), pc.to.X)
	}
}

func TestTheCardsFollowAChainSetElsewhere(t *testing.T) {
	// From distortion, phaser, chorus to chorus, phaser, as an undo
	// sets it: the chorus's and the phaser's cards are kept, and glide
	// to their places, frame by frame.
	p, _, pn, w, run := chainStage(t, chains[3])
	chorus, phaser := pn.cards[2], pn.cards[1]
	if err := p.SetState(ColourState{Units: chains[5]}.Encode()); err != nil {
		t.Fatal(err)
	}
	withUI(t, w, run, func(u *gunim.UI) { pn.FollowState(u) })
	if len(pn.cards) != 2 || pn.cards[0] != chorus || pn.cards[1] != phaser {
		t.Fatalf("the cards are %v, not the chorus's and the phaser's kept", pn.cards)
	}
	if chorus.section.Name != "1 · CHORUS" {
		t.Errorf("the chorus's card is named %q", chorus.section.Name)
	}
	last := chorus.x.Value()
	for k := range 60 {
		run(1)
		if d := chorus.x.Value() - last; d > 1 || d < -80 {
			t.Fatalf("frame %d: the chorus's card moves %g", k, d)
		}
		last = chorus.x.Value()
	}
	if chorus.x.Value() != chorus.to.X || chorus.to.X >= phaser.to.X {
		t.Errorf("the chorus settles at %g, its place %g, the phaser's %g", chorus.x.Value(), chorus.to.X, phaser.to.X)
	}
}

func TestTheChipNamesTheUnits(t *testing.T) {
	h := newHost(&ColourDescriptor)
	p := as[*colourPlugin](t, ColourDescriptor.New(h))
	p.setUnits(chains[3])
	c := as[*colourChip](t, p.View(plug.Chip, h))
	if got := c.Access().Name; got != "Colour: Fuzz, Phaser, Juno I" {
		t.Errorf("the chip reads %q", got)
	}
}
