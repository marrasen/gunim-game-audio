package plugfx

import (
	"errors"
	"math"
	"sync/atomic"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// The delay's parameters' IDs.
const (
	// SyncID times the echoes by the song's tempo, in notes, where on,
	// and by TimeID where off.
	SyncID uint32 = iota + 1
	// NoteID is the note the echoes are apart, synced: one of
	// [NoteNames].
	NoteID
	// FeelID makes the note straight, dotted or a triplet's: one of
	// [FeelNames].
	FeelID
	// TimeID is how far apart the echoes are, unsynced, in
	// milliseconds.
	TimeID
	// FeedbackID is how loud each time round is, of the one before, in
	// percent.
	FeedbackID
	// PingPongID sends the echoes from side to side, where on, and
	// keeps each on its side where off.
	PingPongID
	// LowCutID and HighCutID are where the feedback's filters cut the
	// lows and the highs, in hertz.
	LowCutID
	HighCutID
	// DelayMixID is how much of the echoes is heard, in percent: 100,
	// the echoes alone, for a send, as it starts.
	DelayMixID
)

// NoteNames are the notes NoteID picks, by its value, and noteBeats
// each one's length in beats, a beat a quarter note.
var (
	NoteNames = []string{"1/32", "1/16", "1/8", "1/4", "1/2", "1/1"}
	noteBeats = []float64{0.125, 0.25, 0.5, 1, 2, 4}
)

// FeelNames are the feels FeelID picks, by its value, and feelTimes how
// each stretches a note.
var (
	FeelNames = []string{"Straight", "Dotted", "Triplet"}
	feelTimes = []float64{1, 1.5, 2.0 / 3}
)

// longestDelay is the longest time apart the echoes may be, in seconds,
// and delayFade how long a change of time crossfades for.
const (
	longestDelay = 4
	delayFade    = 0.03
)

// defaultTempo is the tempo a synced delay keeps where the host knows
// none, in beats a minute.
const defaultTempo = 120

// DelayDescriptor is the delay, as it is registered.
var DelayDescriptor = plug.Descriptor{
	ID:      "marrasen.delay",
	Name:    "Delay",
	Vendor:  "Marras",
	Version: "0.1.0",
	Kind:    "Fx|Delay",
	Params: []plug.Param{
		{ID: SyncID, Name: "Sync", Max: 1, Default: 1, Steps: 1, Flags: plug.Automatable, Format: onOff},
		{ID: NoteID, Name: "Note", Max: float64(len(NoteNames) - 1), Default: 2, Steps: len(NoteNames) - 1,
			Flags: plug.Automatable, Format: func(v float64) string { return NoteNames[stepOf(v, len(NoteNames))] }},
		{ID: FeelID, Name: "Feel", Max: float64(len(FeelNames) - 1), Default: 1, Steps: len(FeelNames) - 1,
			Flags: plug.Automatable, Format: func(v float64) string { return FeelNames[stepOf(v, len(FeelNames))] }},
		{ID: TimeID, Name: "Time", Unit: "ms", Min: 1, Max: longestDelay * 1000, Default: 375, Log: true,
			Flags: plug.Automatable, Format: millis},
		{ID: FeedbackID, Name: "Feedback", Unit: "%", Max: 95, Default: 35, Flags: plug.Automatable, Format: percent},
		{ID: PingPongID, Name: "Ping-pong", Max: 1, Default: 1, Steps: 1, Flags: plug.Automatable, Format: onOff},
		{ID: LowCutID, Name: "Low cut", Unit: "Hz", Min: 20, Max: 2000, Default: 250, Log: true,
			Flags: plug.Automatable, Format: hertz},
		{ID: HighCutID, Name: "High cut", Unit: "Hz", Min: 500, Max: 20000, Default: 4400, Log: true,
			Flags: plug.Automatable, Format: hertz},
		{ID: DelayMixID, Name: "Mix", Unit: "%", Max: 100, Default: 100, Flags: plug.Automatable, Format: percent},
	},
	Patches: []plug.Patch{
		{Name: "Default", Category: "Send"},
		{Name: "Slapback", Category: "Send", Params: map[uint32]float64{SyncID: 0, TimeID: 110, FeedbackID: 10,
			PingPongID: 0, HighCutID: 6000}},
		{Name: "Quarter echoes", Category: "Send", Params: map[uint32]float64{NoteID: 3, FeelID: 0, FeedbackID: 45}},
		{Name: "Triplet bounce", Category: "Send", Params: map[uint32]float64{NoteID: 2, FeelID: 2, FeedbackID: 40}},
		{Name: "Dub", Category: "Send", Params: map[uint32]float64{NoteID: 3, FeelID: 1, FeedbackID: 75, LowCutID: 400,
			HighCutID: 1800}},
		{Name: "Vocal throw", Category: "Insert", Params: map[uint32]float64{NoteID: 3, FeelID: 0, FeedbackID: 30,
			DelayMixID: 25}},
	},
	New: func(h plug.Host) plug.Plugin { return &delayPlugin{} },
}

// stepOf returns a stepped parameter's value as an index into n names.
func stepOf(v float64, n int) int { return min(max(int(math.Round(v)), 0), n-1) }

// delayPlugin is one delay.
type delayPlugin struct{ proc delayProc }

// Processor implements [plug.Plugin].
func (p *delayPlugin) Processor() plug.Processor { return &p.proc }

// State implements [plug.Plugin]: its parameters hold all it keeps.
func (p *delayPlugin) State() []byte { return nil }

// SetState implements [plug.Plugin]: it takes only an empty state.
func (p *delayPlugin) SetState(b []byte) error {
	if len(b) > 0 {
		return errors.New("plugfx: the delay keeps no state")
	}
	return nil
}

// Close implements [plug.Plugin].
func (p *delayPlugin) Close() {}

// View implements [plug.Viewer].
func (p *delayPlugin) View(s plug.Size, h plug.Host) gunim.Node { return delayView(s, h) }

// delaySettings are a delay's parameters, as the processor takes them
// and the views draw them.
type delaySettings struct {
	sync, pingPong bool
	note, feel     int
	// ms is the time unsynced, feedback from 0 to 0.95, and the cuts in
	// hertz.
	ms, feedback, lowCut, highCut float64
}

// take sets the parameter of ID id to v, where it is one of the
// settings.
func (s *delaySettings) take(id uint32, v float64) {
	switch id {
	case SyncID:
		s.sync = v > 0.5
	case NoteID:
		s.note = stepOf(v, len(NoteNames))
	case FeelID:
		s.feel = stepOf(v, len(FeelNames))
	case TimeID:
		s.ms = v
	case FeedbackID:
		s.feedback = v / 100
	case PingPongID:
		s.pingPong = v > 0.5
	case LowCutID:
		s.lowCut = v
	case HighCutID:
		s.highCut = v
	}
}

// beats is the synced note's length in beats.
func (s *delaySettings) beats() float64 { return noteBeats[s.note] * feelTimes[s.feel] }

// seconds is how far apart the echoes are at tempo, in seconds, within
// the longest.
func (s *delaySettings) seconds(tempo float64) float64 {
	t := s.ms / 1000
	if s.sync {
		t = s.beats() * 60 / tempo
	}
	return min(t, longestDelay)
}

// tail is how long the echoes ring on after the sound stops, for
// echoes apart secs seconds, in seconds: till the feedback has turned
// them down 60 dB, and an echo more, which ping-pong takes two to, and
// the filters' ringing.
func (s *delaySettings) tail(secs float64) float64 {
	rounds := 0.0
	if s.feedback > 0.001 {
		rounds = math.Ceil(-3 / math.Log10(s.feedback))
	}
	if s.pingPong {
		return 2*(rounds+1)*secs + 0.01
	}
	return (rounds+1)*secs + 0.01
}

// delayProc runs the song engine's delay on the sound fed to it, timed
// by the host's tempo where synced, and blends it with the sound.
type delayProc struct {
	rate   float64
	dly    *synth.DelayUnit
	wl, wr []float32
	blend  blend
	set    delaySettings
	// tempo is the host's tempo, the last it knew.
	tempo float64
	tail  atomic.Int64
}

// Prepare implements [plug.Processor]: everything the delay keeps is
// made here, so processing allocates zero bytes.
func (p *delayProc) Prepare(s plug.Setup) {
	p.rate = s.Rate
	if p.rate <= 0 {
		p.rate = synth.Rate
	}
	p.dly = synth.NewDelayUnit(p.rate, longestDelay, delayFade)
	n := max(s.MaxBlock, 1)
	p.wl, p.wr = make([]float32, n), make([]float32, n)
	p.blend.prepare(p.rate)
	p.blend.widthTo = 1
	if p.tempo == 0 {
		p.tempo = defaultTempo
	}
	for _, prm := range DelayDescriptor.Params {
		p.take(plug.Change{ID: prm.ID, Value: prm.Default})
	}
	p.Reset()
}

// take takes a change to a parameter.
func (p *delayProc) take(c plug.Change) {
	prm, ok := DelayDescriptor.Param(c.ID)
	if !ok {
		return
	}
	v := prm.Clamp(c.Value)
	if c.ID == DelayMixID {
		p.blend.mixTo = float32(v / 100)
		return
	}
	p.set.take(c.ID, v)
	p.apply()
}

// apply sets the delay as the settings and the tempo say.
func (p *delayProc) apply() {
	if p.dly == nil {
		return
	}
	secs := p.set.seconds(p.tempo)
	p.dly.SetTime(secs)
	p.dly.SetFeedback(p.set.feedback)
	p.dly.SetFilters(p.set.lowCut, p.set.highCut)
	p.dly.SetPingPong(p.set.pingPong)
	p.tail.Store(int64(math.Ceil(p.set.tail(secs) * p.rate)))
}

// Process implements [plug.Processor]: a new tempo times the echoes
// from the block's start.
func (p *delayProc) Process(b *plug.Block) {
	if p.dly == nil || b.Frames > len(p.wl) {
		return
	}
	if t := b.Position.Tempo; t > 0 && t != p.tempo {
		p.tempo = t
		p.apply()
	}
	runSegments(b, p)
}

// segment runs frames from to to.
func (p *delayProc) segment(b *plug.Block, from, to int) {
	feed(b.In, p.wl, p.wr, from, to)
	p.dly.Process(p.wl[from:to], p.wr[from:to])
	p.blend.run(b.In, b.Out, p.wl, p.wr, from, to)
}

// Latency implements [plug.Processor]: the delay sounds on time, 0.
func (p *delayProc) Latency() int { return 0 }

// Tail implements [plug.Processor]: till the feedback has turned the
// echoes down 60 dB.
func (p *delayProc) Tail() int { return int(p.tail.Load()) }

// Reset implements [plug.Processor]: the echoes, silenced.
func (p *delayProc) Reset() {
	if p.dly != nil {
		p.dly.Clear()
	}
	p.blend.jump()
}
