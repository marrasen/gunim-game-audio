package synth

import (
	"math"

	"github.com/marrasen/gunim-game-audio/soundfont"
)

// sfLevel turns a bank's voices up to play about as loud as the
// engine's own instruments, measured on General MIDI files.
const sfLevel = 2.6

// sfVoice plays a region of a SoundFont bank: its sample, looped as the
// bank says, through its envelopes, its LFOs and its lowpass filter.
type sfVoice struct {
	on bool
	// r is the region it plays, reg, its own copy, so the voice keeps it
	// whatever becomes of the regions it was found among.
	r      *soundfont.Region
	reg    soundfont.Region
	frames []int16
	age    uint64
	// note is the note of an Instrument it plays, 0 for a song's.
	note uint64
	// key is the note, vel its velocity, 0 to 1, shift the semitones
	// a channel's bend wheel and vibrato move it, and bend those a
	// note's own bend does.
	key, vel, shift, bend float64
	// pos is where in the sample it plays, end where the sample ends,
	// and loopA and loopB its loop, where it loops.
	pos          float64
	end          int
	loopA, loopB int
	// t is how long it has played, in seconds; released when it was let
	// go, -1 while held; relFrom the level the volume envelope let go at.
	t, released, relFrom float64
	modRelFrom           float64
	// gate counts down the frames until it lets go, for a note of a known
	// length; 0 for one held until let go.
	gate int64
	// The filter, both sides, and the cutoff it was tuned for.
	fl, fr svf
	cut    float32
	// level is the volume envelope's level as it stood, for a release to
	// start from, and gain the voice's level the block before, so a
	// change ramps.
	level, gain float64
	panL, panR  float32
	// fine moves the note by a fraction of a semitone, as a song's
	// tuning does, and gainBy turns it up or down, as its patch's gain.
	fine, gainBy float64
}

// sfStep returns the sample's frames a frame of output moves through,
// for a key shifted by semis semitones more.
func (v *sfVoice) sfStep(semis float64) float64 {
	r := v.r
	cents := (v.key-float64(r.Root))*r.KeyTrack + r.Tune + (semis+v.bend)*100
	return float64(r.Sample.Rate) / rate * math.Pow(2, cents/1200)
}

// start plays region r for key at velocity vel, 0 to 127. Where wait is
// false, a sample another goroutine is decoding is not waited for: the
// voice stays silent.
func (v *sfVoice) start(r *soundfont.Region, key, vel int, age uint64, gate int64, wait bool) {
	var frames []int16
	if wait {
		frames = r.Sample.Frames()
	} else {
		frames = r.Sample.TryFrames()
	}
	*v = sfVoice{on: len(frames) > 1, reg: *r, frames: frames, age: age, key: float64(key), vel: float64(vel) / 127,
		released: -1, gate: gate, gainBy: 1}
	v.r = &v.reg
	r = v.r
	v.end = min(len(frames)+r.End, len(frames))
	if v.end <= 0 {
		v.end = len(frames)
	}
	v.pos = float64(min(max(r.Start, 0), max(v.end-1, 0)))
	v.loopA, v.loopB = min(max(r.LoopStart, 0), v.end), min(max(r.LoopEnd, 0), v.end)
	if r.Loop == 0 || v.loopB-v.loopA < 2 {
		v.loopA, v.loopB = 0, 0
	}
	pl, pr := panGains(float32(r.Pan))
	v.panL, v.panR = pl, pr
}

// release lets go of the note: its envelopes go to their release.
func (v *sfVoice) release() {
	if v.released < 0 {
		v.released = v.t
		v.relFrom = v.level
		v.modRelFrom = envAt(v.r.Mod, v.t, -1, 0, false)
	}
}

// choke stops the note at once, but for a few milliseconds' fade, as an
// exclusive class's next note does.
func (v *sfVoice) choke() {
	v.release()
	v.reg.Vol.Release = 0.005
}

// envAt returns envelope e's level at t seconds: rising through its
// attack, holding, falling through its decay to its sustain, and once
// let go at released, falling from from through its release. db says
// the fall is by decibels, as a volume envelope's is, 100 dB over its
// time; otherwise it is straight.
func envAt(e soundfont.Env, t, released, from float64, db bool) float64 {
	fall := func(start, span, d float64) float64 {
		x := d / max(span, 1e-4)
		if db {
			return start * math.Pow(10, -5*x)
		}
		return start * max(1-x, 0)
	}
	if released >= 0 && t >= released {
		return fall(from, e.Release, t-released)
	}
	switch {
	case t < e.Delay:
		return 0
	case t < e.Delay+e.Attack:
		return (t - e.Delay) / max(e.Attack, 1e-4)
	case t < e.Delay+e.Attack+e.Hold:
		return 1
	}
	d := t - e.Delay - e.Attack - e.Hold
	return max(fall(1, e.Decay, d), e.Sustain)
}

