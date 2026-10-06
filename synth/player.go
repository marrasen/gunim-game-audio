package synth

import (
	"errors"
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim/audio/band"
)

// maxBlock is the most frames the player makes at once: it makes them
// in blocks, split where notes start, so each starts on its frame.
const maxBlock = 256

// A Player plays a [Song] without end. It is a [band.Tiered], a
// [band.Triggered] and a [band.Watcher], and does more besides: it
// plays notes on a keypad, plays stings, takes edits as it plays, and
// tells a tool what it plays, note by note.
//
// The mixer's goroutine reads it; its other methods may be called from
// any goroutine.
type Player struct {
	seed uint64
	rng  *rng

	// Set from any goroutine.
	pending   atomic.Pointer[compiled]
	tierReq   atomic.Int32
	waiting   atomic.Bool
	mu        sync.Mutex
	ctl       map[string]band.PartControl
	keys      []int
	stingReq  string
	tiersSeen atomic.Int32

	// The mixer's goroutine alone.
	audit *track
	// auditPatch names the patch auditioned last.
	auditPatch string
	watched    *track
	solo       bool
	c          *compiled
	tracks     []*track
	all        []*track
	ghosts     []*track
	kp         *track
	trans      *track
	duckT      *track
	sting      *stingRun
	stingNow   string
	stopped    bool
	at         int64
	bar        int
	origin     int
	barStart   int64
	barEnd     int64
	tier       int
	risen      bool
	queue      []sched
	qi         int
	evs        []pev
	wanderN    int
	ml, mr     []float32
	rl, rr     []float32
	dl, dr     []float32
	tl, tr     []float32
	duckBuf    []float32
	duckEnv    float32
	duckOn     bool
	rev        *reverb
	dly        *delay
	comp       *compressor
	lim        *limiter
	gain       float32
	// low cuts the rumble under the music, as mastering does.
	low [2]svf
	ctx renderCtx

	// What a tool reads, guarded by wmu.
	wmu    sync.Mutex
	look   Look
	bars   [barRing]barInfo
	nbars  int
	notes  [noteRing]Note
	nnotes int
	err    error
	// levels are the tracks, to read their levels from: replaced, never
	// changed, as songs change.
	levels []*track
	// scope holds the last frames the watched track made, mono, for
	// Scope, scopeAt where the next goes.
	scope   [scopeFrames]float32
	scopeAt int
	// reduction is how far the compressor turns the mix down, in
	// decibels, as float bits.
	reduction atomic.Uint32
	// watch names the track Scope watches, and auditions are the notes
	// asked for with Audition, guarded by mu.
	watch     string
	auditions []audition
	played    atomic.Int64
}

// A track is a track playing.
type track struct {
	c      *ctrack
	name   string
	vs     *voices
	hs     *hits
	ins    inserts
	active bool
	// since is how many phrases the track has played since it came in.
	since   int
	leaving bool
	// arpStep counts the arpeggio's notes since its chord came in.
	arpStep  int
	arpChord int
	arpBuf   []int
	voicing  []int
	pcycle   []int
	pevs     [][]pev
	// lv are the track's peaks, left and right, and its RMS levels, as
	// float bits, and pk and ms the peaks and mean squares they show.
	lv       [4]atomic.Uint32
	pk       [2]float32
	ms       [2]float32
	rng      *rng
	sounding bool
}

// A sched is a note or a drum to start on its frame.
type sched struct {
	frame int64
	t     *track
	drum  bool
	n     note
	d     drum
	pitch float32
	dur   int64
	tune  float32
}

type stingRun struct {
	name   string
	cs     *csting
	start  int
	tracks []*track
}

// Note is a note a player plays, as a tool shows it.
type Note struct {
	// Frame is the frame it starts on, counting the player's frames from
	// its start, and Len how many frames it is held.
	Frame int64
	Len   int64
	// Track names the track that plays it: a song's track, keypad, or
	// a sting's track.
	Track string
	// Pitch is its note, or for a drum the drum's chord note, 0 for
	// none.
	Pitch int
	Vel   float32
	// Drum names the drum, for a drum, as its kit does: bd, or oh.
	Drum string
}

// Look is what a player plays, as a tool shows it.
type Look struct {
	// Frame is how many frames the player has made.
	Frame int64
	// Bar is the song's bar at the frame asked about, from 0, which
	// started at BarFrame and lasts BarFrames frames; BeatsPerBar and
	// PhraseBars are the song's.
	Bar         int
	BarFrame    int64
	BarFrames   float64
	BeatsPerBar int
	PhraseBars  int
	// Tier and Tiers are a tiers song's; Wander says the song wanders.
	Tier, Tiers int
	Wander      bool
	// Chord is the chord playing, at ChordIndex in Chords, the
	// progression; Spelled are the chords by their roots' names, as B
	// for V in E major.
	Chord      string
	ChordIndex int
	Chords     []string
	Spelled    []string
	Tracks     []TrackLook
	// Sting names the sting playing, and Stopped says a sting has ended
	// the song.
	Sting   string
	Stopped bool
	// Keypad says the song plays notes for keys.
	Keypad bool
	// Stings names the song's stings.
	Stings []string
}

// TrackLook is what a track plays, as a tool shows it.
type TrackLook struct {
	Name, Color string
	Tier        int
	Core        bool
	Drums       bool
	Playing     bool
	Piece       band.Piece
	Control     band.PartControl
	// Level is how loud it is now, from 0 to 1 at full scale; Peak are
	// its peaks, left and right, falling back slowly, and RMS its RMS
	// levels, over some 300 ms. Mute and Solo are its own.
	Level      float32
	Peak, RMS  [2]float32
	Mute, Solo bool
}

// barInfo is what a bar plays, for Look.
type barInfo struct {
	frame  int64
	look   Look
	chords []chordSpan
}

