// Package plugmod holds the engine's modulators as plugins in gunim's
// [plug] format: generators, which make control alone, for a host to
// wire to parameters. Its first is an LFO, a low-frequency oscillator.
// It registers itself as it starts; import it for that:
//
//	import _ "github.com/marrasen/gunim-game-audio/plugmod"
package plugmod

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim/audio/plug"
)

// The parameters' IDs. IDs 11 and 12 were the Mod input's target and
// amount, gone for an input of each target's own: they stay unused.
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
	// SeedID picks the random values: of the random shapes, and the
	// jitter's.
	SeedID
)

// The parameters' IDs after the gap.
const (
	// JitterID adds a random value to the wave, held for a 32nd of a
	// cycle, from 0 to 1 of its height.
	JitterID uint32 = iota + 13
	// SmoothID rounds the wave off: each value is the wave's average
	// over a window about it, as wide as half a cycle at full.
	SmoothID
	// GlideID is how much of each step of the Steps shape slides from
	// the step before to its own value, from 0 to all of it.
	GlideID
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
	// Saw ramps up, and SawDown down.
	Saw
	SawDown
	Square
	// ExpRise curves up from -1 to 1, slowly and then fast, and ExpFall
	// drops from 1 fast and then slowly, as a pluck's envelope.
	ExpRise
	ExpFall
	// SampleHold holds a random value a cycle, and SmoothRandom glides
	// from one to the next.
	SampleHold
	SmoothRandom
	// Steps plays the steps of the LFO's [State], one after another,
	// over a cycle.
	Steps
	// Shaper plays the curve the LFO's [State] draws through its
	// points.
	Shaper
)

// Shapes are the shapes' names, by ShapeID's value.
var Shapes = []string{"Sine", "Triangle", "Saw up", "Saw down", "Square", "Exp rise", "Exp fall",
	"Sample & hold", "Smooth random", "Steps", "Shaper"}

// The control inputs' indexes, one for each thing they move. Each runs
// from -1 to 1, as the host scales what it is wired to, and stands at 0
// unconnected; m is its value.
const (
	// RateIn runs the LFO 2^(3m) times as fast: three octaves either
	// way at full. Each frame's step is scaled, so the phase moves on
	// smoothly as the rate changes.
	RateIn = iota
	// DepthIn adds m to the depth, within 0 to 1.
	DepthIn
	// PhaseIn turns the wave on by m half cycles.
	PhaseIn
	// OffsetIn adds m to the offset, within -1 to 1.
	OffsetIn
)

