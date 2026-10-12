// Package plugsynth is the engine's synth as a plugin in gunim's
// [plug] format: an instrument that plays a few of the patch library's
// sounds, a pad, a pluck, a bass, a lead, an electric piano and a drum
// kit, live, note by note, with knobs for the filter, the envelope and
// the level. It registers itself as it starts; import it for that:
//
//	import _ "github.com/marrasen/gunim-game-audio/plugsynth"
//
// The engine runs at [synth.Rate], 48 kHz. A host at another rate gets
// the sound resampled to its own, through a cubic interpolation, which
// costs a few frames of latency the processor reports.
package plugsynth

import (
	"errors"
	"fmt"
	"math"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
)

// The parameters' IDs.
const (
	// SoundID picks the sound, by its place in Sounds.
	SoundID uint32 = iota + 1
	// CutoffID moves the filter's cutoff, in octaves, and ResID adds to
	// its resonance.
	CutoffID
	ResID
	// AttackID and ReleaseID stretch the envelope's attack and release,
	// as factors.
	AttackID
	ReleaseID
	// GainID turns the sound up, in decibels.
	GainID
)

// A Sound is one of the sounds the plugin plays: a preset of the
// patch library, by its name, and the short name its selector shows.
type Sound struct {
	Short, Preset string
}

// Sounds are the sounds the plugin plays, in the order its Sound
// parameter steps through them.
var Sounds = []Sound{
	{"Pad", "Juno pad"},
	{"Pluck", "Harp pluck"},
	{"Bass", "Moog bass"},
	{"Lead", "Jupiter brass lead"},
	{"Keys", "DX7 E-piano"},
	{"Drums", "TR-808"},
}

// Descriptor is the plugin, as it is registered.
var Descriptor = plug.Descriptor{
	ID:      "marrasen.gamesynth",
	Name:    "Game Synth",
	Vendor:  "Marras",
	Version: "0.1.0",
	Kind:    "Instrument|Synth",
	Notes:   true,
	Params: []plug.Param{
		{
			ID: SoundID, Name: "Sound", Min: 0, Max: float64(len(Sounds) - 1), Steps: len(Sounds) - 1,
			Format: func(v float64) string { return Sounds[soundAt(v)].Short },
		},
		{
			ID: CutoffID, Name: "Cutoff", Unit: "oct", Min: -4, Max: 4, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.1f oct", v) },
		},
		{
			ID: ResID, Name: "Resonance", Min: -1, Max: 1, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.2f", v) },
		},
		{
			ID: AttackID, Name: "Attack", Min: 0.125, Max: 8, Default: 1, Log: true, Flags: plug.Automatable,
			Format: times,
		},
		{
			ID: ReleaseID, Name: "Release", Min: 0.125, Max: 8, Default: 1, Log: true, Flags: plug.Automatable,
			Format: times,
		},
		{
			ID: GainID, Name: "Gain", Unit: "dB", Min: -24, Max: 12, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.1f dB", v) },
		},
	},
	Patches: patches(),
	New:     func(h plug.Host) plug.Plugin { return newPlugin(h) },
}

func init() { plug.Register(Descriptor) }

// times writes a factor to show, as ×2.00.
func times(v float64) string { return fmt.Sprintf("×%.2f", v) }

// soundAt returns the sound the Sound parameter's value v picks.
func soundAt(v float64) int { return min(max(int(math.Round(v)), 0), len(Sounds)-1) }

// patches are a patch for each sound, named as the library names it.
func patches() []plug.Patch {
	out := make([]plug.Patch, len(Sounds))
	for i, s := range Sounds {
		out[i] = plug.Patch{Name: s.Preset, Category: s.Short, Params: map[uint32]float64{SoundID: float64(i)}}
	}
	return out
}

// sound is a sound made ready: its patch, and what it sounds like.
type sound struct {
	patch *synth.Patch
	about string
}

