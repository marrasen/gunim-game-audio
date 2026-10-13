package plugfx

import (
	"errors"
	"math"
	"sync/atomic"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// The reverb's parameters' IDs.
const (
	// SizeID is the room's size, from 0.3 to 1.5: how far apart its
	// walls are.
	SizeID uint32 = iota + 1
	// DecayID is how long the reverb takes to fall 60 dB, in seconds.
	DecayID
	// DampingID is how fast the highs die, in percent: 0 bright, 100
	// dark.
	DampingID
	// PreDelayID is how long the sound takes to reach the room, in
	// milliseconds.
	PreDelayID
	// WidthID is how wide the reverb's sound is, in percent: 0 mono.
	WidthID
	// MixID is how much of the reverb is heard, in percent: 100, all
	// reverb, for a send, as it starts.
	MixID
)

// ReverbDescriptor is the reverb, as it is registered.
var ReverbDescriptor = plug.Descriptor{
	ID:      "marrasen.reverb",
	Name:    "Reverb",
	Vendor:  "Marras",
	Version: "0.1.0",
	Kind:    "Fx|Reverb",
	Params: []plug.Param{
		{ID: SizeID, Name: "Size", Min: 0.3, Max: 1.5, Default: 0.8, Flags: plug.Automatable,
			Format: func(v float64) string { return percent(v * 100) }},
		{ID: DecayID, Name: "Decay", Unit: "s", Min: 0.1, Max: 20, Default: 2.2, Log: true, Flags: plug.Automatable,
			Format: seconds},
		{ID: DampingID, Name: "Damping", Unit: "%", Max: 100, Default: 50, Flags: plug.Automatable, Format: percent},
		{ID: PreDelayID, Name: "Pre-delay", Unit: "ms", Max: 240, Default: 20, Flags: plug.Automatable, Format: millis},
		{ID: WidthID, Name: "Width", Unit: "%", Max: 100, Default: 100, Flags: plug.Automatable, Format: percent},
		{ID: MixID, Name: "Mix", Unit: "%", Max: 100, Default: 100, Flags: plug.Automatable, Format: percent},
	},
	Patches: []plug.Patch{
		{Name: "Default", Category: "Send"},
		{Name: "Small room", Category: "Send", Params: map[uint32]float64{SizeID: 0.4, DecayID: 0.6, DampingID: 60, PreDelayID: 4}},
		{Name: "Plate", Category: "Send", Params: map[uint32]float64{SizeID: 0.7, DecayID: 1.8, DampingID: 25, PreDelayID: 10}},
		{Name: "Hall", Category: "Send", Params: map[uint32]float64{SizeID: 1.2, DecayID: 3.2, DampingID: 45, PreDelayID: 30}},
		{Name: "Cathedral", Category: "Send", Params: map[uint32]float64{SizeID: 1.5, DecayID: 9, DampingID: 65, PreDelayID: 60}},
		{Name: "Vocal in a hall", Category: "Insert",
			Params: map[uint32]float64{SizeID: 1.1, DecayID: 2.6, DampingID: 50, PreDelayID: 40, MixID: 22}},
	},
	New: func(h plug.Host) plug.Plugin { return &reverbPlugin{} },
}

// reverbPlugin is one reverb.
type reverbPlugin struct{ proc reverbProc }

// Processor implements [plug.Plugin].
func (p *reverbPlugin) Processor() plug.Processor { return &p.proc }

// State implements [plug.Plugin]: its parameters hold all it keeps.
func (p *reverbPlugin) State() []byte { return nil }

// SetState implements [plug.Plugin]: it takes only an empty state.
func (p *reverbPlugin) SetState(b []byte) error {
	if len(b) > 0 {
		return errors.New("plugfx: the reverb keeps no state")
	}
	return nil
}

// Close implements [plug.Plugin].
func (p *reverbPlugin) Close() {}

// View implements [plug.Viewer].
func (p *reverbPlugin) View(s plug.Size, h plug.Host) gunim.Node { return reverbView(s, h) }

// reverbProc runs the song engine's reverb on the sound fed to it, and
// blends it with the sound, as wide as it is set.
type reverbProc struct {
	rate   float64
	rev    *synth.ReverbUnit
	wl, wr []float32
	blend  blend
	// The reverb's shape as set: size, decay in seconds, tone from 0,
	// dark, to 1, and pre-delay in seconds.
	size, decay, tone, pre float64
	tail                   atomic.Int64
}

// Prepare implements [plug.Processor]: everything the reverb keeps is
// made here, so processing allocates zero bytes.
func (p *reverbProc) Prepare(s plug.Setup) {
	p.rate = s.Rate
	if p.rate <= 0 {
		p.rate = synth.Rate
	}
	p.rev = synth.NewReverbUnit(p.rate)
	n := max(s.MaxBlock, 1)
	p.wl, p.wr = make([]float32, n), make([]float32, n)
	p.blend.prepare(p.rate)
	for _, prm := range ReverbDescriptor.Params {
		p.take(plug.Change{ID: prm.ID, Value: prm.Default})
	}
	p.Reset()
}

// take takes a change to a parameter.
func (p *reverbProc) take(c plug.Change) {
	prm, ok := ReverbDescriptor.Param(c.ID)
	if !ok {
		return
	}
	v := prm.Clamp(c.Value)
	switch c.ID {
	case SizeID:
		p.size = v
	case DecayID:
		p.decay = v
	case DampingID:
		p.tone = 1 - v/100
	case PreDelayID:
		p.pre = v / 1000
	case WidthID:
		p.blend.widthTo = float32(v / 100)
		return
	case MixID:
		p.blend.mixTo = float32(v / 100)
		return
	}
	if p.rev != nil {
		p.rev.Set(p.size, p.decay, p.tone, p.pre)
	}
	p.tail.Store(int64(math.Ceil(reverbTail(p.decay, p.pre) * p.rate)))
}

// reverbTail is how long a reverb of decay rings on after its sound
// stops, past its pre-delay, in seconds: half as long again as it takes
// to fall 60 dB, which falls 90 dB, and the network's longest line.
func reverbTail(decay, pre float64) float64 { return pre + 1.5*decay + 0.12 }

// Process implements [plug.Processor].
func (p *reverbProc) Process(b *plug.Block) {
	if p.rev == nil || b.Frames > len(p.wl) {
		return
	}
	runSegments(b, p)
}

// segment runs frames from to to.
func (p *reverbProc) segment(b *plug.Block, from, to int) {
	feed(b.In, p.wl, p.wr, from, to)
	p.rev.Process(p.wl[from:to], p.wr[from:to])
	p.blend.run(b.In, b.Out, p.wl, p.wr, from, to)
}

// Latency implements [plug.Processor]: the reverb sounds on time, 0.
func (p *reverbProc) Latency() int { return 0 }

// Tail implements [plug.Processor]: the pre-delay, and half as long
// again as the decay, by when the reverb has fallen 90 dB.
func (p *reverbProc) Tail() int { return int(p.tail.Load()) }

// Reset implements [plug.Processor]: the reverb's tail, silenced.
func (p *reverbProc) Reset() {
	if p.rev != nil {
		p.rev.Clear()
	}
	p.blend.jump()
}