// Where the phase counts from, by StartID's value.
const (
	// FromSong counts it from the song's start, by the beats a synced
	// LFO's tempo map counts, or the seconds a free one's: so the LFO is
	// where the song is, in a render as in playback. A new rate or note
	// value takes the phase where it falls at that place in the song.
	//
	// The Rate mod input can't keep that: the LFO runs
	// ahead of the song, or behind it, by what the input did. It keeps
	// that lead on top of the song's phase, so the wave moves smoothly
	// as the input stops or the rate changes. The lead goes back to 0
	// at each Prepare and Reset, and as Position.Playing turns on: so a
	// play or a render is in step with the song as it starts.
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

// percent writes a value from 0 to 1 as a percentage.
func percent(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

// Descriptor is the LFO, as it is registered.
var Descriptor = plug.Descriptor{
	ID:      "marrasen.lfo",
	Name:    "LFO",
	Vendor:  "Marras",
	Version: "0.2.0",
	Kind:    "Generator|LFO",
	ControlIn: []plug.ControlPort{
		{Name: "Rate mod", Min: -1, Max: 1},
		{Name: "Depth mod", Min: -1, Max: 1},
		{Name: "Phase mod", Min: -1, Max: 1},
		{Name: "Offset mod", Min: -1, Max: 1},
	},
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
		{ID: DepthID, Name: "Depth", Max: 1, Default: 1, Flags: plug.Automatable, Format: percent},
		{ID: OffsetID, Name: "Offset", Min: -1, Max: 1, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.2f", v) }},
		{ID: UnipolarID, Name: "Polarity", Max: 1, Steps: 1, Flags: plug.Automatable, Format: onOff("±", "+")},
		{ID: SeedID, Name: "Seed", Max: 999, Steps: 999, Format: func(v float64) string { return fmt.Sprintf("%.0f", v) }},
		{ID: JitterID, Name: "Jitter", Max: 1, Flags: plug.Automatable, Format: percent},
		{ID: SmoothID, Name: "Smooth", Max: 1, Flags: plug.Automatable, Format: percent},
		{ID: GlideID, Name: "Glide", Max: 1, Flags: plug.Automatable, Format: percent},
		{ID: ValueID, Name: "Value", Min: -1, Max: 1, Flags: plug.ReadOnly | plug.Hidden},
		{ID: CycleID, Name: "Cycle", Max: 1, Flags: plug.ReadOnly | plug.Hidden},
	},
	New: func(plug.Host) plug.Plugin { return newPlugin() },
}

func init() { plug.Register(Descriptor) }

// plugin is one LFO.
type plugin struct {
	// mu guards st, which State and SetState reach from the app's
	// goroutine and the views from the window's.
	mu   sync.Mutex
	st   State
	proc processor
}

func newPlugin() *plugin {
	p := &plugin{st: DefaultState()}
	p.proc.pat.Store(compile(p.st))
	return p
}

// Processor implements [plug.Plugin].
func (p *plugin) Processor() plug.Processor { return &p.proc }

// State implements [plug.Plugin]: the steps and the curve, as [State]
// writes them, or nil while they are a new LFO's.
func (p *plugin) State() []byte {
	st := p.current()
	if st.Equal(DefaultState()) {
		return nil
	}
	return st.Encode()
}

// SetState implements [plug.Plugin]: it plays the steps and the curve
// the state holds, and a new LFO's for an empty one. A state it cannot
// read changes nothing.
func (p *plugin) SetState(b []byte) error {
	st, err := DecodeState(b)
	if err != nil {
		return err
	}
	p.set(st)
	return nil
}

// current returns a copy of the state.
func (p *plugin) current() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st.Clone()
}

// set makes st the state, for the sound too, which takes it at its next
// block.
func (p *plugin) set(st State) {
	st = st.Clone()
	p.mu.Lock()
	p.st = st
	p.mu.Unlock()
	p.proc.pat.Store(compile(st))
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
	jitter        float64
	smooth, glide float64
	// pat is the state's steps and curve, as the shapes play them.
	pat *pattern
}

// defaults returns the settings at each parameter's default, with the
// steps and the curve of a new LFO.
func defaults() settings {
	s := settings{pat: defaultPattern}
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
	case JitterID:
		s.jitter = v
	case SmoothID:
		s.smooth = v
	case GlideID:
		s.glide = v
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
// was played up to there: but for the lead the Rate mod input gives it,
// as [FromSong] tells. Every shape, the jitter and the smoothing are a
// function of the phase alone, with no memory of the frames before, so
// they keep to that too.
type processor struct {
	s    settings
	pat  atomic.Pointer[pattern]
	rate float64
	// cycles is the phase counted from the play or run on, at the
	// block's next frame.
	cycles float64
	// lead is how far the Rate mod input has moved the phase counted
	// from the song on from where the song is, in cycles.
	lead float64
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
	if pt := p.pat.Load(); pt != nil {
		p.s.pat = pt
	}
	p.cycles, p.lead, p.playing = 0, 0, false
}

// Reset implements [plug.Processor]: an LFO counted from the play
// starts its cycle again, and one counted from the song goes back to
// where the song is.
func (p *processor) Reset() {
	if p.s.start == FromPlay {
		p.cycles = 0
	}
	p.lead = 0
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
// frame on, and a new state from the block's start. The inputs,
// unconnected, stand at 0.
func (p *processor) Process(b *plug.Block) {
	if pt := p.pat.Load(); pt != nil {
		p.s.pat = pt
	}
	pos := b.Position
	if pos.Playing && !p.playing {
		if p.s.start == FromPlay {
			p.cycles = 0
		}
		p.lead = 0
	}
	p.playing = pos.Playing
	var outs [4][]float32
	for i := range outs {
		outs[i] = b.ControlOutput(i)
	}
	var ins [4][]float32
	for i := range ins {
		ins[i] = b.ControlInput(i)
	}
	ci := 0
	for f := range b.Frames {
		for ci < len(b.Changes) && b.Changes[ci].Frame <= f {
			p.s.take(b.Changes[ci].ID, b.Changes[ci].Value)
			ci++
		}
		var m [4]float64
		for i, in := range ins {
			if in != nil {
				m[i] = min(max(float64(in[f]), -1), 1)
			}
		}
		s, speed := p.s.modded(m)
		c := p.at(pos, f, b.Frames)
		for i, out := range outs {
			if out != nil {
				out[f] = float32(s.wave(c + float64(i)/4))
			}
		}
		if f == b.Frames-1 {
			p.value.Set(s.wave(c))
			p.cycle.Set(c + s.phase - math.Floor(c+s.phase))
		}
		step := p.perFrame(pos, f, b.Frames)
		p.cycles += step * speed
		if p.s.start == FromSong {
			p.lead += step * (speed - 1)
		}
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
		return float64(pos.Frame+int64(f))/p.rate*p.s.rate + p.lead
	}
	return p.beat(pos, f, n)/p.s.beats + p.lead
}

// modded returns the settings as the inputs' values m, by their
// indexes, move them, and how many times as fast the LFO runs.
func (s *settings) modded(m [4]float64) (t settings, speed float64) {
	t, speed = *s, 1
	if m[RateIn] != 0 {
		speed = math.Exp2(3 * m[RateIn])
	}
	if m[DepthIn] != 0 {
		t.depth = min(max(t.depth+m[DepthIn], 0), 1)
	}
	t.phase += m[PhaseIn] / 2
	if m[OffsetIn] != 0 {
		t.offset = min(max(t.offset+m[OffsetIn], -1), 1)
	}
	return t, speed
}

// The jitter's random values: how many a cycle, and what it adds to the
// seed, so they are not the random shapes' own.
const (
	jitterSteps = 32
	jitterSeed  = 1 << 40
)

// maxSmooth is how wide the smoothing's window is at full, in cycles.
const maxSmooth = 0.5

// wave returns the output at c cycles, turned by the Phase setting:
// the shape and its jitter, smoothed, then scaled by the depth and moved
// by the offset, within -1 to 1.
func (s *settings) wave(c float64) float64 {
	c += s.phase
	var v float64
	if w := s.smooth * maxSmooth; w > 1e-6 {
		v = s.area(c-w/2, c+w/2) / w
	} else {
		v = s.shapeAt(c) + s.jitterAt(c)
	}
	if s.unipolar {
		v = (v + 1) / 2
	}
	return min(max(s.offset+s.depth*v, -1), 1)
}

// jitterAt returns the jitter at c cycles.
func (s *settings) jitterAt(c float64) float64 {
	if s.jitter == 0 {
		return 0
	}
	return s.jitter * random(int64(math.Floor(c*jitterSteps)), s.seed+jitterSeed)
}

// area returns the area under the shape and its jitter from a cycles to
// b, b no more than a cycle after a.
func (s *settings) area(a, b float64) float64 {
	sum := 0.0
	for n := math.Floor(a); n < b; n++ {
		x0, x1 := max(a, n)-n, min(b, n+1)-n
		sum += s.integral(int64(n), x1) - s.integral(int64(n), x0)
	}
	if s.jitter != 0 {
		for k := math.Floor(a * jitterSteps); k < b*jitterSteps; k++ {
			x0, x1 := max(a, k/jitterSteps), min(b, (k+1)/jitterSteps)
			sum += s.jitter * random(int64(k), s.seed+jitterSeed) * (x1 - x0)
		}
	}
	return sum
}

// expCurve is how far the exponential shapes bend.
const expCurve = 4

// shapeAt returns the shape alone at c cycles, from -1 to 1.
func (s *settings) shapeAt(c float64) float64 {
	n := math.Floor(c)
	x := c - n
	switch s.shape {
	case Sine:
		return math.Sin(2 * math.Pi * x)
	case Triangle:
		switch {
		case x < 0.25:
			return 4 * x
		case x < 0.75:
			return 2 - 4*x
		default:
			return 4*x - 4
		}
	case Saw:
		return 2*x - 1
	case SawDown:
		return 1 - 2*x
	case Square:
		if x >= 0.5 {
			return -1
		}
		return 1
	case ExpRise:
		return 2*math.Expm1(expCurve*x)/math.Expm1(expCurve) - 1
	case ExpFall:
		e := math.Exp(-expCurve)
		return 2*(math.Exp(-expCurve*x)-e)/(1-e) - 1
	case SampleHold:
		return random(int64(n), s.seed)
	case SmoothRandom:
		a, b := random(int64(n), s.seed), random(int64(n)+1, s.seed)
		return a + (b-a)*x*x*(3-2*x)
	case Steps:
		return s.pat.step(x, s.glide)
	case Shaper:
		return s.pat.curveAt(x)
	}
	return 0
}

// integral returns the area under cycle n of the shape, from its start
// to x, from 0 to 1, through it.
func (s *settings) integral(n int64, x float64) float64 {
	switch s.shape {
	case Sine:
		return (1 - math.Cos(2*math.Pi*x)) / (2 * math.Pi)
	case Triangle:
		switch {
		case x < 0.25:
			return 2 * x * x
		case x < 0.75:
			return 2*x - 2*x*x - 0.25
		default:
			return 2*x*x - 4*x + 2
		}
	case Saw:
		return x*x - x
	case SawDown:
		return x - x*x
	case Square:
		if x >= 0.5 {
			return 1 - x
		}
		return x
	case ExpRise:
		return 2*(math.Expm1(expCurve*x)/expCurve-x)/math.Expm1(expCurve) - x
	case ExpFall:
		e := math.Exp(-expCurve)
		return 2*(-math.Expm1(-expCurve*x)/expCurve-x*e)/(1-e) - x
	case SampleHold:
		return random(n, s.seed) * x
	case SmoothRandom:
		a, b := random(n, s.seed), random(n+1, s.seed)
		return a*x + (b-a)*(x*x*x-x*x*x*x/2)
	case Steps:
		return s.pat.stepsIntegral(x, s.glide)
	case Shaper:
		return s.pat.curveIntegral(x)
	}
	return 0
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