// chordSpan is a chord within a bar: where it starts, in bars.
type chordSpan struct {
	at    float64
	name  string
	index int
}

const (
	barRing  = 32
	noteRing = 4096
)

// NewPlayer returns a player of s, its choices seeded by seed. A song
// that does not compile plays silence, and [Player.Err] says why.
func NewPlayer(s *Song, seed uint64) *Player {
	p := &Player{seed: seed, rng: newRand(seed), ctl: map[string]band.PartControl{}, bar: -1,
		rev: newReverb(), dly: newDelay(), comp: newCompressor(-10, 2), lim: newLimiter(-0.8), gain: 1}
	for _, b := range []*[]float32{&p.ml, &p.mr, &p.rl, &p.rr, &p.dl, &p.dr, &p.tl, &p.tr, &p.duckBuf} {
		*b = make([]float32, maxBlock)
	}
	p.ctx.l, p.ctx.r = make([]float32, control), make([]float32, control)
	p.low[0].set(28, 0.1)
	p.low[1].set(28, 0.1)
	c, err := compile(s)
	if err != nil {
		p.err = err
		return p
	}
	p.tierReq.Store(int32(c.start))
	p.tier = c.start
	p.adopt(c)
	return p
}

// Err returns what kept the song from playing, if anything.
func (p *Player) Err() error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	return p.err
}

// SetSong plays s in place of the song playing, from the next block on:
// a patch or a mix changed is heard at once, a pattern from the next
// bar, and the song's place, tier and parts carry on. It returns what
// keeps s from playing, and plays on as it was then.
func (p *Player) SetSong(s *Song) error {
	c, err := compile(s)
	if err != nil {
		return err
	}
	p.pending.Store(c)
	return nil
}

// Tiers implements [band.Tiered].
func (p *Player) Tiers() int {
	if c := p.pending.Load(); c != nil {
		return max(c.tiers, 1)
	}
	return max(int(p.tiersSeen.Load()), 1)
}

// Tier implements [band.Tiered].
func (p *Player) Tier() int { return int(p.tierReq.Load()) }

// SetTier implements [band.Tiered]: the tier changes at the next
// phrase, with a riser on the way up where the song marks its
// transitions.
func (p *Player) SetTier(n int) {
	p.tierReq.Store(int32(min(max(n, 1), p.Tiers())))
}

// SetPart implements [band.Triggered].
func (p *Player) SetPart(name string, c band.PartControl) error {
	p.wmu.Lock()
	found := false
	for _, t := range p.look.Tracks {
		found = found || t.Name == name
	}
	p.wmu.Unlock()
	if !found {
		return band.ErrNoPart
	}
	p.mu.Lock()
	p.ctl[name] = c
	p.mu.Unlock()
	return nil
}

// Part implements [band.Triggered].
func (p *Player) Part(name string) band.PartControl {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ctl[name]
}

// Key plays the keypad's note for digit d, from 0 to 9, at once, where
// the song has a keypad: each digit a note of the key's pentatonic
// scale, unless the song tunes them otherwise, so a player's typing
// always fits the music.
func (p *Player) Key(d int) {
	if d < 0 || d > 9 {
		return
	}
	p.mu.Lock()
	p.keys = append(p.keys, d)
	p.mu.Unlock()
	p.waiting.Store(true)
}

// ErrNoSting is returned for a sting a song has none of.
var ErrNoSting = errors.New("synth: no such sting")

// Sting plays the sting named name in place of the song, from the next
// beat: the song stops there, and the sting plays, and then silence, or
// the song again from its first tier where the sting resumes it.
func (p *Player) Sting(name string) error {
	p.wmu.Lock()
	ok := slices.Contains(p.look.Stings, name)
	p.wmu.Unlock()
	if !ok {
		return ErrNoSting
	}
	p.mu.Lock()
	p.stingReq = name
	p.mu.Unlock()
	p.waiting.Store(true)
	return nil
}

// Read implements [audio.Source].
func (p *Player) Read(dst []float32) (int, error) {
	n := len(dst) / 2
	if p.c == nil {
		clear(dst[:2*n])
		return n, nil
	}
	for done := 0; done < n; {
		if c := p.pending.Swap(nil); c != nil {
			p.adopt(c)
		}
		if p.waiting.Swap(false) {
			p.takeRequests()
		}
		if p.at >= p.barEnd {
			p.startBar()
		}
		p.liftLate()
		for p.qi < len(p.queue) && p.queue[p.qi].frame <= p.at {
			p.fire(&p.queue[p.qi])
			p.qi++
		}
		seg := int64(min(n-done, maxBlock))
		seg = min(seg, p.barEnd-p.at)
		if p.qi < len(p.queue) {
			seg = min(seg, p.queue[p.qi].frame-p.at)
		}
		seg = max(seg, 1)
		p.render(dst[2*done:2*(done+int(seg))], int(seg))
		p.at += seg
		done += int(seg)
	}
	p.played.Store(p.at)
	return n, nil
}

// takeRequests takes the keys pressed and the sting asked for.
func (p *Player) takeRequests() {
	p.mu.Lock()
	keys := p.keys
	p.keys = nil
	sting := p.stingReq
	p.stingReq = ""
	as := p.auditions
	p.auditions = nil
	watch := p.watch
	p.mu.Unlock()
	p.watched = nil
	for _, t := range p.tracks {
		if t.name == watch {
			p.watched = t
		}
	}
	if len(as) > 0 {
		p.playAuditions(as)
	}
	if p.kp != nil {
		ck := p.c.keypad
		beat := int64(p.c.fpb / float64(p.c.beats))
		for _, d := range keys {
			pitch := ck.notes[d]
			p.kp.vs.play(p.kp.c.patch, note{pitch: float32(pitch), vel: 0.9, gate: beat / 2, res: -1})
			p.publish([]Note{{Frame: p.at, Len: beat / 2, Track: "keypad", Pitch: pitch, Vel: 0.9}})
		}
	}
	if sting != "" && p.c.stings[sting] != nil && !p.stopped {
		// The bar ends at the next beat, and the sting starts there.
		beat := p.c.fpb / float64(p.c.beats)
		into := float64(p.at - p.barStart)
		next := p.barStart + int64(math.Ceil(into/beat-1e-9)*beat)
		if next < p.at {
			next = p.at
		}
		p.barEnd = min(p.barEnd, next)
		for i := p.qi; i < len(p.queue); i++ {
			if p.queue[i].frame >= p.barEnd {
				p.queue = p.queue[:i]
				break
			}
		}
		p.stingNow = sting
	}
}