// sounds returns the sounds' patches, from the library.
func sounds() []sound {
	presets, err := music.Presets()
	if err != nil {
		panic("plugsynth: " + err.Error())
	}
	out := make([]sound, len(Sounds))
	for i, s := range Sounds {
		for _, p := range presets {
			if p.Name == s.Preset {
				out[i] = sound{patch: p.Patch, about: p.About}
			}
		}
		if out[i].patch == nil {
			panic("plugsynth: the patch library has no preset " + s.Preset)
		}
	}
	return out
}

// plugin is one synth.
type plugin struct {
	host   plug.Host
	sounds []sound
	proc   *processor
}

func newPlugin(h plug.Host) *plugin {
	p := &plugin{host: h, sounds: sounds()}
	p.proc = newProcessor(p.sounds)
	return p
}

// Processor implements [plug.Plugin].
func (p *plugin) Processor() plug.Processor { return p.proc }

// State implements [plug.Plugin]: the synth keeps nothing but its
// parameters.
func (p *plugin) State() []byte { return nil }

// SetState implements [plug.Plugin]: it takes only an empty state.
func (p *plugin) SetState(b []byte) error {
	if len(b) > 0 {
		return errors.New("plugsynth: the synth keeps no state")
	}
	return nil
}

// Close implements [plug.Plugin].
func (p *plugin) Close() {}

// lookahead is how many of the engine's frames past the one it gives
// the resampler reads: the cubic's two after it, and one for rounding.
const lookahead = 3

// processor plays the notes it is given on the sound chosen, at the
// engine's rate, and resamples them to the host's.
type processor struct {
	// ins are an instrument for each sound, and cur the one playing;
	// prev is the one played before, ringing out.
	ins       []*synth.Instrument
	cur, prev *synth.Instrument
	knobs     synth.Knobs
	// rate is the host's rate, and step how many of the engine's frames
	// pass in one of the host's.
	rate, step float64
	// bl and br hold the engine's sound, have frames of it, the first
	// the one before pos, where the next frame for the host is read; tl
	// and tr are room for the sound ringing out.
	bl, br, tl, tr []float32
	have           int
	pos            float64
}

func newProcessor(ss []sound) *processor {
	p := &processor{}
	for _, s := range ss {
		in, err := synth.NewInstrument(s.patch)
		if err != nil {
			panic("plugsynth: " + err.Error())
		}
		p.ins = append(p.ins, in)
	}
	p.cur = p.ins[0]
	p.Prepare(plug.Setup{Rate: synth.Rate, MaxBlock: 512, Channels: 2})
	return p
}

// Prepare implements [plug.Processor].
func (p *processor) Prepare(s plug.Setup) {
	p.rate = s.Rate
	if p.rate <= 0 {
		p.rate = synth.Rate
	}
	p.step = synth.Rate / p.rate
	n := int(math.Ceil(float64(max(s.MaxBlock, 1))*p.step)) + lookahead + 4
	p.bl, p.br = make([]float32, n), make([]float32, n)
	p.tl, p.tr = make([]float32, n), make([]float32, n)
	p.Reset()
}

// Reset implements [plug.Processor]: every note ends, at once.
func (p *processor) Reset() {
	for _, in := range p.ins {
		in.Silence()
	}
	p.prev = nil
	// The frame before the first is silence.
	p.bl[0], p.br[0] = 0, 0
	p.have, p.pos = 1, 1
}

// Latency implements [plug.Processor]: the frames the resampler reads
// ahead, as the host counts them.
func (p *processor) Latency() int { return int(math.Ceil(lookahead / p.step)) }

// Tail implements [plug.Processor]: two seconds, as long as the
// sounds' releases ring.
func (p *processor) Tail() int { return int(2 * p.rate) }

// Process implements [plug.Processor]: each change and each note takes
// effect from its frame on.
func (p *processor) Process(b *plug.Block) {
	ci, ei := 0, 0
	for at := 0; at < b.Frames; {
		for ci < len(b.Changes) && b.Changes[ci].Frame <= at {
			p.take(b.Changes[ci])
			ci++
		}
		for ei < len(b.Events) && b.Events[ei].Frame <= at {
			p.play(b.Events[ei])
			ei++
		}
		next := b.Frames
		if ci < len(b.Changes) {
			next = min(next, b.Changes[ci].Frame)
		}
		if ei < len(b.Events) {
			next = min(next, b.Events[ei].Frame)
		}
		p.fill(b.Out, at, next)
		at = next
	}
	for ; ci < len(b.Changes); ci++ {
		p.take(b.Changes[ci])
	}
	for ; ei < len(b.Events); ei++ {
		p.play(b.Events[ei])
	}
}

