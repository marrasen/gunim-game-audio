package plugsynth

import (
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// everything is a patch with something for every parameter to turn: a
// pulse and an fm wave, a filter and its envelope, an LFO, noise and
// drive.
var everything = &synth.Patch{
	Osc:       []synth.Osc{{Wave: "pulse", Width: 0.3}, {Wave: "fm", Ratio: 2, Index: 2, Level: 0.6}},
	Filter:    synth.Filter{Type: "lp", Cutoff: 1800, Res: 0.3, Env: 1},
	FilterEnv: synth.Env{Attack: 0.01, Decay: 0.4, Sustain: 0.3, Release: 0.3},
	Amp:       synth.Env{Attack: 0.01, Decay: 0.3, Sustain: 0.6, Release: 0.3},
	LFO:       []synth.LFO{{To: "pitch", Wave: "sine", Hz: 4, Depth: 0.3}},
	Noise:     0.1,
	Drive:     0.2,
}

// playing returns a synth playing patch pt, as a state of its own.
func playing(t *testing.T, pt *synth.Patch) *plugin {
	t.Helper()
	p, _ := newSynth(t)
	if err := p.SetState(State{Name: "Everything", Patch: pt}.Encode()); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestEveryParameterTurnsTheSound plays a note with each parameter
// turned from its default, as a host's control drives it, and each
// sounds otherwise than with none turned.
func TestEveryParameterTurnsTheSound(t *testing.T) {
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	render := func(id uint32, v float64) []float32 {
		ps := params()
		for i := range ps {
			if ps[i].ID == id {
				ps[i].Default = v
			}
		}
		out := plugtest.Play(t, playing(t, everything).Processor(), setup, 24000, ps, note(1, 57, 0, 18000)...)
		return out[0]
	}
	plain := render(0, 0)
	for _, prm := range Descriptor.Params {
		if prm.ID == SwellID || prm.ID == GhostID {
			continue
		}
		v := prm.Plain(0.85)
		if prm.ID == ModID || prm.ID == PressureID || prm.ID == VowelID {
			v = prm.Max
		}
		got := render(prm.ID, v)
		diff := 0.0
		for i := range got {
			diff += math.Abs(float64(got[i] - plain[i]))
		}
		if diff/float64(len(got)) < 1e-4 {
			t.Errorf("%s at %s leaves the sound as it was", prm.Name, prm.Text(v))
		}
	}
}

// TestTheModWheelAndPressureReachTheSynth plays a lead with a note's mod
// wheel up, which swings its pitch, and with its key pressed, which
// turns it up.
func TestTheModWheelAndPressureReachTheSynth(t *testing.T) {
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	sine := &synth.Patch{Osc: []synth.Osc{{Wave: "sine"}}, Amp: synth.Env{Attack: 0.002, Sustain: 1, Release: 0.05}}
	play := func(e plug.ExprKind, v float64) []float32 {
		events := append(note(1, 69, 0, 47999), plug.Event{Frame: 100, Kind: plug.Expression, Note: 1, Expr: e, Value: v})
		return plugtest.Play(t, playing(t, sine).Processor(), setup, 48000, params(), events...)[0]
	}
	still, wheel := play(plug.Mod, 0), play(plug.Mod, 1)
	swing := func(x []float32) float64 {
		lo, hi := math.Inf(1), 0.0
		for at := 4800; at+1920 <= len(x); at += 1920 {
			f := hz(x[at:at+1920], 48000)
			lo, hi = min(lo, f), max(hi, f)
		}
		return hi - lo
	}
	if s0, s1 := swing(still), swing(wheel); s0 > 2 || s1 < 12 {
		t.Fatalf("the pitch swings by %.1f Hz with the wheel down and %.1f with it up", s0, s1)
	}
	if r := rms(play(plug.Pressure, 1)[4800:]) / rms(still[4800:]); math.Abs(r-math.Pow(10, 5.0/20)) > 0.03 {
		t.Fatalf("pressed all the way, the note sounds %.3f times as loud", r)
	}
}

// TestAPatchEditedPlaysOnUnbroken holds a note, and sets a state whose
// patch has its cutoff moved, as the patch editor does: the processor
// takes it in place, the note playing on, unbroken, on the same
// instrument.
func TestAPatchEditedPlaysOnUnbroken(t *testing.T) {
	p := playing(t, everything)
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2})
	l, r := make([]float32, 128), make([]float32, 128)
	block := func(events ...plug.Event) []float32 {
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 128, Events: events})
		return append([]float32(nil), l...)
	}
	block(plug.Event{Kind: plug.NoteOn, Note: 1, Pitch: 57, Velocity: 0.8})
	for range 100 {
		block()
	}
	cur := p.proc.cur
	edited := clonePatch(everything)
	edited.Filter.Cutoff = 4000
	if err := p.SetState(State{Name: "Everything", Patch: edited}.Encode()); err != nil {
		t.Fatal(err)
	}
	before := block()
	if p.proc.cur != cur || p.proc.prev != nil || cur.Held() != 1 {
		t.Fatal("the edited patch starts the note again on another instrument")
	}
	if rms(before) < 0.01 {
		t.Fatalf("the note is %.4f loud as the edit lands", rms(before))
	}
	more := clonePatch(edited)
	more.Osc = more.Osc[:1]
	if err := p.SetState(State{Name: "Everything", Patch: more}.Encode()); err != nil {
		t.Fatal(err)
	}
	block()
	if p.proc.cur == cur {
		t.Fatal("a patch with an oscillator less plays on the instrument before")
	}
}

// TestWhisperSingsItsVowelsInTurn finds the whisper of Wire Cathedral
// in the browser, singing the song's vowels in turn.
func TestWhisperSingsItsVowelsInTurn(t *testing.T) {
	s, ok := Find(Songs, "Wire Cathedral", "whisper")
	if !ok {
		t.Fatal("the browser lacks Wire Cathedral's whisper")
	}
	if got := s.Patch().Vowels; got != "a i o u e a o i" {
		t.Fatalf("whisper sings %q in turn", got)
	}
}