// adopt plays c from now on, keeping what plays.
func (p *Player) adopt(c *compiled) {
	old := map[string]*track{}
	for _, t := range p.tracks {
		old[t.name] = t
	}
	p.tracks = p.tracks[:0:0]
	for i, ct := range c.tracks {
		t, ok := old[ct.t.Name]
		if ok {
			delete(old, ct.t.Name)
			t.retune(ct)
		} else {
			t = newTrack(ct, p.seed+uint64(i)*104729)
		}
		p.tracks = append(p.tracks, t)
	}
	for _, t := range old {
		t.release()
		p.ghosts = append(p.ghosts, t)
	}
	kp := p.kp
	p.kp = nil
	if c.keypad != nil {
		if kp != nil && kp.c.patch.kind == c.keypad.t.patch.kind {
			kp.retune(c.keypad.t)
			p.kp = kp
		} else {
			p.kp = newTrack(c.keypad.t, p.seed+77)
		}
	}
	if c.trans != nil {
		ct := &ctrack{t: &Track{Name: "transitions", Reverb: float64(c.trans.reverb)}, patch: c.trans.patch, gain: c.trans.gain}
		if p.trans == nil {
			p.trans = newTrack(ct, p.seed+99)
		} else {
			p.trans.retune(ct)
		}
	}
	p.solo = false
	for _, ct := range c.tracks {
		p.solo = p.solo || ct.t.Solo
	}
	if p.audit != nil {
		// Notes auditioned sound on as their patch is edited.
		if pt, ok := c.patches[p.auditPatch]; ok && pt.kind == p.audit.c.patch.kind {
			p.audit.retune(&ctrack{t: p.audit.c.t, patch: pt, gain: 1})
		} else {
			p.audit.release()
		}
	}
	p.waiting.Store(true)
	p.duckT = nil
	if c.duck >= 0 {
		p.duckT = p.tracks[c.duck]
	}
	m := c.s.Mix
	rv := m.Reverb
	if rv.Size == 0 {
		rv.Size = 0.8
	}
	if rv.Decay == 0 {
		rv.Decay = 2.2
	}
	if rv.Tone == 0 {
		rv.Tone = 0.5
	}
	if rv.PreDelay == 0 {
		rv.PreDelay = 0.02
	}
	p.rev.set(rv.Size, rv.Decay, rv.Tone, rv.PreDelay)
	ec := m.Delay
	if ec.Beats == 0 {
		ec.Beats = 0.75
	}
	if ec.Feedback == 0 {
		ec.Feedback = 0.35
	}
	if ec.Tone == 0 {
		ec.Tone = 0.4
	}
	p.dly.set(ec.Beats, c.bpm, ec.Feedback, ec.Tone)
	th, ratio := m.Threshold, m.Ratio
	if th == 0 {
		th = -10
	}
	if ratio == 0 {
		ratio = 2
	}
	p.comp.threshold, p.comp.ratio = float32(th), float32(ratio)
	p.gain = float32(dbGain(m.Gain))
	p.ctx.beatHz = c.bpm / 60
	p.tiersSeen.Store(int32(max(c.tiers, 1)))
	if t := int(p.tierReq.Load()); t > max(c.tiers, 1) {
		p.tierReq.Store(int32(max(c.tiers, 1)))
	}
	first := p.c == nil
	p.c = c
	if !first {
		// The bar playing ends where the new tempo says.
		sb := p.bar - p.origin
		end := p.barStart + int64(math.Round(float64(sb+1)*c.fpb)) - int64(math.Round(float64(sb)*c.fpb))
		if end > p.at {
			p.barEnd = end
		}
		// Tracks added while the song plays come in at the next phrase.
	}
	p.gather()
	p.wmu.Lock()
	p.levels = slices.Clone(p.tracks)
	p.wmu.Unlock()
	p.publishLook()
}

// gather lists the tracks to make sound from.
func (p *Player) gather() {
	p.all = append(p.all[:0], p.tracks...)
	if p.kp != nil {
		p.all = append(p.all, p.kp)
	}
	if p.trans != nil {
		p.all = append(p.all, p.trans)
	}
	if p.sting != nil {
		p.all = append(p.all, p.sting.tracks...)
	}
	if p.audit != nil {
		p.all = append(p.all, p.audit)
	}
	p.all = append(p.all, p.ghosts...)
}

func newTrack(ct *ctrack, seed uint64) *track {
	t := &track{name: ct.t.Name, rng: newRand(seed), arpChord: -1}
	t.retune(ct)
	return t
}

// retune plays ct from now on: its notes sounding take its patch, so a
// patch's change is heard at once.
func (t *track) retune(ct *ctrack) {
	t.c = ct
	p := ct.patch
	if p.kind == kindDrums {
		t.vs = nil
		if t.hs == nil {
			t.hs = newHits(p.poly)
		}
	} else {
		t.hs = nil
		if t.vs == nil || len(t.vs.vs) < p.poly {
			t.vs = newVoices(p.poly, t.rng.next())
		}
		for _, v := range t.vs.vs {
			if v.on && v.p != nil && v.p.kind == p.kind {
				v.p = p
				v.amp.set(p.src.Amp)
				v.fenv.set(p.src.FilterEnv)
				if len(v.lfoP) != len(p.lfo) {
					v.lfoP = make([]float32, len(p.lfo))
					v.lfoR = make([]float32, len(p.lfo))
				}
			} else if v.on {
				v.on = false
			}
		}
	}
	t.ins.set(ct.t)
	if len(t.pcycle) != len(ct.params) {
		t.pcycle = make([]int, len(ct.params))
		t.pevs = make([][]pev, len(ct.params))
	}
	for i := range t.pcycle {
		t.pcycle[i] = math.MinInt
	}
}

