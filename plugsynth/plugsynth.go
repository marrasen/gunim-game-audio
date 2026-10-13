// Package plugsynth is the engine's synth as a plugin in gunim's
// [plug] format: an instrument that plays any patch, live, note by note,
// with knobs for the filter, the envelope and the level, and for a swell
// and a ghost note ahead of the notes a host tells of before they land.
// It offers every sound the module has, in a browser: the patch library's
// presets, General MIDI's 128 instruments and its drum kits, and the
// patches the songs play. Its state holds the whole patch, so a host
// may hand it one of its own too. It registers itself as it starts;
// import it for that:
//
//	import _ "github.com/marrasen/gunim-game-audio/plugsynth"
//
// The engine runs at [synth.Rate], 48 kHz. A host at another rate gets
// the sound resampled to its own, through a cubic interpolation, which
// costs a few frames of latency the processor reports.
package plugsynth

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
)

// The parameters' IDs. ID 1 picked one of six sounds before the state
// held the patch, and stays unused.
const (
	// CutoffID moves the filter's cutoff, in octaves, and ResID adds to
	// its resonance.
	CutoffID uint32 = iota + 2
	ResID
	// AttackID and ReleaseID stretch the envelope's attack and release,
	// as factors.
	AttackID
	ReleaseID
	// GainID turns the sound up, in decibels.
	GainID
	// SwellID is how long, in milliseconds, a swell rises into each
	// note before it lands, and GhostID how long before it a quiet
	// grace note sounds: zero for none. They sound for the notes the
	// host tells of ahead.
	SwellID
	GhostID
)

// lookAhead is how long before a note the synth asks to hear of it: the
// longest swell.
const lookAhead = time.Second

// Descriptor is the plugin, as it is registered.
var Descriptor = plug.Descriptor{
	ID:      "marrasen.gamesynth",
	Name:    "Game Synth",
	Vendor:  "Marras",
	Version: "0.2.0",
	Kind:    "Instrument|Synth",
	Notes:   true,
	// The swell and the ghost sound ahead of the notes.
	LookAhead: lookAhead,
	Params: []plug.Param{
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
		{
			ID: SwellID, Name: "Swell", Unit: "ms", Max: float64(lookAhead / time.Millisecond),
			Flags: plug.Automatable, Format: millis,
		},
		{ID: GhostID, Name: "Ghost", Unit: "ms", Max: 500, Flags: plug.Automatable, Format: millis},
	},
	New: func(h plug.Host) plug.Plugin { return newPlugin(h) },
}

func init() { plug.Register(Descriptor) }

// millis writes a time to show, as 250 ms, or Off for none.
func millis(v float64) string {
	if v <= 0 {
		return "Off"
	}
	return fmt.Sprintf("%.0f ms", v)
}

// times writes a factor to show, as ×2.00.
func times(v float64) string { return fmt.Sprintf("×%.2f", v) }

// DefaultSound is the sound the plugin plays with an empty state.
const DefaultSound = "Juno pad"

// defaultState is the state of DefaultSound.
var defaultState = sync.OnceValue(func() State {
	s, ok := Named(DefaultSound)
	if !ok {
		panic("plugsynth: the patch library has no preset " + DefaultSound)
	}
	return State{Bank: s.Bank, Group: s.Group, Name: s.Name, Patch: s.Patch()}
})

// plugin is one synth.
type plugin struct {
	host plug.Host
	proc *processor
	// mu guards st and raw, which State and SetState reach from the
	// app's goroutine and the views from the window's.
	mu  sync.Mutex
	st  State
	raw []byte
}

func newPlugin(h plug.Host) *plugin {
	st := defaultState()
	snd, err := newSound(st.Patch)
	if err != nil {
		panic("plugsynth: " + err.Error())
	}
	return &plugin{host: h, st: st, proc: newProcessor(snd)}
}

// Processor implements [plug.Plugin].
func (p *plugin) Processor() plug.Processor { return p.proc }

// State implements [plug.Plugin]: the sound, as [State] writes it, or
// nil for the default sound, as the plugin starts.
func (p *plugin) State() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.raw
}