// take takes a change to a parameter.
func (p *processor) take(c plug.Change) {
	prm, ok := Descriptor.Param(c.ID)
	if !ok {
		return
	}
	v := prm.Clamp(c.Value)
	switch c.ID {
	case SoundID:
		in := p.ins[soundAt(v)]
		if in == p.cur {
			return
		}
		// The sound before rings out, its notes let go.
		if p.prev != nil {
			p.prev.Silence()
		}
		p.cur.Release()
		p.prev, p.cur = p.cur, in
	case CutoffID:
		p.knobs.Cutoff = v
	case ResID:
		p.knobs.Res = v
	case AttackID:
		p.knobs.Attack = math.Log2(v)
	case ReleaseID:
		p.knobs.Release = math.Log2(v)
	case GainID:
		p.knobs.Gain = v
	}
	p.cur.SetKnobs(p.knobs)
}

// play plays a note's event on the sound playing. The synth takes a
// note's bend; it leaves its pressure and modulation be.
func (p *processor) play(e plug.Event) {
	switch e.Kind {
	case plug.NoteOn:
		p.cur.NoteOn(e.Note, e.Pitch, e.Velocity, e.Group)
	case plug.NoteOff:
		p.cur.NoteOff(e.Note)
	case plug.Expression:
		if e.Expr == plug.Bend {
			p.cur.Bend(e.Note, e.Value)
		}
	case plug.Pedal:
		p.cur.Pedal(e.Group, e.Value)
	}
}

// fill writes the frames of out from index from up to index to,
// resampled from the engine's sound, which it makes as far as it needs.
func (p *processor) fill(out [][]float32, from, to int) {
	if to <= from {
		return
	}
	// Drop the frames read, but the one before pos.
	if drop := int(p.pos) - 1; drop > 0 {
		copy(p.bl, p.bl[drop:p.have])
		copy(p.br, p.br[drop:p.have])
		p.have -= drop
		p.pos -= float64(drop)
	}
	if need := int(p.pos+float64(to-from-1)*p.step) + lookahead; need > p.have {
		p.render(min(need, len(p.bl)) - p.have)
	}
	bl, br := p.bl, p.br
	for j := from; j < to; j++ {
		i := int(p.pos)
		f := float32(p.pos - float64(i))
		l := cubic(bl[i-1], bl[i], bl[i+1], bl[i+2], f)
		r := cubic(br[i-1], br[i], br[i+1], br[i+2], f)
		switch len(out) {
		case 0:
		case 1:
			out[0][j] = (l + r) / 2
		default:
			out[0][j], out[1][j] = l, r
			for _, ch := range out[2:] {
				ch[j] = 0
			}
		}
		p.pos += p.step
	}
}

// render makes n frames more of the engine's sound.
func (p *processor) render(n int) {
	l, r := p.bl[p.have:p.have+n], p.br[p.have:p.have+n]
	p.cur.Render(l, r)
	if p.prev != nil {
		if !p.prev.Sounding() {
			p.prev = nil
		} else {
			tl, tr := p.tl[:n], p.tr[:n]
			p.prev.Render(tl, tr)
			for i := range l {
				l[i] += tl[i]
				r[i] += tr[i]
			}
		}
	}
	p.have += n
}

// cubic returns the curve through a, b, c and d, a frame apart, at f
// from b towards c: a Catmull-Rom spline, which passes through each.
func cubic(a, b, c, d, f float32) float32 {
	c1 := 0.5 * (c - a)
	c2 := a - 2.5*b + 2*c - 0.5*d
	c3 := 0.5*(d-a) + 1.5*(b-c)
	return ((c3*f+c2)*f+c1)*f + b
}
