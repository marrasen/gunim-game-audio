package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/midi"
)

func demoFile(t *testing.T) *midi.File {
	t.Helper()
	f, err := midi.Read(bytes.NewReader(demoSong()))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTheDemoMakesAScore(t *testing.T) {
	sc := newScore(demoFile(t), "demo")
	if got := len(sc.used()); got != 13 {
		t.Errorf("the demo plays %d channels, want 13", got)
	}
	if sc.BPM < 111 || sc.BPM > 113 || sc.BarBeats != 4 {
		t.Errorf("the demo is at %v BPM in %d, want 112 in 4", sc.BPM, sc.BarBeats)
	}
	if len(sc.Beats) < 24*4 {
		t.Errorf("the demo has %d beats, want 96 or more", len(sc.Beats))
	}
	for i := 1; i < len(sc.Notes); i++ {
		if sc.Notes[i].Start < sc.Notes[i-1].Start {
			t.Fatalf("note %d starts before the one before it", i)
		}
	}
	// The piano strikes its first chord at bar 5: three notes sound then.
	bar := 60.0 / 112 * 4
	n := 0
	sc.span(4*bar+0.1, 4*bar+0.1, func(nt *Note) {
		if nt.Ch == 0 {
			n++
		}
	})
	if n != 3 {
		t.Errorf("the piano sounds %d notes in bar 5, want 3", n)
	}
	if b, ph := sc.beatAt(bar + 60.0/112/2); b != 4 || ph < 0.45 || ph > 0.55 {
		t.Errorf("half a beat into bar 2 is beat %d, %.2f through, want beat 4, 0.5", b, ph)
	}
	if got := sc.programAt(3, 10); got != 81 {
		t.Errorf("channel 4 plays program %d, want 81", got)
	}
}

// newTestApp returns an app with a mixer no speaker plays, and mix,
// which plays d of it.
func newTestApp(t *testing.T) (*app, func(d time.Duration)) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(1200, 800), widget.NewSurface())
	mix := audio.NewMixer()
	a := newApp(w.Client(), mix)
	buf := make([]float32, 2*480)
	return a, func(d time.Duration) {
		for n := audio.Frames(d); n > 0; n -= 480 {
			mix.Mix(buf)
		}
	}
}

func TestTheAppPlaysAndControlsTheSong(t *testing.T) {
	a, play := newTestApp(t)
	ctx := context.Background()
	a.handle(ctx, DemoAsked{})
	if a.cur != 0 || a.score == nil || !a.playing() {
		t.Fatalf("the demo does not play: cur %d", a.cur)
	}
	play(2 * time.Second)
	if h := a.heard(); h < 1.9 || h > 2.1 {
		t.Errorf("after 2 s, %.2f s is heard", h)
	}
	a.handle(ctx, StyleChosen{Style: "nes"})
	a.handle(ctx, StyleChosen{Style: "amiga"})
	if a.style != "nes" || a.p.GM().Style() != "nes" {
		t.Errorf("the style is %q, the synth's %q, want nes", a.style, a.p.GM().Style())
	}
	a.handle(ctx, ChannelClicked{Channel: 2})
	if !a.p.GM().Muted(2) || a.p.GM().Muted(3) {
		t.Error("a click on channel 3 did not mute it alone")
	}
	a.handle(ctx, ChannelClicked{Channel: 5, Solo: true})
	for ch := range 16 {
		if want := ch != 5; a.p.GM().Muted(ch) != want {
			t.Errorf("with channel 6 alone, channel %d muted %v", ch+1, !want)
		}
	}
	a.handle(ctx, ChannelClicked{Channel: 5, Solo: true})
	a.handle(ctx, SpeedSet{Speed: 2})
	a.handle(ctx, Sought{Time: 30})
	play(time.Second)
	if h := a.heard(); h < 31.8 || h > 32.2 {
		t.Errorf("at twice the speed from 30 s, a second later %.2f s is heard", h)
	}
	a.handle(ctx, PlayToggled{})
	if a.playing() || a.state().Clock.Rate != 0 {
		t.Error("pause does not pause")
	}
	a.handle(ctx, PlayToggled{})
	if !a.playing() {
		t.Error("play does not play again")
	}
}

