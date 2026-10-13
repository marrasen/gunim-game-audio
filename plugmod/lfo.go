// Package plugmod holds the engine's modulators as plugins in gunim's
// [plug] format: generators, which make control alone, for a host to
// wire to parameters. Its first is an LFO, a low-frequency oscillator.
// It registers itself as it starts; import it for that:
//
//	import _ "github.com/marrasen/gunim-game-audio/plugmod"
package plugmod

import (
	"errors"
	"fmt"
	"math"

	"github.com/marrasen/gunim/audio/plug"
)

// The parameters' IDs.
const (
	// ShapeID picks the wave, as [Shapes] names them.
	ShapeID uint32 = iota + 1
	// SyncID, on, runs the LFO at NoteID's note value of the tempo, and
	// off, at RateID's rate in hertz.
	SyncID
	RateID
	NoteID
	// StartID is where the phase counts from, as [Starts] names them.
	StartID
	// PhaseID turns the wave on, in degrees.
	PhaseID
	// DepthID scales the wave, and OffsetID moves it, before the output
	// clamps it to -1 to 1.
	DepthID
	OffsetID
	// UnipolarID, on, runs the wave from 0 to 1, and off, from -1 to 1.
	UnipolarID
	// SeedID picks the random shapes' values.
	SeedID
)

// The values the LFO reports as it plays, read-only parameters, for its
// views to draw: the main output's value, and where it is in its cycle,
// from 0 to 1.
const (
	ValueID uint32 = iota + 20
	CycleID
)

// The shapes, by ShapeID's value.
const (
	Sine = iota
	Triangle
	Saw
	Square
	SampleHold
	SmoothRandom
)

// Shapes are the shapes' names, by ShapeID's value.
var Shapes = []string{"Sine", "Triangle", "Saw", "Square", "Sample & hold", "Smooth random"}

// Where the phase counts from, by StartID's value.
const (
	// FromSong counts it from the song's start, by the beats a synced
	// LFO's tempo map counts, or the seconds a free one's: so the LFO is
	// where the song is, in a render as in playback. A new rate or note
	// value takes the phase where it falls at that place in the song.
	FromSong = iota
	// FromPlay counts it from where the playing starts, or jumps to: as
	// Position.Playing turns on, and at each Prepare and Reset. A render
	// plays with Playing clear, so the host's Prepare or Reset as the
	// render starts is what starts it there.
	FromPlay
	// FreeRunning counts it on from the first block, through stops and
	// jumps.
	FreeRunning
)

// Starts are the starts' names, by StartID's value.
var Starts = []string{"Song", "Play", "Free"}

// Notes are the note values a synced LFO's cycle lasts, by NoteID's
// value, and Beats how many beats each lasts: a bar is four beats, and a
// beat a quarter note.
var (
	Notes = []string{"8 bars", "4 bars", "2 bars", "1 bar", "1/2.", "1/2", "1/2T",
		"1/4.", "1/4", "1/4T", "1/8.", "1/8", "1/8T", "1/16.", "1/16", "1/16T", "1/32"}
	Beats = []float64{32, 16, 8, 4, 3, 2, 4.0 / 3,
		1.5, 1, 2.0 / 3, 0.75, 0.5, 1.0 / 3, 0.375, 0.25, 1.0 / 6, 0.125}
)

// The control outputs' indexes: the main output, and the wave a
// quarter, a half and three quarters of a cycle on from it.
const (
	MainOut = iota
	Out90
	Out180
	Out270
)

// named returns a format that writes a stepped value by its name.
func named(names []string) func(v float64) string {
	return func(v float64) string {
		i := min(max(int(math.Round(v)), 0), len(names)-1)
		return names[i]
	}
}

// onOff writes a switch, by the names of its two settings.
func onOff(off, on string) func(v float64) string {
	return func(v float64) string {
		if v >= 0.5 {
			return on
		}
		return off
	}
}

