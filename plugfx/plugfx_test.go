package plugfx

import (
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// setup is how the tests run a processor, unless one says otherwise.
var setup = plug.Setup{Rate: synth.Rate, MaxBlock: 256, Channels: 2}

// rig drives a processor in blocks, as a host does.
type rig struct {
	pr plug.Processor
	d  *plug.Descriptor
	s  plug.Setup
	// tempo is the tempo each block carries, by the frame it starts at.
	tempo func(frame int) float64
	// before runs before the block starting at frame.
	before func(frame int)
}

// run prepares the processor and runs in through it, with each
// parameter's Default at frame 0 and then changes, and returns what
// came out.
func (r rig) run(t *testing.T, in [][]float32, changes ...plug.Change) [][]float32 {
	t.Helper()
	frames := len(in[0])
	r.pr.Prepare(r.s)
	var all []plug.Change
	for _, prm := range r.d.Params {
		all = append(all, plug.Change{ID: prm.ID, Value: prm.Default})
	}
	more := slices.Clone(changes)
	slices.SortStableFunc(more, func(a, b plug.Change) int { return a.Frame - b.Frame })
	all = append(all, more...)
	out := [][]float32{make([]float32, frames), make([]float32, frames)}
	for at := 0; at < frames; at += r.s.MaxBlock {
		n := min(r.s.MaxBlock, frames-at)
		if r.before != nil {
			r.before(at)
		}
		b := &plug.Block{Frames: n, In: [][]float32{in[0][at : at+n], in[1][at : at+n]},
			Out: [][]float32{out[0][at : at+n], out[1][at : at+n]}, Position: plug.Position{Frame: int64(at), Playing: true}}
		if r.tempo != nil {
			b.Position.Tempo = r.tempo(at)
		}
		for len(all) > 0 && all[0].Frame < at+n {
			c := all[0]
			c.Frame -= at
			b.Changes = append(b.Changes, c)
			all = all[1:]
		}
		r.pr.Process(b)
	}
	return out
}

// impulse is a click at frame at, in frames of silence.
func impulse(frames, at int) [][]float32 {
	in := [][]float32{make([]float32, frames), make([]float32, frames)}
	in[0][at], in[1][at] = 1, 1
	return in
}

// sine is a sine of hz at level, frames long.
func sine(frames int, hz, level float64) [][]float32 {
	in := [][]float32{make([]float32, frames), make([]float32, frames)}
	for i := range frames {
		v := float32(level * math.Sin(2*math.Pi*hz*float64(i)/synth.Rate))
		in[0][i], in[1][i] = v, v
	}
	return in
}

// peak returns the loudest frame of x.
func peak(x ...[]float32) float32 {
	var p float32
	for _, ch := range x {
		for _, v := range ch {
			p = max(p, float32(math.Abs(float64(v))))
		}
	}
	return p
}

// same fails t where a and b differ anywhere.
func same(t *testing.T, what string, a, b [][]float32) {
	t.Helper()
	if peak(b...) < 0.01 {
		t.Fatalf("%s: the song's engine makes silence", what)
	}
	for ch := range 2 {
		for i := range a[ch] {
			if a[ch][i] != b[ch][i] {
				t.Fatalf("%s: channel %d frame %d is %g, where the song's engine makes %g", what, ch, i, a[ch][i], b[ch][i])
			}
		}
	}
}

// engine runs in through the song engine's unit, a block at a time.
func engine(in [][]float32, block int, process func(l, r []float32)) [][]float32 {
	out := [][]float32{slices.Clone(in[0]), slices.Clone(in[1])}
	for at := 0; at < len(out[0]); at += block {
		end := min(at+block, len(out[0]))
		process(out[0][at:end], out[1][at:end])
	}
	return out
}

// noise is a burst of white noise, then silence.
func noise(frames, burst int) [][]float32 {
	in := [][]float32{make([]float32, frames), make([]float32, frames)}
	x := uint32(1)
	for i := range burst {
		for ch := range 2 {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			in[ch][i] = float32(x)/float32(math.MaxUint32)*0.6 - 0.3
		}
	}
	return in
}

func TestEachEffectSoundsAsTheSongsEngine(t *testing.T) {
	in := noise(48000, 4000)
	t.Run("reverb", func(t *testing.T) {
		rev := synth.NewReverbUnit(synth.Rate)
		rev.Set(1.2, 3.2, 1-0.45, 0.03)
		want := engine(in, setup.MaxBlock, rev.Process)
		p := ReverbDescriptor.New(nil)
		got := rig{pr: p.Processor(), d: &ReverbDescriptor, s: setup}.run(t, in,
			plug.Change{ID: SizeID, Value: 1.2}, plug.Change{ID: DecayID, Value: 3.2},
			plug.Change{ID: DampingID, Value: 45}, plug.Change{ID: PreDelayID, Value: 30})
		same(t, "the reverb", got, want)
	})
	t.Run("delay", func(t *testing.T) {
		// A song's delay at its defaults: 0.75 beats, feedback 0.35, tone
		// 0.4; the plugin's: a dotted eighth, 35%, and 4.4 kHz.
		dly := synth.NewDelayUnit(synth.Rate, 2, 0)
		dly.SetBeats(0.75, 128, 0.35, 0.4)
		want := engine(in, setup.MaxBlock, dly.Process)
		p := DelayDescriptor.New(nil)
		got := rig{pr: p.Processor(), d: &DelayDescriptor, s: setup, tempo: func(int) float64 { return 128 }}.run(t, in)
		same(t, "the delay", got, want)
	})
	colour := func(t *testing.T, kind string, changes []plug.Change, process func(l, r []float32)) {
		t.Helper()
		want := engine(in, setup.MaxBlock, process)
		p := ColourDescriptor.New(nil)
		if err := p.SetState(ColourState{Units: []Unit{{Kind: kind, Slot: 2}}}.Encode()); err != nil {
			t.Fatal(err)
		}
		got := rig{pr: p.Processor(), d: &ColourDescriptor, s: setup}.run(t, in, changes...)
		same(t, kind, got, want)
	}
	for mode, name := range synth.ChorusKinds {
		t.Run("chorus "+name, func(t *testing.T) {
			c := synth.NewChorusUnit(synth.Rate, name)
			c.Set(name, 0.8)
			colour(t, Chorus, []plug.Change{{ID: UnitParamID(2, ChorusModeParam), Value: float64(mode)},
				{ID: UnitParamID(2, ChorusMixParam), Value: 80}}, c.Process)
		})
	}
	t.Run("phaser", func(t *testing.T) {
		ph := synth.NewPhaserUnit(synth.Rate)
		ph.Set(1.5, 0.5, 0.6, 1)
		colour(t, Phaser, []plug.Change{{ID: UnitParamID(2, PhaserRateParam), Value: 1.5},
			{ID: UnitParamID(2, PhaserDepthParam), Value: 50}, {ID: UnitParamID(2, PhaserFeedbackParam), Value: 60}},
			ph.Process)
	})
	for kind, name := range synth.DistortKinds {
		t.Run("distortion "+name, func(t *testing.T) {
			d := synth.NewDistortionUnit(synth.Rate)
			d.Set(0.7, name)
			colour(t, Distortion, []plug.Change{{ID: UnitParamID(2, DistortTypeParam), Value: float64(kind)},
				{ID: UnitParamID(2, DistortDriveParam), Value: 70}}, d.Process)
		})
	}
}
