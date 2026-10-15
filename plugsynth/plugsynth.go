// Package plugsynth is the engine's synth as a plugin in gunim's
// [plug] format: an instrument that plays any patch, live, note by note,
// with knobs for the filter, the envelope and the level, and for a swell
// and a ghost note ahead of the notes a host tells of before they land.
// Its parameters turn the patch as it plays, so a host's automation and
// control wires drive the filter, the envelope, the tone, the pitch,
// the LFOs, the vowel and a mod wheel and pressure for every note; each
// note's own mod wheel and pressure, as [plug.Expression] events, move
// it as its patch answers them. Its views hold a patch editor, whose
// edits are edits of the state.
// It offers every sound the module has to the host's browser, as
// presets: the patch library's presets, General MIDI's 128 instruments
// and its drum kits, the patches the songs play, and the instruments of
// the SoundFont banks a program hands it with [UseFont], played on the
// banks' samples. Its state holds the
// whole patch, so a host may hand it one of its own too. A drum kit
// plays each part of the kit, as the kick or the hats, into an aux
// output of its own where the host connects it. It registers itself as
// it starts; import it for that:
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
	// DecayID stretches the envelope's decay, as a factor, and
	// SustainID adds to its sustain.
	DecayID
	SustainID
	// EnvID adds to how many octaves the filter's envelope opens it by.
	EnvID
	// DriveID and NoiseID add to the patch's drive and noise.
	DriveID
	NoiseID
	// TuneID moves every note, in semitones; WidthID adds to a pulse's
	// width; FMID turns an fm wave's index up, in octaves.
	TuneID
	WidthID
	FMID
	// PanID moves the sound between the speakers.
	PanID
	// LFORateID and LFODepthID scale the LFOs' speed and depth, as
	// factors.
	LFORateID
	LFODepthID
	// VowelID is the vowel every note sings: 0 for the patch's own, or
	// 1 to 5 for a, e, i, o or u.
	VowelID
	// ModID and PressureID are a mod wheel and a key's pressure on every
	// note, from 0 to 1, on top of each note's own.
	ModID
	PressureID
	// AheadVelID is how hard the swell and the grace note play, as a
	// share of their note's velocity.
	AheadVelID
)

// Vowels are the vowels VowelID picks from 1 on.
const Vowels = "aeiou"

// lookAhead is how long before a note the synth asks to hear of it: the
// longest swell.
const lookAhead = time.Second