func (t *track) release() {
	if t.vs != nil {
		t.vs.releaseAll()
	}
}

// startBar starts the next bar: it starts or ends a sting, chooses the
// parts where a phrase starts, marks a tier's change, and lays out the
// bar's notes.
func (p *Player) startBar() {
	p.bar++
	p.barStart = p.at
	c := p.c
	if p.stingNow != "" {
		p.startSting(p.stingNow)
		p.stingNow = ""
	} else if st := p.sting; st != nil && p.bar-st.start >= st.cs.s.Bars {
		p.endSting()
	}
	sb := p.bar - p.origin
	p.barEnd = p.barStart + int64(math.Round(float64(sb+1)*c.fpb)) - int64(math.Round(float64(sb)*c.fpb))
	if p.sting == nil && !p.stopped && mod(sb, c.phraseBars) == 0 {
		p.phrase(sb)
	}
	p.lift(sb)
	p.schedule(sb)
	p.publishBar(sb)
}

// phrase chooses the parts that play in the phrase starting at song bar
// sb, and marks the tier's change.
func (p *Player) phrase(sb int) {
	c := p.c
	prev := p.tier
	p.tier = min(max(int(p.tierReq.Load()), 1), max(c.tiers, 1))
	p.risen = false
	p.mu.Lock()
	ctl := make([]band.PartControl, len(p.tracks))
	for i, t := range p.tracks {
		ctl[i] = p.ctl[t.name]
	}
	p.mu.Unlock()
	var on []bool
	if c.wander {
		on = p.wander(sb)
	} else {
		on = make([]bool, len(p.tracks))
		for i, t := range p.tracks {
			on[i] = t.c.t.Tier <= p.tier
		}
	}
	for i, t := range p.tracks {
		switch ctl[i] {
		case band.PartOn:
			on[i] = true
		case band.PartOff:
			on[i] = false
		case band.PartAuto:
		}
		t.leaving = t.active && !on[i]
		if on[i] && !t.active {
			t.since = 0
		}
		t.active = on[i]
		if t.active {
			t.since++
		}
	}
	if c.trans != nil && !c.wander && sb > 0 {
		tr := c.trans
		switch {
		case p.tier > prev:
			p.hit(tr.boom, p.barStart, 0, 0.9)
			p.hit(tr.crash, p.barStart, 0, 0.7)
		case p.tier < prev:
			p.hit(tr.down, p.barStart, int64(c.fpb), 0.8)
		}
	}
}

// wander chooses which tracks play in a wander song's phrase: the core
// ones always, and of the rest a number that wanders up and down,
// keeping most of those that played.
func (p *Player) wander(sb int) []bool {
	on := make([]bool, len(p.tracks))
	var opt []int
	for i, t := range p.tracks {
		if t.c.t.Core {
			on[i] = true
		} else {
			opt = append(opt, i)
		}
	}
	if len(opt) == 0 {
		return on
	}
	if sb == 0 {
		p.wanderN = max(1, len(opt)/3)
	} else {
		p.wanderN += p.rng.intn(3) - 1
		if p.wanderN < len(opt)/3 {
			p.wanderN++
		}
	}
	p.wanderN = min(max(p.wanderN, 1), len(opt))
	var in, out []int
	for _, i := range opt {
		if p.tracks[i].active {
			in = append(in, i)
		} else {
			out = append(out, i)
		}
	}
	// Now and then one part makes way for another.
	if len(in) > 0 && len(out) > 0 && p.rng.float() < 0.4 {
		j, k := p.rng.intn(len(in)), p.rng.intn(len(out))
		in[j], out[k] = out[k], in[j]
	}
	for len(in) > p.wanderN {
		j := p.rng.intn(len(in))
		out = append(out, in[j])
		in = append(in[:j], in[j+1:]...)
	}
	for len(in) < p.wanderN && len(out) > 0 {
		k := p.rng.intn(len(out))
		in = append(in, out[k])
		out = append(out[:k], out[k+1:]...)
	}
	for _, i := range in {
		on[i] = true
	}
	return on
}

// lift starts a riser in the bars before a phrase whose tier climbs.
func (p *Player) lift(sb int) {
	c := p.c
	if c.trans == nil || c.wander || p.sting != nil || p.stopped || p.risen {
		return
	}
	if mod(sb, c.phraseBars) == c.phraseBars-c.trans.lift && int(p.tierReq.Load()) > p.tier {
		p.risen = true
		p.hit(c.trans.riser, p.barStart, int64(float64(c.trans.lift)*c.fpb), 0.85)
	}
}

// liftLate starts a shorter riser where the tier climbs within the bars
// a riser would take.
func (p *Player) liftLate() {
	c := p.c
	if c.trans == nil || c.wander || p.sting != nil || p.stopped || p.risen || int(p.tierReq.Load()) <= p.tier {
		return
	}
	sb := p.bar - p.origin
	if mod(sb, c.phraseBars) < c.phraseBars-c.trans.lift {
		return
	}
	end := p.barStart + int64(float64(c.phraseBars-mod(sb, c.phraseBars))*c.fpb)
	if left := end - p.at; float64(left) > c.fpb/4 {
		p.risen = true
		p.trans.hs.play(c.trans.riser, 0.85, 0, 0, left, 0)
	}
}

