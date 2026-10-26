package plugsynth

import (
	"bytes"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
	"github.com/marrasen/gunim-game-audio/soundfont"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// useTestFont hands the synth the test bank as f says, for the test's
// run.
func useTestFont(t *testing.T, f Font) {
	t.Helper()
	b, err := soundfont.Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	f.Font = b
	UseFont(f)
	t.Cleanup(func() { DropFont(f.Name) })
}

// heardHz plays key 72 on p for half a second, and returns how high
// its right side, where the test bank's square is panned, sounds.
func heardHz(t *testing.T, p *plugin) float64 {
	t.Helper()
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	out := plugtest.Play(t, p.Processor(), setup, 24000, params(), note(1, 72, 0, 20000)...)
	return hz(out[1][4800:19200], 48000)
}

func TestAGeneralMIDIBanksInstrumentsAreSounds(t *testing.T) {
	useTestFont(t, Font{Name: "test-gm", Title: "Test GM"})
	square, ok := Find("Test GM", "Piano", "Square")
	if !ok {
		t.Fatalf("the bank's program 0 is not filed as a piano: %v", Groups("Test GM"))
	}
	kit, ok := Find("Test GM", GMDrums, "Kit")
	if !ok || !kit.kit() || previewPitch(kit) != 36 {
		t.Errorf("the bank's kit is not a kit, previewed on its kick: %+v", kit)
	}
	if previewPitch(square) != 60 {
		t.Errorf("the square is previewed on %v, not middle C", previewPitch(square))
	}
	p, _ := newSynth(t)
	if err := p.SetState(square.State()); err != nil {
		t.Fatal(err)
	}
	// The square of 100 frames at 44100 Hz is 441 Hz at its root, 60:
	// key 72 plays it an octave up.
	if got := heardHz(t, p); math.Abs(got-882) > 10 {
		t.Errorf("the bank's square plays key 72 at %.0f Hz, not 882", got)
	}
	// Its state is a state the synth reads, as one of its own.
	if _, err := DecodeState(square.State()); err != nil {
		t.Error(err)
	}
	var banks []string
	for _, s := range Sounds() {
		if len(banks) == 0 || banks[len(banks)-1] != s.Bank {
			banks = append(banks, s.Bank)
		}
	}
	want := []string{Library, GeneralMIDI, "Test GM", Songs}
	if len(banks) != len(want) {
		t.Fatalf("the banks are %v, want %v", banks, want)
	}
	for i := range want {
		if banks[i] != want[i] {
			t.Fatalf("the banks are %v, want %v", banks, want)
		}
	}
}

func TestABankOfOneInstrumentIsARecordedSound(t *testing.T) {
	useTestFont(t, Font{Name: "test-one", Title: "Test Square", Group: "Synth", About: "A square.", Credit: "By the test."})
	s, ok := Find(Recorded, "Synth", "Test Square")
	if !ok {
		t.Fatalf("the bank is not among the recorded instruments: %v", Groups(Recorded))
	}
	if s.About != "A square. By the test." {
		t.Errorf("its sound says %q", s.About)
	}
	if f := facts(s.patch); len(f) != 2 || f[1] != "Test Square" {
		t.Errorf("its chips say %v", f)
	}
	DropFont("test-one")
	if _, ok := Find(Recorded, "Synth", "Test Square"); ok {
		t.Error("a bank let go still offers its sounds")
	}
}

func TestASynthPlaysItsBankOnceTheBankComes(t *testing.T) {
	p, _ := newSynth(t)
	st := State{Bank: Recorded, Group: "Synth", Name: "Late", Patch: &synth.Patch{SoundFont: "test-late"}}
	if err := p.SetState(st.Encode()); err != nil {
		t.Fatal(err)
	}
	// With no bank, General MIDI's piano plays key 72: 523 Hz.
	if got := heardHz(t, p); math.Abs(got-523) > 10 {
		t.Errorf("with its bank missing, key 72 plays %.0f Hz, not General MIDI's piano's 523", got)
	}
	useTestFont(t, Font{Name: "test-late", Title: "Late", Group: "Synth"})
	if got := heardHz(t, p); math.Abs(got-882) > 10 {
		t.Errorf("once its bank came, key 72 plays %.0f Hz, not the bank's 882", got)
	}
	DropFont("test-late")
	if got := heardHz(t, p); math.Abs(got-523) > 10 {
		t.Errorf("once its bank went, key 72 plays %.0f Hz, not General MIDI's piano's 523", got)
	}
}

func TestASynthLetGoWithoutCloseIsNotKeptForItsBank(t *testing.T) {
	// A host that drops a synth without closing it, as a view going
	// away does, leaves nothing behind: the synth, its voices and its
	// host are the collector's.
	freed := make(chan struct{})
	func() {
		p, _ := newSynth(t)
		runtime.AddCleanup(p, func(struct{}) { close(freed) }, struct{}{})
	}()
	for range 50 {
		runtime.GC()
		select {
		case <-freed:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("a synth let go without Close was kept")
}

func TestARecordedSoundsPatchPageShowsItsLevelAndVoices(t *testing.T) {
	useTestFont(t, Font{Name: "test-page", Title: "Test Square", Group: "Synth"})
	p, h := newSynth(t)
	s, ok := Find(Recorded, "Synth", "Test Square")
	if !ok {
		t.Fatal("no recorded square")
	}
	if err := p.SetState(s.State()); err != nil {
		t.Fatal(err)
	}
	pn, ok := p.View(plug.Full, h).(*panel)
	if !ok {
		t.Fatal("the synth's full view is not its panel")
	}
	cards := pn.pages.editor.flow.cards
	if len(cards) != 1 || cards[0].Name != "RECORDED" {
		t.Fatalf("a recorded sound's patch page shows %d cards", len(cards))
	}
}

// A bank's note sounds from its NoteOn on, though its first frames are
// silent, longer than Latency says: a host that stops running a synth
// whose sound falls silent keeps it running through them. Once the note
// has rung out, nothing sounds.
func TestABanksNoteSoundsThroughItsSilentStart(t *testing.T) {
	useTestFont(t, Font{Name: "test-gm", Title: "Test GM"})
	kit, ok := Find("Test GM", GMDrums, "Kit")
	if !ok {
		t.Fatal("the test bank has no kit")
	}
	p, _ := newSynth(t)
	if err := p.SetState(kit.State()); err != nil {
		t.Fatal(err)
	}
	pr := p.proc
	// Offline, the note waits for its sample, read in as the sound is
	// made.
	pr.Prepare(plug.Setup{Rate: 48000, MaxBlock: 16, Channels: 2, Offline: true})
	l, r := make([]float32, 16), make([]float32, 16)
	run := func(evs ...plug.Event) bool {
		pr.Process(&plug.Block{Frames: 16, Out: [][]float32{l, r}, Events: evs})
		return !quiet(l) || !quiet(r)
	}
	run()
	if pr.Sounding() {
		t.Fatal("a synth given no note sounds")
	}
	if run(plug.Event{Frame: 15, Kind: plug.NoteOn, Note: 1, Pitch: 36, Velocity: 0.8}) {
		t.Fatal("the bank's note sounds in its first frame")
	}
	silent := 1
	for !run() {
		if !pr.Sounding() {
			t.Fatalf("the bank's note falls silent %d frames on, before its sound comes", silent)
		}
		if silent += 16; silent > 48000 {
			t.Fatal("the bank's note never sounds")
		}
	}
	if silent <= pr.Latency() {
		t.Fatalf("the bank's note is silent %d frames, within Latency, %d: the test shows nothing", silent, pr.Latency())
	}
	run(plug.Event{Kind: plug.NoteOff, Note: 1})
	for range 48000 / 16 {
		if !pr.Sounding() {
			return
		}
		run()
	}
	t.Fatal("the bank's note let go still sounds a second on")
}