// Descriptor is the plugin, as it is registered.
var Descriptor = plug.Descriptor{
	ID:      "marrasen.gamesynth",
	Name:    "Game Synth",
	Vendor:  "Marras",
	Version: "0.3.0",
	Kind:    "Instrument|Synth",
	Notes:   true,
	// The swell and the ghost sound ahead of the notes.
	LookAhead: lookAhead,
	AuxOut:    drumOuts(),
	Ports:     ports,
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
		{ID: DecayID, Name: "Decay", Min: 0.125, Max: 8, Default: 1, Log: true, Flags: plug.Automatable, Format: times},
		{ID: SustainID, Name: "Sustain", Min: -1, Max: 1, Flags: plug.Automatable, Format: signed},
		{
			ID: EnvID, Name: "Filter env", Unit: "oct", Min: -6, Max: 6, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.1f oct", v) },
		},
		{ID: DriveID, Name: "Drive", Min: -1, Max: 1, Flags: plug.Automatable, Format: signed},
		{ID: NoiseID, Name: "Noise", Min: -1, Max: 1, Flags: plug.Automatable, Format: signed},
		{
			ID: TuneID, Name: "Tune", Unit: "st", Min: -24, Max: 24, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.2f st", v) },
		},
		{ID: WidthID, Name: "Width", Min: -0.5, Max: 0.5, Flags: plug.Automatable, Format: signed},
		{
			ID: FMID, Name: "FM", Unit: "oct", Min: -4, Max: 4, Flags: plug.Automatable,
			Format: func(v float64) string { return fmt.Sprintf("%+.1f oct", v) },
		},
		{ID: PanID, Name: "Pan", Min: -1, Max: 1, Flags: plug.Automatable, Format: pan},
		{ID: LFORateID, Name: "LFO rate", Min: 0.125, Max: 8, Default: 1, Log: true, Flags: plug.Automatable, Format: times},
		{ID: LFODepthID, Name: "LFO depth", Max: 2, Default: 1, Flags: plug.Automatable, Format: times},
		{ID: VowelID, Name: "Vowel", Max: 5, Steps: 5, Flags: plug.Automatable, Format: vowel},
		{ID: ModID, Name: "Mod wheel", Max: 1, Flags: plug.Automatable, Format: percent},
		{ID: PressureID, Name: "Pressure", Max: 1, Flags: plug.Automatable, Format: percent},
		{ID: AheadVelID, Name: "Velocity", Max: 1, Default: 1, Flags: plug.Automatable, Format: percent},
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

// The aux outputs' indexes, one for each of a drum kit's parts, as
// [synth.DrumPart] sorts its drums: a drum plays into its part's output
// where that is connected, and into the main output where it is not.
// Any other patch plays into the main output alone, and the
// descriptor's Ports leaves its aux outputs out.
const (
	KickOut   = synth.KickPart
	SnareOut  = synth.SnarePart
	HatsOut   = synth.HatPart
	TomsOut   = synth.TomPart
	CymbalOut = synth.CymbalPart
	PercOut   = synth.PercPart
)

// drumOuts returns the aux outputs, one a part, named as the part is.
func drumOuts() []plug.AudioPort {
	out := make([]plug.AudioPort, synth.DrumParts)
	for i, name := range synth.DrumPartNames {
		out[i] = plug.AudioPort{Name: name}
	}
	return out
}

// ports says which outputs the synth has in a state: an aux output for
// each part its drum kit plays into, or none beside the main output for
// any other sound. A state it cannot read stands for the default sound,
// as the plugin plays that where its first state fails.
func ports(state []byte) plug.Ports {
	st := defaultState()
	if len(state) > 0 {
		if s, err := DecodeState(state); err == nil {
			st = s
		}
	}
	parts := synth.KitParts(st.Patch)
	return plug.Ports{AuxOut: parts[:]}
}

// times writes a factor to show, as ×2.00.
func times(v float64) string { return fmt.Sprintf("×%.2f", v) }

// signed writes a value either side of zero to show, as +0.25.
func signed(v float64) string { return fmt.Sprintf("%+.2f", v) }

// percent writes a value from 0 to 1 to show, as 40%.
func percent(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

// pan writes a place between the speakers to show, as L 40, C or R 40.
func pan(v float64) string {
	switch n := math.Round(v * 100); {
	case n < 0:
		return fmt.Sprintf("L %.0f", -n)
	case n > 0:
		return fmt.Sprintf("R %.0f", n)
	}
	return "C"
}

// vowel writes the vowel VowelID picks to show: the patch's own at 0.
func vowel(v float64) string {
	if i := int(math.Round(v)); i >= 1 && i <= len(Vowels) {
		return Vowels[i-1 : i]
	}
	return "Patch"
}

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
	p := &plugin{host: h, st: st, proc: newProcessor(snd)}
	synths.Store(p, struct{}{})
	return p
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

// pick plays sound s, as a step to it picks it.
func (p *plugin) pick(s Sound) {
	// The library's own sounds always load.
	_ = p.SetState(s.State())
}

// Close implements [plug.Plugin].
func (p *plugin) Close() { synths.Delete(p) }

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
	// aheadVel is how hard they play, as a share of their note's
	// velocity.
	aheadVel float64
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
	// parts holds each drum part's sound, from frame base on, as bl and
	// br hold the rest, while split is set: from the first block a part's
	// output is connected in, until none is and the parts' sound is
	// played out. tails is room for the parts ringing out, and room the
	// parts of the frames being made. routed counts, for each part, the
	// engine's frames up to which it played into its own room.
	parts, tails [synth.DrumParts]synth.Part
	room         [synth.DrumParts]synth.Part
	split        bool
	routed       [synth.DrumParts]int64
	// outs are the parts' outputs in the block running, nil where one is
	// unconnected.
	outs [synth.DrumParts][][]float32
	// at counts the host's frames since the last Reset.
	at int64
	// offline says the host renders offline: a bank's notes wait for
	// their samples.
	offline bool
	// latency and tail are what Latency and Tail report, which they read
	// on the host's goroutine as blocks run on the audio thread.
	latency, tail atomic.Int64
}

func newProcessor(snd *voicing) *processor {
	p := &processor{cur: snd.in, pre: snd.pre, aheadVel: 1}
	p.Prepare(plug.Setup{Rate: synth.Rate, MaxBlock: 512, Channels: 2})
	return p
}

// Prepare implements [plug.Processor].
func (p *processor) Prepare(s plug.Setup) {
	p.offline = s.Offline
	p.cur.SetOffline(s.Offline)
	p.pre.SetOffline(s.Offline)
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
	for i := range p.parts {
		p.parts[i] = synth.Part{L: make([]float32, n), R: make([]float32, n)}
		p.tails[i] = synth.Part{L: make([]float32, n), R: make([]float32, n)}
	}
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
	p.split = false
	p.routed = [synth.DrumParts]int64{}
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
	p.connect(b)
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

// newSound returns the sound of patch pt. A sound of a bank has its
// samples read in, for its first notes to play at once.
func newSound(pt *synth.Patch) (*voicing, error) {
	if f := synth.SoundFontNamed(pt.SoundFont); f != nil && pt.Kind == "" {
		go f.Preload(pt.Bank, pt.Program)
	}
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

// connect takes the block's drum outputs, and splits the parts' sound
// from the rest while any is connected or still holds sound.
func (p *processor) connect(b *plug.Block) {
	connected := false
	for i := range p.outs {
		p.outs[i] = b.AuxOutput(i)
		connected = connected || p.outs[i] != nil
	}
	if connected && !p.split {
		// The parts' room starts silent, in step with the rest.
		for i := range p.parts {
			clear(p.parts[i].L[:p.have])
			clear(p.parts[i].R[:p.have])
		}
		p.split = true
	}
	if !connected && p.split {
		held := false
		for _, end := range p.routed {
			held = held || end > p.base
		}
		p.split = held
	}
}

// offer hands the processor sound snd, made off the audio thread, for
// its next block to take. A sound offered before it, still untaken,
// is dropped.
func (p *processor) offer(snd *voicing) { p.next.Store(snd) }

// adopt takes the sound offered, if any, and reports whether it did. A
// sound of the same shape as the one playing, as a patch edited is,
// plays on in its place, the notes held playing on, unbroken, as
// [synth.Instrument.Retune] says; for any other, the notes held start
// again on it, at the knobs as they stand, and the sound before rings
// out. A sound still ringing out from before that stops at once.
func (p *processor) adopt() bool {
	snd := p.next.Swap(nil)
	if snd == nil {
		return false
	}
	in := snd.in
	in.SetOffline(p.offline)
	snd.pre.SetOffline(p.offline)
	in.SetKnobs(p.knobs)
	snd.pre.SetKnobs(p.knobs)
	if p.cur.Retune(in) {
		// The sound ahead plays the same patch, so it retunes too.
		if !p.pre.Retune(snd.pre) {
			p.pre.Silence()
			p.pre = snd.pre
		}
		return true
	}
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
	case DecayID:
		p.knobs.Decay = math.Log2(v)
	case SustainID:
		p.knobs.Sustain = v
	case EnvID:
		p.knobs.Env = v
	case DriveID:
		p.knobs.Drive = v
	case NoiseID:
		p.knobs.Noise = v
	case TuneID:
		p.knobs.Tune = v
	case WidthID:
		p.knobs.Width = v
	case FMID:
		p.knobs.FM = v
	case PanID:
		p.knobs.Pan = v
	case LFORateID:
		p.knobs.LFORate = math.Log2(v)
	case LFODepthID:
		p.knobs.LFODepth = v - 1
	case VowelID:
		p.knobs.Vowel = 0
		if i := int(math.Round(v)); i >= 1 && i <= len(Vowels) {
			p.knobs.Vowel = Vowels[i-1]
		}
	case ModID:
		p.knobs.Mod = v
	case PressureID:
		p.knobs.Pressure = v
	case SwellID:
		p.swell = int64(math.Round(v * synth.Rate / 1000))
		return
	case GhostID:
		p.ghost = int64(math.Round(v * synth.Rate / 1000))
		return
	case AheadVelID:
		p.aheadVel = min(max(v, 0), 1)
		return
	}
	p.cur.SetKnobs(p.knobs)
	p.pre.SetKnobs(p.knobs)
}

// play plays a note's event on the sound playing: its bend, and its
// mod wheel and pressure, which the patch answers as its
// [synth.Patch.Responses] say.
func (p *processor) play(e plug.Event) {
	switch e.Kind {
	case plug.NoteOn:
		p.landed(e.Note)
		p.cur.NoteOnTurn(e.Note, e.Pitch, e.Velocity, e.Group, e.Turn)
	case plug.NoteOff:
		p.cur.NoteOff(e.Note)
	case plug.Expression:
		switch e.Expr {
		case plug.Bend:
			p.cur.Bend(e.Note, e.Value)
		case plug.Mod:
			p.cur.Mod(e.Note, e.Value)
		case plug.Pressure:
			p.cur.Pressure(e.Note, e.Value)
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
		if p.split {
			for i := range p.parts {
				copy(p.parts[i].L, p.parts[i].L[drop:p.have])
				copy(p.parts[i].R, p.parts[i].R[drop:p.have])
			}
		}
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
		if p.split {
			for k := range p.parts {
				pl, pr := p.parts[k].L, p.parts[k].R
				dl := cubic(pl[i-1], pl[i], pl[i+1], pl[i+2], f)
				dr := cubic(pr[i-1], pr[i], pr[i+1], pr[i+2], f)
				if p.outs[k] != nil {
					put(p.outs[k], j, dl, dr)
				} else {
					l += dl
					r += dr
				}
			}
		}
		put(out, j, l, r)
	}
}

// put writes frame j of out: l and r on a stereo one, their mean on a
// mono one, and silence on the channels after the second.
func put(out [][]float32, j int, l, r float32) {
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

// render makes n frames more of the engine's sound. While the parts
// are split, a drum whose part's output is connected plays into the
// part's room, and the rest into bl and br, as before.
func (p *processor) render(n int) {
	l, r := p.bl[p.have:p.have+n], p.br[p.have:p.have+n]
	if !p.split {
		p.cur.Render(l, r)
	} else {
		p.cur.RenderParts(l, r, p.rooms(&p.parts, p.have, n))
	}
	if p.prev != nil {
		if !p.prev.Sounding() {
			p.prev = nil
		} else {
			tl, tr := p.tl[:n], p.tr[:n]
			if !p.split {
				p.prev.Render(tl, tr)
			} else {
				p.prev.RenderParts(tl, tr, p.rooms(&p.tails, 0, n))
				for k := range p.parts {
					add(p.parts[k].L[p.have:p.have+n], p.parts[k].R[p.have:p.have+n], p.tails[k].L[:n], p.tails[k].R[:n])
				}
			}
			add(l, r, tl, tr)
		}
	}
	if p.split {
		for k := range p.outs {
			if p.outs[k] != nil {
				p.routed[k] = p.base + int64(p.have+n)
			}
		}
	}
	if p.aheadOn() {
		p.renderAhead(l, r, p.base+int64(p.have))
	}
	p.have += n
}

// rooms returns the room the parts of the n frames being made play
// into, from frame at of parts: a part whose output is connected plays
// into its own; the others play into the rest, and their own room,
// cleared, stays silent.
func (p *processor) rooms(parts *[synth.DrumParts]synth.Part, at, n int) *[synth.DrumParts]synth.Part {
	for k := range parts {
		pl, pr := parts[k].L[at:at+n], parts[k].R[at:at+n]
		if p.outs[k] != nil {
			p.room[k] = synth.Part{L: pl, R: pr}
		} else {
			clear(pl)
			clear(pr)
			p.room[k] = synth.Part{}
		}
	}
	return &p.room
}

// add adds tl and tr to l and r, all as long as each other.
func add(l, r, tl, tr []float32) {
	for i := range l {
		l[i] += tl[i]
		r[i] += tr[i]
	}
}

// cubic returns the curve through a, b, c and d, a frame apart, at f
// from b towards c: a Catmull-Rom spline, which passes through each.
func cubic(a, b, c, d, f float32) float32 {
	c1 := 0.5 * (c - a)
	c2 := a - 2.5*b + 2*c - 0.5*d
	c3 := 0.5*(d-a) + 1.5*(b-c)
	return ((c3*f+c2)*f+c1)*f + b
}