// hit queues a transition's drum at frame.
func (p *Player) hit(d drum, frame, dur int64, vel float32) {
	if p.trans == nil {
		return
	}
	p.queue = append(p.queue, sched{frame: frame, t: p.trans, drum: true, d: d, dur: dur, n: note{vel: vel}})
}

func (p *Player) startSting(name string) {
	cs := p.c.stings[name]
	for _, t := range p.tracks {
		t.release()
		t.active, t.leaving = false, false
	}
	st := &stingRun{name: name, cs: cs, start: p.bar}
	for i, ct := range cs.tracks {
		st.tracks = append(st.tracks, newTrack(ct, p.seed+uint64(i)*31337+5))
	}
	if p.sting != nil {
		p.ghosts = append(p.ghosts, p.sting.tracks...)
	}
	p.sting = st
	p.gather()
}

func (p *Player) endSting() {
	st := p.sting
	p.ghosts = append(p.ghosts, st.tracks...)
	p.sting = nil
	if st.cs.resume {
		p.origin = p.bar
		p.tierReq.Store(1)
		p.tier = 1
		for _, t := range p.tracks {
			t.active = false
		}
	} else {
		p.stopped = true
	}
	p.gather()
}

// chordAt returns the chord at pos, in bars of the song or the sting
// playing.
func (p *Player) chordAt(pos float64) (ch Chord, index int) {
	if p.sting != nil {
		ch := p.sting.cs.chords
		i := mod(int(math.Floor(pos+1e-9)), len(ch))
		return ch[i], i
	}
	return p.c.chordAt(pos)
}

// songPos returns where song bar sb is, in bars of the song, or of the
// sting playing.
func (p *Player) songPos(sb int) float64 {
	if p.sting != nil {
		return float64(p.bar - p.sting.start)
	}
	return float64(sb)
}

// schedule lays out the notes of song bar sb, or of the sting's bar.
func (p *Player) schedule(sb int) {
	// Transition hits queued for this bar stay.
	keep := p.queue[:0]
	for _, s := range p.queue[p.qi:] {
		if s.frame >= p.barStart && s.t == p.trans {
			keep = append(keep, s)
		}
	}
	p.queue, p.qi = keep, 0
	switch {
	case p.sting != nil:
		bar := p.bar - p.sting.start
		for _, t := range p.sting.tracks {
			p.scheduleTrack(t, bar, p.sting.cs.key)
		}
	case !p.stopped:
		for _, t := range p.tracks {
			if t.active {
				p.scheduleTrack(t, sb, p.c.key)
			}
		}
	}
	slices.SortStableFunc(p.queue, func(a, b sched) int {
		switch {
		case a.frame < b.frame:
			return -1
		case a.frame > b.frame:
			return 1
		}
		return 0
	})
}

// scheduleTrack lays out t's notes in bar.
func (p *Player) scheduleTrack(t *track, bar int, k key) {
	ct := t.c
	c := p.c
	if ct.mel != nil {
		p.scheduleMelody(t, bar)
		return
	}
	bars := float64(ct.bars)
	cycle := floorDiv(bar, ct.bars)
	lo := float64(mod(bar, ct.bars)) / bars
	hi := lo + 1/bars
	p.evs = ct.pat.query(cycle, p.evs[:0])
	for _, e := range p.evs {
		if e.at < lo-1e-9 || e.at >= hi-1e-9 {
			continue
		}
		inBar := (e.at - lo) * bars
		durBars := e.dur * bars
		pos := float64(bar) + inBar
		vel, pan, cutoff, res, legato := float32(1), float32(0), float32(0), float32(-1), ct.legato
		var octave int
		var vowel byte
		var tune float32
		for i := range ct.params {
			v, ok := p.paramAt(t, i, cycle, e.at)
			if !ok {
				continue
			}
			switch ct.params[i].which {
			case parVel:
				vel = float32(v)
			case parPan:
				pan = float32(v)
			case parCutoff:
				cutoff = float32(v)
			case parRes:
				res = float32(v)
			case parLegato:
				legato = v
			case parOctave:
				octave = int(math.Round(v))
			case parVowel:
				vowel = byte(v)
			case parTune:
				tune = float32(v)
			}
		}
		frame := p.barStart + int64(inBar*c.fpb+0.5)
		if c.swing > 0 {
			s16 := inBar * 16
			if r := math.Round(s16); math.Abs(s16-r) < 1e-6 && int(r)%2 == 1 {
				frame += int64(c.swing * c.fpb / 16)
			}
		}
		if h := ct.t.Human; h > 0 {
			frame += int64(float64(t.rng.bipolar()) * h * 0.006 * rate)
			vel *= 1 + float32(h)*0.12*t.rng.bipolar()
			frame = max(frame, p.barStart)
		}
		gate := max(int64(durBars*c.fpb*legato), 64)
		act := ct.acts[e.atom]
		chord, ci := p.chordAt(pos)
		oct := ct.oct + act.oct + octave
		n := note{vel: vel, gate: gate, pan: pan, cutoff: cutoff, res: res, vowel: vowel}
		switch act.kind {
		case actDegree:
			p.queueNote(t, frame, n, k.degree(act.n, oct)+act.acc)
		case actTone:
			p.queueNote(t, frame, n, chord.tone(act.n, oct))
		case actBass:
			p.queueNote(t, frame, n, chord.Bass+12*(oct+1))
		case actNote:
			p.queueNote(t, frame, n, act.n+12*(act.oct+octave))
		case actChord:
			v := chord.voice(12*(oct+1)+6, t.voicing)
			t.voicing = append(t.voicing[:0], v...)
			for _, pitch := range v {
				p.queueNote(t, frame, n, pitch)
			}
		case actArp:
			p.queueNote(t, frame, n, p.arp(t, chord, ci, oct))
		case actDrum:
			var pitch float32
			if act.tone >= 0 {
				pitch = float32(chord.tone(act.tone, oct-2))
			} else if act.drum.kind == drTimpani {
				pitch = float32(chord.Root + 12*(oct-1))
			}
			p.queue = append(p.queue, sched{frame: frame, t: t, drum: true, d: act.drum, n: n, pitch: pitch,
				dur: int64(durBars * c.fpb), tune: tune})
		}
	}
}