// Descriptor is the LFO, as it is registered.
var Descriptor = plug.Descriptor{
	ID:      "marrasen.lfo",
	Name:    "LFO",
	Vendor:  "Marras",
	Version: "0.1.0",
	Kind:    "Generator|LFO",
	ControlOut: []plug.ControlPort{
		{Name: "LFO", Min: -1, Max: 1},
		{Name: "LFO 90°", Min: -1, Max: 1},
		{Name: "LFO 180°", Min: -1, Max: 1},
		{Name: "LFO 270°", Min: -1, Max: 1},
	},
	Params: []plug.Param{
		{ID: ShapeID, Name: "Shape", Max: float64(len(Shapes) - 1), Steps: len(Shapes) - 1, Flags: plug.Automatable,
			Format: named(Shapes)},
		{ID: SyncID, Name: "Sync", Max: 1, Default: 1, Steps: 1, Flags: plug.Automatable, Format: onOff("Hz", "Tempo")},
		{ID: RateID, Name: "Rate", Unit: "Hz", Min: 0.01, Max: 40, Default: 1, Log: true, Flags: plug.Automatable,
			Format: func(v float64) string {
				if v < 1 {
					return fmt.Sprintf("%.3f Hz", v)
				}
				return fmt.Sprintf("%.2f Hz", v)
			}},
		{ID: NoteID, Name: "Note", Max: float64(len(Notes) - 1), Default: 3, Steps: len(Notes) - 1,
			Flags: plug.Automatable, Format: named(Notes)},
		{ID: StartID, Name: "Start", Max: float64(len(Starts) - 1), Steps: len(Starts) - 1, Format: named(Starts)},
		{ID: PhaseID, Name: "Phase", Unit: "°", Max: 360, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%.0f°", v) }},
		{ID: DepthID, Name: "Depth", Max: 1, Default: 1, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }},
		{ID: OffsetID, Name: "Offset", Min: -1, Max: 1, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.2f", v) }},
		{ID: UnipolarID, Name: "Polarity", Max: 1, Steps: 1, Flags: plug.Automatable, Format: onOff("±", "+")},
		{ID: SeedID, Name: "Seed", Max: 999, Steps: 999, Format: func(v float64) string { return fmt.Sprintf("%.0f", v) }},
		{ID: ValueID, Name: "Value", Min: -1, Max: 1, Flags: plug.ReadOnly | plug.Hidden},
		{ID: CycleID, Name: "Cycle", Max: 1, Flags: plug.ReadOnly | plug.Hidden},
	},
	New: func(plug.Host) plug.Plugin { return &plugin{} },
}

func init() { plug.Register(Descriptor) }

// plugin is one LFO.
type plugin struct{ proc processor }

// Processor implements [plug.Plugin].
func (p *plugin) Processor() plug.Processor { return &p.proc }

// State implements [plug.Plugin]: an LFO keeps nothing beyond its
// parameters.
func (p *plugin) State() []byte { return nil }

// SetState implements [plug.Plugin]: it takes only an empty state.
func (p *plugin) SetState(b []byte) error {
	if len(b) > 0 {
		return errors.New("plugmod: an LFO keeps no state")
	}
	return nil
}

// Close implements [plug.Plugin].
func (p *plugin) Close() {}

// settings are the LFO's parameters, as the processor takes them.
type settings struct {
	shape, start  int
	sync          bool
	rate, beats   float64
	phase         float64
	depth, offset float64
	unipolar      bool
	seed          uint64
}

// defaults returns the settings at each parameter's default.
func defaults() settings {
	var s settings
	for _, p := range Descriptor.Params {
		s.take(p.ID, p.Default)
	}
	return s
}

// take sets the parameter id to v.
func (s *settings) take(id uint32, v float64) {
	prm, ok := Descriptor.Param(id)
	if !ok || prm.Flags&plug.ReadOnly != 0 {
		return
	}
	v = prm.Clamp(v)
	switch id {
	case ShapeID:
		s.shape = int(math.Round(v))
	case SyncID:
		s.sync = v >= 0.5
	case RateID:
		s.rate = v
	case NoteID:
		s.beats = Beats[int(math.Round(v))]
	case StartID:
		s.start = int(math.Round(v))
	case PhaseID:
		s.phase = v / 360
	case DepthID:
		s.depth = v
	case OffsetID:
		s.offset = v
	case UnipolarID:
		s.unipolar = v >= 0.5
	case SeedID:
		s.seed = uint64(math.Round(v))
	}
}

// fallbackTempo is the tempo a synced LFO runs at where the host knows
// none, in quarter notes a minute.
const fallbackTempo = 120

// processor makes the LFO's outputs, a value a frame.
//
// Its phase is a count of cycles: the whole part counts the cycles, as
// the random shapes pick a value for each, and the rest is where in the
// cycle it is. Counted from the song, it is worked out afresh for each
// frame from the block's Position, so it is the same however the song
// was played up to there.
type processor struct {
	s    settings
	rate float64
	// cycles is the phase counted from the play or run on, at the
	// block's next frame.
	cycles float64
	// playing says the last block was played, to tell the play start.
	playing bool
	value   plug.Meter
	cycle   plug.Meter
}

// Prepare implements [plug.Processor].
func (p *processor) Prepare(s plug.Setup) {
	p.rate = s.Rate
	if p.rate <= 0 {
		p.rate = 48000
	}
	p.s = defaults()
	p.cycles, p.playing = 0, false
}

// Reset implements [plug.Processor]: an LFO counted from the play
// starts its cycle again.
func (p *processor) Reset() {
	if p.s.start == FromPlay {
		p.cycles = 0
	}
}

// Latency implements [plug.Processor]: the LFO is never late.
func (p *processor) Latency() int { return 0 }

// Tail implements [plug.Processor]: the LFO makes no sound.
func (p *processor) Tail() int { return 0 }

// Report implements [plug.Reporter]: the main output's value, and where
// it is in its cycle, as the last block ended.
func (p *processor) Report(id uint32) float64 {
	switch id {
	case ValueID:
		return p.value.Get()
	case CycleID:
		return p.cycle.Get()
	}
	return 0
}

// Process implements [plug.Processor]: each change takes effect from its
// frame on.
func (p *processor) Process(b *plug.Block) {
	pos := b.Position
	if pos.Playing && !p.playing && p.s.start == FromPlay {
		p.cycles = 0
	}
	p.playing = pos.Playing
	var outs [4][]float32
	for i := range outs {
		outs[i] = b.ControlOutput(i)
	}
	ci := 0
	for f := range b.Frames {
		for ci < len(b.Changes) && b.Changes[ci].Frame <= f {
			p.s.take(b.Changes[ci].ID, b.Changes[ci].Value)
			ci++
		}
		c := p.at(pos, f, b.Frames)
		for i, out := range outs {
			if out != nil {
				out[f] = float32(p.s.wave(c + float64(i)/4))
			}
		}
		if f == b.Frames-1 {
			p.value.Set(p.s.wave(c))
			p.cycle.Set(c + p.s.phase - math.Floor(c+p.s.phase))
		}
		p.cycles += p.perFrame(pos, f, b.Frames)
	}
	for ; ci < len(b.Changes); ci++ {
		p.s.take(b.Changes[ci].ID, b.Changes[ci].Value)
	}
}

// beat returns where frame f of a block n frames long at pos falls, in
// quarter notes: as the host's tempo map counts them, through any ramp,
// or, where the host knows no tempo, at fallbackTempo from the start.
func (p *processor) beat(pos plug.Position, f, n int) float64 {
	if pos.Tempo > 0 {
		return pos.BeatAt(f, n, p.rate)
	}
	return float64(pos.Frame+int64(f)) / p.rate * fallbackTempo / 60
}

// perFrame returns how many cycles pass from frame f of a block n
// frames long at pos to the next.
func (p *processor) perFrame(pos plug.Position, f, n int) float64 {
	if p.s.sync {
		return (p.beat(pos, f+1, n) - p.beat(pos, f, n)) / p.s.beats
	}
	return p.s.rate / p.rate
}

// at returns the phase at frame f of a block n frames long at pos, in
// cycles, before the Phase setting turns it.
func (p *processor) at(pos plug.Position, f, n int) float64 {
	if p.s.start != FromSong {
		return p.cycles
	}
	if !p.s.sync {
		return float64(pos.Frame+int64(f)) / p.rate * p.s.rate
	}
	return p.beat(pos, f, n) / p.s.beats
}

// wave returns the output at c cycles, turned by the Phase setting:
// the shape, scaled by the depth and moved by the offset, within -1 to 1.
func (s *settings) wave(c float64) float64 {
	c += s.phase
	n := math.Floor(c)
	x := c - n
	var v float64
	switch s.shape {
	case Sine:
		v = math.Sin(2 * math.Pi * x)
	case Triangle:
		switch {
		case x < 0.25:
			v = 4 * x
		case x < 0.75:
			v = 2 - 4*x
		default:
			v = 4*x - 4
		}
	case Saw:
		v = 2*x - 1
	case Square:
		v = 1
		if x >= 0.5 {
			v = -1
		}
	case SampleHold:
		v = random(int64(n), s.seed)
	case SmoothRandom:
		a, b := random(int64(n), s.seed), random(int64(n)+1, s.seed)
		v = a + (b-a)*x*x*(3-2*x)
	}
	if s.unipolar {
		v = (v + 1) / 2
	}
	return min(max(s.offset+s.depth*v, -1), 1)
}

// random returns the random shapes' value for cycle n, from -1 to 1:
// the same for the same cycle and seed, every time.
func random(n int64, seed uint64) float64 {
	// SplitMix64's mix of the cycle and the seed.
	z := uint64(n)*0x9e3779b97f4a7c15 + seed*0xbf58476d1ce4e5b9 + 0x94d049bb133111eb
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	z ^= z >> 31
	return float64(z>>11)/(1<<52) - 1
}