// render adds n frames of the voice to l and r.
func (v *sfVoice) render(l, r []float32) {
	reg := v.r
	fr := v.frames
	n := len(l)
	for at := 0; at < n && v.on; at += control {
		m := min(control, n-at)
		// Modulation, once a control block.
		mod := envAt(reg.Mod, v.t, v.released, v.modRelFrom, false)
		vib := 0.0
		if reg.VibToPitch != 0 && v.t > reg.VibDelay {
			vib = math.Sin(2*math.Pi*reg.VibHz*(v.t-reg.VibDelay)) * reg.VibToPitch / 100
		}
		mlfo := 0.0
		if v.t > reg.ModDelay && (reg.ModLFOToPitch != 0 || reg.ModLFOToFilter != 0 || reg.ModLFOToVolume != 0) {
			mlfo = math.Sin(2 * math.Pi * reg.ModHz * (v.t - reg.ModDelay))
		}
		step := v.sfStep(v.shift + v.fine + vib + (mod*reg.ModToPitch+mlfo*reg.ModLFOToPitch)/100)
		level := envAt(reg.Vol, v.t, v.released, v.relFrom, true)
		v.level = level
		// The velocity's level is the region's, as its modulators make it.
		gain := reg.Gain * level * sfLevel * v.gainBy
		if reg.ModLFOToVolume != 0 {
			gain *= math.Pow(10, mlfo*reg.ModLFOToVolume/20)
		}
		// The filter, closing as the velocity falls where the region's
		// modulators say, as the SoundFont's default one has it.
		cutHz := reg.Cutoff * math.Pow(2, (mod*reg.ModToFilter+mlfo*reg.ModLFOToFilter)/1200)
		filtered := cutHz < 18000
		if filtered {
			c := float32(min(cutHz, 20000))
			if d := c - v.cut; d > v.cut*0.01 || d < -v.cut*0.01 {
				v.cut = c
				res := float32(min(reg.Q/24, 0.85))
				v.fl.set(c, res)
				v.fr.tuneAs(&v.fl)
			}
		}
		g0, g1 := float32(v.gain), float32(gain)
		v.gain = gain
		for i := range m {
			ip := int(v.pos)
			looping := v.loopB > 0 && (reg.Loop == 1 || v.released < 0)
			next := ip + 1
			switch {
			case looping && next >= v.loopB:
				// The frame after the loop's end is its start.
				next = v.loopA
			case ip >= v.end-1 || ip < 0:
				v.on = false
			}
			if !v.on {
				break
			}
			f := float32(v.pos - float64(ip))
			a, b := float32(fr[ip]), float32(fr[min(next, len(fr)-1)])
			s := (a + (b-a)*f) / 32768
			if filtered {
				s, _, _ = v.fl.step(s)
			}
			g := g0 + (g1-g0)*float32(i+1)/float32(m)
			l[at+i] += s * g * v.panL
			r[at+i] += s * g * v.panR
			v.pos += step
			// Round the loop while it loops: for ever, or until let go.
			for looping && v.pos >= float64(v.loopB) {
				v.pos -= float64(v.loopB - v.loopA)
			}
			if v.gate > 0 {
				if v.gate--; v.gate == 0 {
					v.release()
				}
			}
		}
		v.t += float64(m) / rate
		if v.released >= 0 && level < 1e-4 {
			v.on = false
		}
		if v.released < 0 && level < 1e-5 && v.t > reg.Vol.Delay+reg.Vol.Attack+reg.Vol.Hold+0.05 && reg.Vol.Sustain < 1e-4 {
			// Decayed to nothing while held, as a piano's note does.
			v.on = false
		}
	}
}

// sfVoices are a channel's SoundFont voices. live says they play on an
// audio thread, which may not wait for a sample decoded elsewhere.
type sfVoices struct {
	vs   []sfVoice
	age  uint64
	live bool
}

func newSFVoices(n int) *sfVoices { return &sfVoices{vs: make([]sfVoice, n)} }

// play starts region r of key, cutting off the notes of its exclusive
// class, and returns the voice.
func (vv *sfVoices) play(r *soundfont.Region, key, vel int, gate int64) *sfVoice {
	vv.age++
	if r.Exclusive != 0 {
		for i := range vv.vs {
			if v := &vv.vs[i]; v.on && v.r.Exclusive == r.Exclusive {
				v.choke()
			}
		}
	}
	best := 0
	for i := range vv.vs {
		v := &vv.vs[i]
		if !v.on {
			best = i
			break
		}
		b := &vv.vs[best]
		// A voice let go before one held, then the oldest.
		if (v.released >= 0) != (b.released >= 0) {
			if v.released >= 0 {
				best = i
			}
			continue
		}
		if v.age < b.age {
			best = i
		}
	}
	v := &vv.vs[best]
	v.start(r, key, vel, vv.age, gate, !vv.live)
	return v
}

// render adds the voices' sound to l and r, and reports whether any
// sounds.
func (vv *sfVoices) render(l, r []float32, shift float64) bool {
	sounding := false
	for i := range vv.vs {
		if v := &vv.vs[i]; v.on {
			v.shift = shift
			v.render(l, r)
			sounding = true
		}
	}
	return sounding
}

// releaseAll lets go of every voice.
func (vv *sfVoices) releaseAll() {
	for i := range vv.vs {
		if vv.vs[i].on {
			vv.vs[i].release()
		}
	}
}

// silence stops every voice at once.
func (vv *sfVoices) silence() {
	for i := range vv.vs {
		vv.vs[i].on = false
	}
}
