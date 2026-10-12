package synth

import (
	"math"
	"strconv"
)

// Rate is the sample rate the engine runs at, in frames a second.
const Rate = rate

// An Instrument plays a patch live, note by note, as a keyboard plays a
// synth: a note sounds from its NoteOn until its NoteOff, and then rings
// out through its release. Each note has an ID of its own while it
// sounds, which its NoteOff and its Bend name, and a group, its lane:
// each group has a sustain pedal, which holds only that group's notes.
//
// It plays synth, pluck and drums patches on the same voices a song's
// tracks play, so a patch sounds alike in both. A drums patch plays its
// kit by General MIDI's drum notes, as 36 for bd and 38 for sn; its
// notes ring out by themselves, so a NoteOff, a Bend and the pedal leave
// them be.
//
// Its methods run on one goroutine at a time, as a plugin's processor
// runs on the audio thread; none of them allocates, locks or waits.
type Instrument struct {
	// orig is the patch as given, src the patch as the knobs turn it,
	// and p it compiled, playing src.
	orig *Patch
	src  Patch
	p    *patch
	// gain is the patch's own level, before the knobs turn it.
	gain float32
	vs   *voices
	hs   *hits
	ctx  renderCtx
	// kit is a drums patch's drums, by note.
	kit [128]*drum
	// held are the notes sounding by their IDs, the oldest first, n of
	// them.
	held [maxLive]liveNote
	n    int
	// pedals are the groups' sustain pedals, as last moved.
	pedals [maxPedals]pedal
}

// The most notes an Instrument keeps by their IDs, and the most groups
// whose pedals it keeps. Past maxLive, a new note lets the oldest go.
const (
	maxLive   = 128
	maxPedals = 32
)

// liveNote is a note an Instrument plays, by its ID, on the voice that
// plays it, until its NoteOff, or its group's pedal's lift.
type liveNote struct {
	id, group int32
	v         *voice
	age       uint64
	// sustained says its NoteOff came while the pedal held it.
	sustained bool
}

// pedal is a group's sustain pedal.
type pedal struct {
	group int32
	used  bool
	down  bool
}

// Knobs turn an Instrument's patch as it plays, from how the patch is
// made: their zero value leaves it as it is. A drums patch takes only
// Gain.
type Knobs struct {
	// Cutoff moves the filter's cutoff, in octaves, and Res adds to its
	// resonance, from -1 to 1.
	Cutoff, Res float64
	// Attack and Release stretch the envelope's attack and release by
	// that many octaves: 1 doubles them.
	Attack, Release float64
	// Gain turns the sound up, in decibels.
	Gain float64
}

// NewInstrument returns an instrument that plays p. It plays a patch of
// a SoundFont bank on the engine's own instruments, as a song does where
// the bank is missing.
func NewInstrument(p *Patch) (*Instrument, error) {
	src := *p
	src.SoundFont = ""
	if src.Kind == "" && len(src.Osc) == 0 && src.Noise == 0 && p.SoundFont != "" {
		gm := GMPatch(p.Program & 127)
		if p.Gain != 0 {
			gm.Gain *= p.Gain
		}
		src = *gm
	}
	in := &Instrument{orig: &src, src: src}
	c, err := in.src.compile("instrument")
	if err != nil {
		return nil, err
	}
	in.p, in.gain = c, c.gain
	in.ctx = renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	if c.kind == kindDrums {
		in.hs = newHits(c.poly)
		for i := range in.hs.hs {
			in.hs.hs[i].noise = newRand(uint64(i)*31 + 7)
		}
		for key := range in.kit {
			if d, ok := c.kit[strconv.Itoa(key)]; ok {
				in.kit[key] = &d
			} else if d, ok := c.kit[gmDrumNames[key]]; ok && gmDrumNames[key] != "" {
				in.kit[key] = &d
			}
		}
		return in, nil
	}
	in.vs = newVoices(c.poly, 1)
	// Each voice gets what it plays with now, so a note takes nothing new
	// as it starts.
	for _, v := range in.vs.vs {
		v.lfoP = make([]float32, len(c.lfo))
		v.lfoR = make([]float32, len(c.lfo))
		if c.kind == kindPluck {
			v.str = make([]float32, pluckFrame)
		}
	}
	return in, nil
}

// gmDrumNames are the drums General MIDI's notes play, by the names a
// kit has them by.
var gmDrumNames = [128]string{
	35: "bd", 36: "bd", 37: "rim", 38: "sn", 39: "cp", 40: "sn",
	41: "lt", 42: "hh", 43: "lt", 44: "hh", 45: "mt", 46: "oh",
	47: "mt", 48: "ht", 49: "cr", 50: "ht", 51: "rd", 52: "cr",
	53: "rd", 54: "tamb", 55: "cr", 56: "cb", 57: "cr", 59: "rd",
	69: "sh", 70: "sh", 75: "rim", 82: "sh",
}