func (p *Player) queueNote(t *track, frame int64, n note, pitch int) {
	n.pitch = float32(pitch)
	p.queue = append(p.queue, sched{frame: frame, t: t, n: n})
}

// arp returns the next note of t's arpeggio over chord, the ci'th of
// its progression.
func (p *Player) arp(t *track, chord Chord, ci, oct int) int {
	if ci != t.arpChord {
		t.arpChord, t.arpStep = ci, 0
	}
	ct := t.c
	t.arpBuf = t.arpBuf[:0]
	for i := range len(chord.Tones) * ct.arpOct {
		t.arpBuf = append(t.arpBuf, chord.tone(i, oct))
	}
	n := len(t.arpBuf)
	s := t.arpStep
	t.arpStep++
	i := 0
	switch ct.arp {
	case arpUp:
		i = s % n
	case arpDown:
		i = n - 1 - s%n
	case arpUpDown, arpDownUp:
		if n == 1 {
			break
		}
		k := s % (2*n - 2)
		i = k
		if k >= n {
			i = 2*n - 2 - k
		}
		if ct.arp == arpDownUp {
			i = n - 1 - i
		}
	case arpConverge:
		k := s % n
		i = k / 2
		if k%2 == 1 {
			i = n - 1 - k/2
		}
	case arpRandom:
		i = t.rng.intn(n)
	}
	return t.arpBuf[i]
}

// scheduleMelody lays out a written melody's notes in bar.
func (p *Player) scheduleMelody(t *track, bar int) {
	c := p.c
	ct := t.c
	mel := ct.mel[c.melodyVariant(ct, bar)]
	for _, m := range mel[mod(bar, len(mel))] {
		frame := p.barStart + int64(m.at*c.fpb+0.5)
		gate := max(int64(m.dur*c.fpb*ct.legato/0.9), 64)
		p.queueNote(t, frame, note{vel: m.vel, gate: gate, res: -1}, m.pitch)
	}
}

// paramAt returns the value of t's parameter i at at, in cycle, and
// false where its pattern rests there.
func (p *Player) paramAt(t *track, i, cycle int, at float64) (float64, bool) {
	cp := &t.c.params[i]
	if t.pcycle[i] != cycle {
		t.pevs[i] = cp.pat.query(cycle, t.pevs[i][:0])
		t.pcycle[i] = cycle
	}
	found := -1
	for j, e := range t.pevs[i] {
		if e.at <= at+1e-9 && at < e.at+e.dur-1e-9 {
			found = j
		}
	}
	if found < 0 {
		return 0, false
	}
	return cp.vals[t.pevs[i][found].atom].at(float64(cycle) + at), true
}

// fire starts s.
func (p *Player) fire(s *sched) {
	t := s.t
	if s.drum {
		if t.hs != nil {
			t.hs.play(s.d, s.n.vel, s.n.pan, s.pitch, s.dur, s.tune)
		}
	} else if t.vs != nil {
		t.vs.play(t.c.patch, s.n)
	}
	if t == p.duckT {
		p.duckOn = true
	}
}

// render makes n frames into dst.
func (p *Player) render(dst []float32, n int) {
	ml, mr := p.ml[:n], p.mr[:n]
	rl, rr := p.rl[:n], p.rr[:n]
	dl, dr := p.dl[:n], p.dr[:n]
	clear(ml)
	clear(mr)
	clear(rl)
	clear(rr)
	clear(dl)
	clear(dr)
	duck := p.duckBuf[:n]
	beat := 60 / p.c.bpm
	att := 1 - float32(math.Exp(-1/(0.004*rate)))
	rel := float32(math.Exp(-1 / (0.28 * beat * rate)))
	for i := range duck {
		if p.duckOn {
			p.duckEnv += (1 - p.duckEnv) * att
			if p.duckEnv > 0.98 {
				p.duckOn = false
			}
		} else {
			p.duckEnv *= rel
		}
		duck[i] = p.duckEnv
	}
	ghosts := false
	for _, t := range p.all {
		tl, tr := p.tl[:n], p.tr[:n]
		clear(tl)
		clear(tr)
		var sounding bool
		if t.vs != nil {
			sounding = t.vs.render(tl, tr, &p.ctx)
		} else if t.hs != nil {
			sounding = t.hs.render(tl, tr)
		}
		t.sounding = sounding
		ct := t.c
		if !sounding || !p.audible(ct.t) {
			t.meter(nil, nil)
			if t == p.watched {
				p.tap(nil, nil, n)
			}
			continue
		}
		t.ins.process(tl, tr)
		gl, gr := min(1, 1-ct.pan), min(1, 1+ct.pan)
		gl *= ct.gain
		gr *= ct.gain
		rv, dv := float32(ct.t.Reverb), float32(ct.t.Delay)
		dk := float32(ct.t.Duck)
		for i := range tl {
			g := 1 - dk*duck[i]
			l, r := tl[i]*gl*g, tr[i]*gr*g
			tl[i], tr[i] = l, r
			ml[i] += l
			mr[i] += r
			rl[i] += l * rv
			rr[i] += r * rv
			dl[i] += l * dv
			dr[i] += r * dv
		}
		t.meter(tl, tr)
		if t == p.watched {
			p.tap(tl, tr, n)
		}
	}
	for _, g := range p.ghosts {
		ghosts = ghosts || g.sounding
	}
	if !ghosts && len(p.ghosts) > 0 {
		p.ghosts = p.ghosts[:0]
		p.gather()
	}
	p.dly.process(dl, dr)
	for i := range dl {
		// The echoes sound in the room too.
		rl[i] += dl[i] * 0.3
		rr[i] += dr[i] * 0.3
	}
	p.rev.process(rl, rr)
	for i := range ml {
		ml[i] = (ml[i] + rl[i] + dl[i]) * p.gain
		mr[i] = (mr[i] + rr[i] + dr[i]) * p.gain
	}
	for i := range ml {
		_, _, ml[i] = p.low[0].step(ml[i])
		_, _, mr[i] = p.low[1].step(mr[i])
	}
	p.comp.process(ml, mr)
	p.reduction.Store(math.Float32bits(p.comp.reduction))
	p.lim.process(ml, mr)
	for i := range ml {
		dst[2*i] = ml[i]
		dst[2*i+1] = mr[i]
	}
}

