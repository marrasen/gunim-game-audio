package synth

import "slices"

// The engine's effects, as a song's mix and its tracks run them, for a
// host to run on sound of its own, at its own rate: a plugin's, as
// package plugfx makes them. At [Rate] each sounds as it does in a song,
// frame for frame. Their methods run on one goroutine at a time, as a
// plugin's processor runs on the audio thread; past their New, they
// run lock-free and allocation-free.

// ReverbUnit is the reverb a song's tracks send to: a feedback delay network
// of eight lines after a pre-delay and four allpass diffusers, which
// sounds a room to a hall by its size and decay. It takes the middle of
// the sound sent to it, and gives its sound alone, in stereo.
type ReverbUnit struct{ r *reverb }

// NewReverbUnit returns a reverb for sound at sampleRate frames a second,
// at size 0.8, a decay of 2.2 s, a tone of 0.5 and a pre-delay of 20 ms.
func NewReverbUnit(sampleRate float64) *ReverbUnit { return &ReverbUnit{newReverbAt(sampleRate)} }

// Set shapes the reverb: size from 0.3 to 1.5, decay in seconds to fall
// 60 dB, tone from 0, dark, to 1, bright, and the pre-delay in seconds,
// up to a quarter of a second. A song's Mix.Reverb sets it so.
func (r *ReverbUnit) Set(size, decay, tone, preDelay float64) {
	r.r.set(min(max(size, 0.3), 1.5), max(decay, 0.1), min(max(tone, 0), 1), max(preDelay, 0))
}

// Process takes the sound sent to the reverb from l and r, and leaves
// the reverb's sound in them.
func (r *ReverbUnit) Process(l, rr []float32) { r.r.process(l, rr) }

// Clear silences the reverb, its tail and all.
func (r *ReverbUnit) Clear() { r.r.clear() }

// DelayUnit is the delay a song's tracks send to: in ping-pong, the sound
// goes in on the left, its echo crosses to the right and comes back to
// the left through the feedback, each time round darker and thinner, as
// its filters cut its highs and its lows. Made stereo, each side echoes
// on its own side. It gives its echoes alone.
type DelayUnit struct{ d *delay }

// NewDelayUnit returns a ping-pong delay for sound at sampleRate frames a
// second, its echoes up to longest seconds apart. A change of its time
// crossfades to the new one over fade seconds, where a song's jumps.
func NewDelayUnit(sampleRate, longest, fade float64) *DelayUnit {
	d := newDelayAt(sampleRate, longest)
	d.fadeLen = int(fade * sampleRate)
	return &DelayUnit{d}
}

// SetBeats sets the delay as a song's Mix.Delay does: echoes beats
// apart at bpm, feedback from 0 to 0.95 as loud as the echo before, and
// its tone from 0, dark, to 1, bright, which sets the highs' cut from
// 1.2 to 9.2 kHz.
func (d *DelayUnit) SetBeats(beats, bpm, feedback, tone float64) { d.d.set(beats, bpm, feedback, tone) }

// SetTime sets the echoes seconds apart, within the longest.
func (d *DelayUnit) SetTime(seconds float64) { d.d.setFrames(int(seconds * d.d.sr)) }

// SetFeedback sets how loud each time round is, of the one before, from
// 0 to 0.95: in ping-pong a time round is two echoes, left and right.
func (d *DelayUnit) SetFeedback(f float64) { d.d.feedback = float32(min(max(f, 0), 0.95)) }

// SetFilters sets where the feedback's filters cut, in hertz: the lows
// under lowHz, and the highs over highHz.
func (d *DelayUnit) SetFilters(lowHz, highHz float64) {
	d.d.low.setAt(float32(lowHz), d.d.sr)
	d.d.tone.setAt(float32(highHz), d.d.sr)
	d.d.lowL.a, d.d.toneL.a = d.d.low.a, d.d.tone.a
}

// SetPingPong sets the echoes crossing from side to side, or, off, each
// on its own side.
func (d *DelayUnit) SetPingPong(on bool) { d.d.stereo = !on }

// Frames returns how many frames apart the echoes are.
func (d *DelayUnit) Frames() int { return d.d.frames }

// Process takes the sound sent to the delay from l and r, and leaves the
// echoes in them.
func (d *DelayUnit) Process(l, r []float32) { d.d.process(l, r) }

// Clear silences the echoes.
func (d *DelayUnit) Clear() { d.d.clear() }

// ChorusKinds are the kinds of chorus, by the names a song's track's
// ChorusType gives them:
//
//	soft      two copies, one each side, 9 ms late, moving at 0.45 Hz
//	juno1     a Juno-60's chorus I: a bucket brigade's 1.7 to 5.4 ms,
//	          swept by a triangle at 0.5 Hz, the right side against the
//	          left
//	juno2     its chorus II, the same sweep at 0.86 Hz
//	juno12    both buttons down: a fast, shallow 9.75 Hz vibrato
//	ensemble  a string machine's ensemble: three copies, each swept by a
//	          slow and a fast sine a third of a cycle apart
var ChorusKinds = []string{"soft", "juno1", "juno2", "juno12", "ensemble"}