// SetState implements [plug.Plugin]: it plays the sound the state holds,
// and DefaultSound for an empty one. A state it cannot read, or whose
// patch it cannot play, changes nothing. The sound is made here, and
// handed to the processor whole, which takes it as its next block
// starts: the notes held start again on it, and those of the sound
// before ring out.
func (p *plugin) SetState(b []byte) error {
	st := defaultState()
	if len(b) > 0 {
		var err error
		if st, err = DecodeState(b); err != nil {
			return err
		}
	}
	snd, err := newSound(st.Patch)
	if err != nil {
		return fmt.Errorf("plugsynth: %s: %w", st.Name, err)
	}
	p.mu.Lock()
	p.st, p.raw = st, nil
	if len(b) > 0 {
		p.raw = st.Encode()
	}
	p.mu.Unlock()
	p.proc.offer(snd)
	return nil
}

// sound returns the state the plugin plays.
func (p *plugin) sound() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st
}

// pick plays sound s, as the browser picks it.
func (p *plugin) pick(s Sound) {
	// The library's own sounds always load.
	_ = p.SetState(s.State())
}

// Close implements [plug.Plugin].
func (p *plugin) Close() {}

// before is how many of the engine's frames, silent, the buffer holds
// before its first: the cubic reads the one before where it reads, and
// the first host frame reads up to 3 frames back.
const before = 4

// processor plays the notes it is given on the sound chosen, at the
// engine's rate, and resamples them to the host's.
//
// The engine runs in step with the host: before an event at the host's
// frame f, it has made every frame before f*step, its own time then, so
// the event takes effect there. The host's frame j reads the engine at
// (j-delay)*step, delay frames back, which it has always made by then.
type processor struct {
	// cur is the sound playing, and prev the one played before it,
	// ringing out; next is a sound SetState made, for the next block to
	// take. pre plays cur's sound ahead of the notes.
	cur, prev, pre *synth.Instrument
	next           atomic.Pointer[voicing]
	knobs          synth.Knobs
	// ahead are the notes told of, and swell and ghost the knobs'
	// times, in the engine's frames.
	ahead        aheadNotes
	swell, ghost int64
	// rate is the host's rate, and step how many of the engine's frames
	// pass in one of the host's; delay is how many of the host's frames
	// the sound comes late.
	rate, step float64
	delay      int
	// bl and br hold the engine's sound, from its frame base on, have
	// frames of it; tl and tr are room for the sound ringing out, and
	// al and ar for the sound ahead of the notes.
	bl, br, tl, tr, al, ar []float32
	base                   int64
	have                   int
	// at counts the host's frames since the last Reset.
	at int64
	// latency and tail are what Latency and Tail report, which they read
	// on the host's goroutine as blocks run on the audio thread.
	latency, tail atomic.Int64
}

func newProcessor(snd *voicing) *processor {
	p := &processor{cur: snd.in, pre: snd.pre}
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
	// The host's frame j reads up to the engine's frame
	// floor((j-delay)*step)+2, which must come before (j+1)*step, the
	// engine's time at the host's next frame.
	p.delay = max(int(math.Ceil(3/p.step-1)), 0)
	p.latency.Store(int64(p.delay))
	n := int(math.Ceil(float64(max(s.MaxBlock, 1))*p.step)) + before + 8
	p.bl, p.br = make([]float32, n), make([]float32, n)
	p.tl, p.tr = make([]float32, n), make([]float32, n)
	p.al, p.ar = make([]float32, aheadStep), make([]float32, aheadStep)
	p.Reset()
}

// Reset implements [plug.Processor]: every note ends at once, every
// pedal lifts, every bend goes back to none, and the notes told of are
// forgotten.
func (p *processor) Reset() {
	p.adopt()
	p.cur.Silence()
	p.pre.Silence()
	p.ahead.n = 0
	p.prev = nil
	clear(p.bl[:before])
	clear(p.br[:before])
	p.base, p.have, p.at = -before, before, 0
	p.keepTail()
}

// Latency implements [plug.Processor]: an event at the host's frame f
// sounds from frame f+Latency, give or take a frame of the engine's
// where the rates differ.
func (p *processor) Latency() int { return int(p.latency.Load()) }

// Tail implements [plug.Processor]: as long as the sound playing now
// rings once its notes are let go, plus the delay. It counts the current
// sound alone; a sound picked before it, still ringing out, may ring
// longer. It reads what the processor last kept.
func (p *processor) Tail() int { return int(p.tail.Load()) }