func TestTheNextSongFollowsTheLast(t *testing.T) {
	a, play := newTestApp(t)
	ctx := context.Background()
	a.handle(ctx, DemoAsked{})
	a.handle(ctx, DemoAsked{})
	if len(a.list) != 2 || a.cur != 1 {
		t.Fatalf("two demos: %d on the list, playing %d", len(a.list), a.cur)
	}
	a.handle(ctx, Picked{Index: 0})
	a.handle(ctx, SpeedSet{Speed: 3})
	a.handle(ctx, Sought{Time: 53})
	for range 60 {
		play(100 * time.Millisecond)
		a.follow()
		if a.cur == 1 {
			break
		}
	}
	if a.cur != 1 {
		t.Fatalf("after the first song ends, song %d plays, want 2", a.cur+1)
	}
	a.handle(ctx, Removed{Index: 1})
	if len(a.list) != 1 || a.cur != 0 {
		t.Errorf("the song playing taken off: %d left, playing %d", len(a.list), a.cur)
	}
}

func TestADroppedFileThatIsNoMIDIFileSaysSo(t *testing.T) {
	a, _ := newTestApp(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "notes.mid")
	if err := os.WriteFile(bad, []byte("not midi at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "demo.mid")
	if err := os.WriteFile(good, demoSong(), 0o644); err != nil {
		t.Fatal(err)
	}
	a.add([]string{bad})
	if len(a.list) != 0 || a.msg == "" {
		t.Fatalf("a bad file: %d on the list, message %q", len(a.list), a.msg)
	}
	a.add([]string{dir})
	if len(a.list) != 1 || a.cur != 0 || a.list[0].Name != "demo" {
		t.Fatalf("a folder: %v, playing %d", a.list, a.cur)
	}
}

// window runs the player's window offscreen, showing s.
type window struct {
	t *testing.T
	w *gunim.Window
	v *root
}

func newWindow(t *testing.T, s Player) *window {
	t.Helper()
	h := &window{t: t, w: gunimtest.New(t, geom.Sz(1360, 880), widget.NewSurface())}
	h.w.RegisterTheme(widget.Dark())
	gunim.RegisterView(h.w, "midiplayer", func(s Player) *root {
		h.v = buildView(s)
		return h.v
	}, (*root).update)
	if err := h.w.Client().Mount(gunim.Root, "midiplayer", "midiplayer", s); err != nil {
		t.Fatal(err)
	}
	h.frames(60)
	return h
}

func (h *window) frames(n int) {
	for range n {
		h.w.Frame(time.Second / 60)
	}
}

// post sends the window ev, and runs a few frames.
func (h *window) post(evs ...any) {
	for _, ev := range evs {
		h.w.Input(ev)
		h.frames(2)
	}
}

func (h *window) click(at geom.Point, mods input.Mods) {
	h.post(input.PointerMove{Pos: at}, input.PointerDown{Pos: at, Button: input.ButtonPrimary, Mods: mods, Clicks: 1},
		input.PointerUp{Pos: at, Button: input.ButtonPrimary, Mods: mods})
}

// intent returns the next intent the window sends.
func (h *window) intent() gunim.Intent {
	h.t.Helper()
	select {
	case ev := <-h.w.Client().Intents():
		return ev.Intent
	case <-time.After(time.Second):
		h.t.Fatal("the window sent no intent")
		return nil
	}
}

func demoState(t *testing.T) Player {
	sc := newScore(demoFile(t), "Neon Overture")
	return Player{Playlist: []Item{{Name: "Neon Overture", Length: sc.Length}}, Current: 0, Score: sc, ScoreGen: 1,
		Clock: Clock{Time: 20, At: time.Now(), Rate: 1}, Playing: true, Style: "gm", Solo: -1, Speed: 1, Volume: 1}
}

func TestTheWindowTakesClicksDropsAndKeys(t *testing.T) {
	h := newWindow(t, demoState(t))
	// A click on the first station, the piano's, mutes it; with Shift, it
	// plays alone.
	st := h.v.l.station[0]
	if st.Empty() {
		t.Fatal("the piano has no station")
	}
	mid := geom.Pt(st.Min.X+st.Size().W/2, st.Min.Y+st.Size().H/2)
	h.click(mid, 0)
	if in, ok := h.intent().(ChannelClicked); !ok || in.Channel != 0 || in.Solo {
		t.Errorf("a click on the piano's station sent %#v", in)
	}
	h.click(mid, input.ModShift)
	if in, ok := h.intent().(ChannelClicked); !ok || !in.Solo {
		t.Errorf("a click with Shift sent %#v", in)
	}
	// The NES's cartridge, the third on the deck, picks its style.
	db, _ := boundsOf(h, h.v.deck)
	cart := h.v.deck.cart(2)
	h.click(geom.Pt(db.Min.X+cart.Min.X+cart.Size().W/2, db.Min.Y+cart.Min.Y+20), 0)
	if in, ok := h.intent().(StyleChosen); !ok || in.Style != "nes" {
		t.Errorf("a click on the NES cartridge sent %#v", in)
	}
	// Files dropped anywhere on the window.
	paths := []string{"/music/a.mid", "/music/b.midi"}
	h.post(driver.FilesOver{Pos: geom.Pt(600, 300)})
	if h.v.drag == 0 || !h.v.dragOK {
		t.Error("a drag of files not yet named does not light the window")
	}
	h.post(driver.FilesOver{Pos: geom.Pt(600, 300), Paths: paths})
	if h.v.drag == 0 || !h.v.dragOK || h.v.dragN != 2 {
		t.Errorf("a drag of two MIDI files over the window: lit %v, ok %v, %d", h.v.drag, h.v.dragOK, h.v.dragN)
	}
	h.post(input.Drop{Pos: geom.Pt(600, 300), Data: input.Files{Paths: paths}, Paths: paths})
	if in, ok := h.intent().(FilesDropped); !ok || len(in.Paths) != 2 {
		t.Errorf("a drop sent %#v", in)
	}
	h.post(driver.FilesOver{Pos: geom.Pt(600, 300), Paths: []string{"/music/cover.jpg"}})
	if h.v.dragOK {
		t.Error("a drag of a picture is taken as MIDI files")
	}
	h.post(driver.FilesLeft{})
	if h.v.drag != 0 {
		t.Error("the drag left, and the window is still lit")
	}
	// Space plays and pauses; 4 picks the Game Boy.
	h.post(input.KeyPress{Key: input.KeySpace})
	if _, ok := h.intent().(PlayToggled); !ok {
		t.Error("Space did not play or pause")
	}
	h.post(input.KeyPress{Char: '4'})
	if in, ok := h.intent().(StyleChosen); !ok || in.Style != "gb" {
		t.Errorf("4 sent %#v", in)
	}
}

// boundsOf returns where the deck lies in the window: at the top
// right, under the title bar.
func boundsOf(h *window, d *deck) (geom.Rect, bool) {
	w := float32(1360)
	deckW := min(float32(470), w*0.45)
	return xyxy(w-margin-deckW, 6, w-margin, 6+headerH), d.size.W > 0
}

func TestEveryStyleAndNoSongDrawsWithoutFault(t *testing.T) {
	s := demoState(t)
	h := newWindow(t, s)
	for _, style := range styleOrder {
		s.Style = style
		_ = h.w.Client().Update("midiplayer", s)
		h.frames(90)
	}
	s.Score, s.ScoreGen, s.Playlist, s.Current = nil, 2, nil, -1
	_ = h.w.Client().Update("midiplayer", s)
	h.frames(30)
	if h.v.l.score != nil {
		t.Error("the window still shows a song")
	}
}