// publishBar records what song bar sb plays, for Look, Watch and Notes.
func (p *Player) publishBar(sb int) {
	c := p.c
	b := barInfo{frame: p.barStart}
	b.look = p.lookNow(sb)
	// The chords that start within the bar.
	base, step := p.songPos(sb), c.chordBars
	if p.sting != nil {
		step = 1
	}
	for at := 0.0; at < 1-1e-9; {
		ch, i := p.chordAt(base + at)
		b.chords = append(b.chords, chordSpan{at: at, name: ch.Name, index: i})
		at = (math.Floor((base+at)/step+1e-9)+1)*step - base
	}
	notes := make([]Note, 0, len(p.queue))
	for _, s := range p.queue {
		nt := Note{Frame: s.frame, Len: s.n.gate, Track: s.t.name, Pitch: int(s.n.pitch), Vel: s.n.vel}
		if s.drum {
			nt.Len, nt.Pitch, nt.Drum = max(s.dur, rate/20), int(s.pitch), s.d.name
		}
		notes = append(notes, nt)
	}
	p.wmu.Lock()
	p.bars[p.nbars%barRing] = b
	p.nbars++
	p.look = b.look
	p.wmu.Unlock()
	p.publish(notes)
}

// publishLook records the song's look as it changes between bars.
func (p *Player) publishLook() {
	l := p.lookNow(max(p.bar-p.origin, 0))
	p.wmu.Lock()
	p.look = l
	p.wmu.Unlock()
}

// lookNow returns the song's look in song bar sb.
func (p *Player) lookNow(sb int) Look {
	c := p.c
	l := Look{Bar: sb, BarFrame: p.barStart, BarFrames: c.fpb, BeatsPerBar: c.beats, PhraseBars: c.phraseBars,
		Tier: p.tier, Tiers: c.tiers, Wander: c.wander, Stopped: p.stopped, Keypad: c.keypad != nil}
	if p.sting != nil {
		l.Sting = p.sting.name
		for _, ch := range p.sting.cs.chords {
			l.Chords = append(l.Chords, ch.Name)
			l.Spelled = append(l.Spelled, ch.Spelled())
		}
	} else {
		for _, ch := range c.prog(sb) {
			l.Chords = append(l.Chords, ch.Name)
			l.Spelled = append(l.Spelled, ch.Spelled())
		}
	}
	ch, i := p.chordAt(p.songPos(sb))
	l.Chord, l.ChordIndex = ch.Name, i
	l.Stings = sortedKeys(c.stings)
	p.mu.Lock()
	for _, t := range p.tracks {
		tl := TrackLook{Name: t.name, Color: t.c.t.Color, Tier: t.c.t.Tier, Core: t.c.t.Core, Drums: t.c.patch.kind == kindDrums,
			Playing: t.active, Control: p.ctl[t.name], Piece: band.Loop, Mute: t.c.t.Mute, Solo: t.c.t.Solo}
		if t.active && t.since <= 1 {
			tl.Piece = band.Intro
		}
		if t.leaving {
			tl.Playing, tl.Piece = true, band.Outro
		}
		l.Tracks = append(l.Tracks, tl)
	}
	p.mu.Unlock()
	return l
}

// publish records notes, for Notes.
func (p *Player) publish(notes []Note) {
	p.wmu.Lock()
	for _, n := range notes {
		p.notes[p.nnotes%noteRing] = n
		p.nnotes++
	}
	p.wmu.Unlock()
}

// Played returns how many frames the player has made.
func (p *Player) Played() int64 { return p.played.Load() }

// Notes appends to dst the notes heard at or after frame from, or to
// start later, those of the bar playing included, in the order they
// start.
func (p *Player) Notes(dst []Note, from int64) []Note {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	lo := max(0, p.nnotes-noteRing)
	for i := lo; i < p.nnotes; i++ {
		n := p.notes[i%noteRing]
		if n.Frame+n.Len >= from {
			dst = append(dst, n)
		}
	}
	slices.SortStableFunc(dst, func(a, b Note) int {
		switch {
		case a.Frame < b.Frame:
			return -1
		case a.Frame > b.Frame:
			return 1
		}
		return 0
	})
	return dst
}

// Look returns what the player plays at frame at, as heard, with the
// tracks' levels now.
func (p *Player) Look(at int64) Look {
	p.wmu.Lock()
	l := p.look
	var spans []chordSpan
	for i := p.nbars - 1; i >= max(0, p.nbars-barRing); i-- {
		b := p.bars[i%barRing]
		if b.frame <= at {
			l = b.look
			spans = b.chords
			break
		}
	}
	p.wmu.Unlock()
	l.Tracks = slices.Clone(l.Tracks)
	if len(spans) > 0 && l.BarFrames > 0 {
		pos := float64(at-l.BarFrame) / l.BarFrames
		for _, s := range spans {
			if s.at <= pos+1e-9 {
				l.Chord, l.ChordIndex = s.name, s.index
			}
		}
	}
	l.Frame = p.played.Load()
	for i := range l.Tracks {
		for _, t := range p.tracks0() {
			if t.name != l.Tracks[i].Name {
				continue
			}
			tl := &l.Tracks[i]
			for ch := range 2 {
				tl.Peak[ch] = math.Float32frombits(t.lv[ch].Load())
				tl.RMS[ch] = math.Float32frombits(t.lv[2+ch].Load())
			}
			tl.Level = max(tl.Peak[0], tl.Peak[1])
		}
	}
	return l
}