// ChorusUnit is the chorus of a song's track: it thickens a sound with
// copies of it, each a few milliseconds late, by a slowly moving amount.
type ChorusUnit struct{ c *chorus }

// NewChorusUnit returns a chorus of kind, one of [ChorusKinds], for sound at
// sampleRate frames a second, at a mix of 0.5.
func NewChorusUnit(sampleRate float64, kind string) *ChorusUnit {
	c := newChorusAt(chorusKinds[kind], sampleRate)
	c.mix = 0.5
	return &ChorusUnit{c}
}

// Set sets the chorus's kind, one of [ChorusKinds], and its mix, from 0
// to 1, as a track's ChorusType and Chorus do. A new kind starts its
// sweep afresh.
func (c *ChorusUnit) Set(kind string, mix float64) {
	c.c.setKind(chorusKinds[kind])
	c.c.mix = float32(min(max(mix, 0), 1))
}

// Process runs the sound in l and r through the chorus.
func (c *ChorusUnit) Process(l, r []float32) { c.c.process(l, r) }

// Clear silences what the chorus holds.
func (c *ChorusUnit) Clear() {
	clear(c.c.buf[0])
	clear(c.c.buf[1])
	c.c.dark[0].z, c.c.dark[1].z = 0, 0
}

// CopyFrom makes c sound on as o does, from where o is: its kind, its
// mix, its sweep and the sound it holds. Both run at one rate.
func (c *ChorusUnit) CopyFrom(o *ChorusUnit) {
	bufs := c.c.buf
	*c.c = *o.c
	for ch := range bufs {
		copy(bufs[ch], o.c.buf[ch])
	}
	c.c.buf = bufs
}

// PhaserUnit sweeps notches through a sound: six allpass stages a side,
// their corner swept by a sine round 800 Hz, the right side's a quarter
// of a cycle on from the left's, with feedback round them.
type PhaserUnit struct{ p *phaser }

// NewPhaserUnit returns a phaser for sound at sampleRate frames a second,
// sweeping at 0.5 Hz, at a depth of 0.7, feedback of 0.4 and a mix of 1.
func NewPhaserUnit(sampleRate float64) *PhaserUnit { return &PhaserUnit{newPhaserAt(sampleRate)} }

// Set sets the sweep's rate, in hertz, its depth, from 0 to 1, up to
// 2.5 octaves either way, the feedback, from 0 to 0.95, and the mix,
// from 0 to 1, the deepest notches.
func (p *PhaserUnit) Set(hz, depth, feedback, mix float64) {
	p.p.hz = float32(min(max(hz, 0), 40))
	p.p.depth = float32(min(max(depth, 0), 1))
	p.p.feedback = float32(min(max(feedback, 0), 0.95))
	p.p.mix = float32(min(max(mix, 0), 1))
}

// Process runs the sound in l and r through the phaser.
func (p *PhaserUnit) Process(l, r []float32) { p.p.process(l, r) }

// Clear silences what the phaser holds.
func (p *PhaserUnit) Clear() { p.p.clear() }

// CopyFrom makes p sound on as o does, from where o is.
func (p *PhaserUnit) CopyFrom(o *PhaserUnit) { *p.p = *o.p }

// DistortKinds are the kinds of distortion, by the names a song's
// track's DistortType gives them: a fuzz's hard clip, a guitar
// amplifier's and its cabinet's, and a wavefolder's.
var DistortKinds = []string{"fuzz", "amp", "fold"}

// DistortionUnit is the distortion of a song's track.
type DistortionUnit struct{ d distortion }

// NewDistortionUnit returns a distortion for sound at sampleRate frames a
// second, a fuzz at 0.5.
func NewDistortionUnit(sampleRate float64) *DistortionUnit {
	d := &DistortionUnit{distortion{sr: sampleRate}}
	d.Set(0.5, "fuzz")
	return d
}

// Set sets how hard the distortion drives, from 0 to 1, and its kind,
// one of [DistortKinds], as a track's Distort and DistortType do. At 0
// it passes the sound by.
func (d *DistortionUnit) Set(amount float64, kind string) {
	if !slices.Contains(DistortKinds, kind) {
		kind = "fuzz"
	}
	d.d.set(amount, kind)
}

// Process runs the sound in l and r through the distortion.
func (d *DistortionUnit) Process(l, r []float32) {
	if d.d.on {
		d.d.process(l, r)
	}
}

// Clear silences what the distortion's filters hold.
func (d *DistortionUnit) Clear() {
	for ch := range 2 {
		d.d.tight[ch].z, d.d.dc[ch].z = 0, 0
		d.d.cabLP[ch][0].reset()
		d.d.cabLP[ch][1].reset()
		d.d.cabHP[ch].reset()
		d.d.mid[ch].reset()
	}
}

// CopyFrom makes d sound on as o does, from where o is.
func (d *DistortionUnit) CopyFrom(o *DistortionUnit) { d.d = o.d }

// ReverbEarly returns when each of the reverb's eight lines first
// sends back the sound, at size, in seconds after its pre-delay: the
// first reflections, which tell how big the room is.
func ReverbEarly(size float64) [8]float64 {
	var t [8]float64
	for i, ms := range lineMs {
		t[i] = ms * min(max(size, 0.3), 1.5) / 1000
	}
	return t
}