// NoteOn starts note id at pitch, a MIDI note number, fractional where
// it falls between two, at velocity vel, from 0 to 1, in group. An id
// still sounding is let go first.
func (in *Instrument) NoteOn(id int32, pitch, vel float64, group int32) {
	vel = min(max(vel, 0), 1)
	if in.p.kind == kindDrums {
		key := int(math.Round(pitch))
		if key < 0 || key > 127 || in.kit[key] == nil {
			return
		}
		in.hs.play(*in.kit[key], float32(vel), 0, float32(pitch), rate/2, 0)
		return
	}
	if i := in.find(id); i >= 0 {
		in.let(i)
	}
	if in.n == maxLive {
		in.let(0)
	}
	v := in.vs.play(in.p, note{pitch: float32(pitch), vel: float32(vel), res: -1})
	in.held[in.n] = liveNote{id: id, group: group, v: v, age: v.age}
	in.n++
}

// NoteOff lets note id go: it rings out through its release, or on
// while its group's pedal is down, until the pedal lifts.
func (in *Instrument) NoteOff(id int32) {
	i := in.find(id)
	if i < 0 {
		return
	}
	if in.pedalDown(in.held[i].group) {
		in.held[i].sustained = true
		return
	}
	in.let(i)
}

// Bend bends note id by semis semitones from its pitch, up, or down
// below 0, from now on.
func (in *Instrument) Bend(id int32, semis float64) {
	if i := in.find(id); i >= 0 {
		if h := &in.held[i]; h.v.on && h.v.age == h.age {
			h.v.bend = float32(semis)
		}
	}
}

// Pedal moves group's sustain pedal to value, from 0, up, to 1, down; it
// is down from a half. As it lifts, the notes it held are let go.
func (in *Instrument) Pedal(group int32, value float64) {
	down := value >= 0.5
	free := -1
	for i := range in.pedals {
		pd := &in.pedals[i]
		switch {
		case pd.used && pd.group == group:
			if pd.down && !down {
				in.lift(group)
			}
			pd.down = down
			return
		case free < 0 && (!pd.used || !pd.down):
			free = i
		}
	}
	if down && free >= 0 {
		in.pedals[free] = pedal{group: group, used: true, down: true}
	}
}

// pedalDown reports whether group's pedal is down.
func (in *Instrument) pedalDown(group int32) bool {
	for i := range in.pedals {
		if pd := &in.pedals[i]; pd.used && pd.group == group {
			return pd.down
		}
	}
	return false
}

// lift lets go of the notes of group the pedal held.
func (in *Instrument) lift(group int32) {
	for i := 0; i < in.n; {
		if h := &in.held[i]; h.group == group && h.sustained {
			in.let(i)
			continue
		}
		i++
	}
}

// find returns where note id is held, or -1.
func (in *Instrument) find(id int32) int {
	for i := range in.n {
		if in.held[i].id == id {
			return i
		}
	}
	return -1
}

// let lets go of the note held at i, unless its voice has been taken for
// another, and forgets it.
func (in *Instrument) let(i int) {
	if h := &in.held[i]; h.v.on && h.v.age == h.age {
		h.v.release()
	}
	copy(in.held[i:in.n], in.held[i+1:in.n])
	in.n--
}

// SetKnobs turns the patch by k, the notes sounding as well as those to
// come.
func (in *Instrument) SetKnobs(k Knobs) {
	o := in.orig
	cut := o.Filter.Cutoff
	if cut <= 0 {
		cut = 20000
	}
	in.src.Filter.Cutoff = min(max(cut*math.Exp2(k.Cutoff), 20), 20000)
	in.src.Filter.Res = min(max(o.Filter.Res+k.Res, 0), 0.98)
	in.src.Amp.Attack = o.Amp.Attack * math.Exp2(k.Attack)
	in.src.Amp.Release = o.Amp.Release * math.Exp2(k.Release)
	in.p.gain = in.gain * float32(math.Pow(10, k.Gain/20))
	if in.vs != nil {
		for _, v := range in.vs.vs {
			if v.on {
				v.amp.set(in.src.Amp)
			}
		}
	}
}

// Render fills l and r, as long as each other, with the instrument's
// sound.
func (in *Instrument) Render(l, r []float32) {
	clear(l)
	clear(r)
	if in.vs != nil {
		in.vs.render(l, r, &in.ctx)
	}
	if in.hs != nil {
		in.hs.render(l, r)
	}
}

// Sounding reports whether a note still sounds.
func (in *Instrument) Sounding() bool {
	if in.vs != nil {
		for _, v := range in.vs.vs {
			if v.on {
				return true
			}
		}
	}
	if in.hs != nil {
		for i := range in.hs.hs {
			if in.hs.hs[i].on {
				return true
			}
		}
	}
	return false
}

// Release lets go of every note, and lifts every pedal.
func (in *Instrument) Release() {
	for in.n > 0 {
		in.let(in.n - 1)
	}
	in.pedals = [maxPedals]pedal{}
}

// Silence stops every note at once, and lifts every pedal.
func (in *Instrument) Silence() {
	in.Release()
	if in.vs != nil {
		for _, v := range in.vs.vs {
			v.on = false
		}
	}
	if in.hs != nil {
		for i := range in.hs.hs {
			in.hs.hs[i].on = false
		}
	}
}