// tracks0 returns the tracks, for reading their levels from another
// goroutine: the slice is replaced, never changed, as songs change.
func (p *Player) tracks0() []*track {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	return p.levels
}

// Watch implements [band.Watcher].
func (p *Player) Watch() band.Status {
	l := p.Look(p.played.Load())
	st := band.Status{Bar: l.Bar, PhraseBars: max(l.PhraseBars, 1)}
	for _, t := range l.Tracks {
		st.Parts = append(st.Parts, band.PartStatus{Name: t.Name, Tier: t.Tier, Playing: t.Playing, Piece: t.Piece, Control: t.Control})
	}
	return st
}

// scopeFrames is how many of its last frames Scope keeps of a track.
const scopeFrames = 2048

// meter takes the frames a track made, after its fader, into its
// meters: its peaks rise at once and fall back, and its RMS eases over
// some 300 ms. Nil frames are silence.
func (t *track) meter(l, r []float32) {
	n := max(len(l), 1)
	fall := float32(math.Exp(-float64(n) / (0.6 * rate)))
	ease := float32(1 - math.Exp(-float64(n)/(0.3*rate)))
	for ch, x := range [2][]float32{l, r} {
		var pk, ms float32
		for _, s := range x {
			pk = max(pk, abs32(s))
			ms += s * s
		}
		if len(x) > 0 {
			ms /= float32(len(x))
		} else {
			// Silence passes the time a block would.
			fall = float32(math.Exp(-float64(maxBlock) / (0.6 * rate)))
			ease = float32(1 - math.Exp(-float64(maxBlock)/(0.3*rate)))
		}
		t.pk[ch] = max(pk, t.pk[ch]*fall)
		t.ms[ch] += (ms - t.ms[ch]) * ease
		if t.pk[ch] < 1e-6 {
			t.pk[ch] = 0
		}
		if t.ms[ch] < 1e-12 {
			t.ms[ch] = 0
		}
		t.lv[ch].Store(math.Float32bits(t.pk[ch]))
		t.lv[2+ch].Store(math.Float32bits(float32(math.Sqrt(float64(t.ms[ch])))))
	}
}

// audible reports whether t is heard: not muted, and soloed where any
// track is.
func (p *Player) audible(t *Track) bool {
	return !t.Mute && (!p.solo || t.Solo)
}

// tap keeps the watched track's frames, mono, for Scope.
func (p *Player) tap(l, r []float32, n int) {
	p.wmu.Lock()
	for i := range n {
		var s float32
		if l != nil {
			s = (l[i] + r[i]) / 2
		}
		p.scope[p.scopeAt] = s
		p.scopeAt = (p.scopeAt + 1) % scopeFrames
	}
	p.wmu.Unlock()
}

// WatchTrack sets the track whose sound Scope keeps, by name, "" for none.
func (p *Player) WatchTrack(name string) {
	p.mu.Lock()
	p.watch = name
	p.mu.Unlock()
	p.waiting.Store(true)
}

// Scope fills dst with the last frames the watched track made, mono,
// oldest first, as an oscilloscope shows them: at most 2048.
func (p *Player) Scope(dst []float32) []float32 {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	n := min(len(dst), scopeFrames)
	for i := range n {
		dst[i] = p.scope[(p.scopeAt-n+i+scopeFrames)%scopeFrames]
	}
	return dst[:n]
}

// Reduction returns how far the mix's compressor turns it down now, in
// decibels.
func (p *Player) Reduction() float32 { return math.Float32frombits(p.reduction.Load()) }

// audition is a note asked for with Audition.
type audition struct {
	patch string
	drum  string
	pitch int
	vel   float32
	secs  float64
}

// Audition plays a note of the song's patch named patch at once, held
// for secs seconds, over whatever plays, as a tool tries a patch out.
func (p *Player) Audition(patch string, pitch int, vel float32, secs float64) {
	p.mu.Lock()
	p.auditions = append(p.auditions, audition{patch: patch, pitch: pitch, vel: vel, secs: secs})
	p.mu.Unlock()
	p.waiting.Store(true)
}

// AuditionDrum plays the drum named drum of the song's drums patch named
// patch at once.
func (p *Player) AuditionDrum(patch, drum string, vel float32) {
	p.mu.Lock()
	p.auditions = append(p.auditions, audition{patch: patch, drum: drum, vel: vel, secs: 1})
	p.mu.Unlock()
	p.waiting.Store(true)
}

// playAuditions plays the notes asked for, on a track of their own.
func (p *Player) playAuditions(as []audition) {
	for _, a := range as {
		pt, ok := p.c.patches[a.patch]
		if !ok {
			continue
		}
		p.auditPatch = a.patch
		ct := &ctrack{t: &Track{Name: "audition", Reverb: 0.15}, patch: pt, gain: 1}
		if p.audit == nil {
			p.audit = newTrack(ct, p.seed+4242)
			p.gather()
		} else {
			p.audit.retune(ct)
		}
		if pt.kind == kindDrums {
			d, ok := pt.kit[a.drum]
			if !ok {
				continue
			}
			var pitch float32
			if d.kind == drTimpani {
				ch, _ := p.chordAt(p.songPos(p.bar - p.origin))
				pitch = float32(ch.Root + 36)
			}
			p.audit.hs.play(d, a.vel, 0, pitch, int64(p.c.fpb), 0)
			continue
		}
		p.audit.vs.play(pt, note{pitch: float32(a.pitch), vel: a.vel, gate: int64(a.secs * rate), res: -1})
	}
}