// keepTail keeps the tail as it stands now, for Tail.
func (p *processor) keepTail() {
	p.tail.Store(int64(math.Ceil(p.cur.Tail()*p.rate)) + int64(p.delay))
}

// Process implements [plug.Processor]: each change and each note takes
// effect from its frame on.
func (p *processor) Process(b *plug.Block) {
	swapped := p.adopt()
	p.prune(int64(math.Ceil(p.engineAt(p.at))))
	for _, u := range b.Upcoming {
		p.hear(u)
	}
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
	p.at += int64(b.Frames)
	if swapped || len(b.Changes) > 0 {
		p.keepTail()
	}
}

// voicing is a sound to play: in plays the notes, and pre the same sound
// ahead of them.
type voicing struct{ in, pre *synth.Instrument }

// newSound returns the sound of patch pt.
func newSound(pt *synth.Patch) (*voicing, error) {
	in, err := synth.NewInstrument(pt)
	if err != nil {
		return nil, err
	}
	pre, err := synth.NewInstrument(pt)
	if err != nil {
		return nil, err
	}
	return &voicing{in: in, pre: pre}, nil
}

// offer hands the processor sound snd, made off the audio thread, for
// its next block to take. A sound offered before it, still untaken,
// is dropped.
func (p *processor) offer(snd *voicing) { p.next.Store(snd) }

// adopt takes the sound offered, if any, and reports whether it did:
// the notes held start again on it, at the knobs as they stand, and the
// sound before rings out. A sound still ringing out from before that
// stops at once.
func (p *processor) adopt() bool {
	snd := p.next.Swap(nil)
	if snd == nil {
		return false
	}
	in := snd.in
	in.SetKnobs(p.knobs)
	snd.pre.SetKnobs(p.knobs)
	p.pre.Silence()
	p.pre = snd.pre
	p.ahead.restart()
	if p.prev != nil {
		p.prev.Silence()
	}
	p.cur.Pass(in)
	p.prev, p.cur = p.cur, in
	return true
}

// take takes a change to a parameter.
func (p *processor) take(c plug.Change) {
	prm, ok := Descriptor.Param(c.ID)
	if !ok {
		return
	}
	v := prm.Clamp(c.Value)
	switch c.ID {
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
	case SwellID:
		p.swell = int64(math.Round(v * synth.Rate / 1000))
		return
	case GhostID:
		p.ghost = int64(math.Round(v * synth.Rate / 1000))
		return
	}
	p.cur.SetKnobs(p.knobs)
	p.pre.SetKnobs(p.knobs)
}

// play plays a note's event on the sound playing. The synth takes a
// note's bend; it leaves its pressure and modulation be.
func (p *processor) play(e plug.Event) {
	switch e.Kind {
	case plug.NoteOn:
		p.landed(e.Note)
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
// resampled from the engine's sound, which it makes up to the host's
// time at to.
func (p *processor) fill(out [][]float32, from, to int) {
	if to <= from {
		return
	}
	j0 := p.at + int64(from)
	// Drop the frames before the first the cubic reads.
	if drop := int(int64(math.Floor(p.engineAt(j0-int64(p.delay)))) - 1 - p.base); drop > 0 {
		copy(p.bl, p.bl[drop:p.have])
		copy(p.br, p.br[drop:p.have])
		p.have -= drop
		p.base += int64(drop)
	}
	end := int64(math.Ceil(p.engineAt(p.at + int64(to))))
	if n := int(end - p.base - int64(p.have)); n > 0 {
		p.render(n)
	}
	bl, br := p.bl, p.br
	for j := from; j < to; j++ {
		u := p.engineAt(p.at+int64(j)-int64(p.delay)) - float64(p.base)
		i := int(math.Floor(u))
		f := float32(u - float64(i))
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
	}
}

// engineAt returns the engine's time at the host's frame j, in its
// frames: on a frame where it falls within rounding of one, so a host
// frame that falls on an engine frame reads it whole.
func (p *processor) engineAt(j int64) float64 {
	u := float64(j) * p.step
	if r := math.Round(u); math.Abs(u-r) < 1e-6 {
		return r
	}
	return u
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
	if p.aheadOn() {
		p.renderAhead(l, r, p.base+int64(p.have))
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
