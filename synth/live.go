package synth

import (
	"math"
	"strconv"

	"github.com/marrasen/gunim-game-audio/soundfont"
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
// tracks play, so a patch sounds alike in both, and a patch of a
// SoundFont bank on the bank's samples, where the bank is registered. A drums patch plays its
// kit by General MIDI's drum notes: a note plays the kit's drum named by
// its number, as GMKit's are, or else the first drum of its role the
// kit lists, as bd, bd9, sbd or nbd on 36, or else the built-in drum of
// that role, as bd. So the TR-909's bd9 and the Commodore 64's sbd play
// on 36, as a song's pattern plays them by name. Its notes ring out by
// themselves, so a NoteOff, a Bend and the pedal leave them be.
//
// Its methods run on one goroutine at a time, as a plugin's processor
// runs on the audio thread; none of them allocates, locks or waits.
type Instrument struct {
	// orig is the patch as given, src the patch as the knobs turn it,
	// and p it compiled, playing src.
	orig *Patch
	src  Patch
	p    *patch
	// level is the Gain knob's, as a factor, and k the knobs as last
	// set.
	level float32
	k     Knobs
	// vowels counts the notes struck with no turn of their own, for a
	// patch's Vowels to sing in turn.
	vowels int
	vs     *voices
	hs     *hits
	ctx    renderCtx
	// sf are a SoundFont patch's voices, and regions the regions a note
	// plays, kept from note to note; notes counts the notes struck, to
	// tell the voices of each apart.
	sf      *sfVoices
	regions []soundfont.Region
	notes   uint64
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
	// age is the voice's age, as it took the note; for a SoundFont
	// patch's note, the note its voices play, as they count it.
	age uint64
	// n is the note as struck, bend how far it is bent, and mod and
	// press its controllers, for a mono patch to go back to it.
	n                note
	bend, mod, press float32
	// turn is the note's turn, as NoteOnTurn took it.
	turn int64
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
// Gain. A SoundFont patch takes Gain, Pan and Tune, and the filter's
// Cutoff and the envelope's knobs on its notes to come.
type Knobs struct {
	// Cutoff moves the filter's cutoff, in octaves, and Res adds to its
	// resonance, from -1 to 1. Env adds to how many octaves the filter's
	// envelope opens it by.
	Cutoff, Res, Env float64
	// Attack, Decay and Release stretch the envelope's attack, decay
	// and release by that many octaves: 1 doubles them. Sustain adds to
	// its sustain, from -1 to 1.
	Attack, Decay, Sustain, Release float64
	// Gain turns the sound up, in decibels, and Pan moves it between
	// the speakers, from -1 to 1.
	Gain, Pan float64
	// Drive and Noise add to the patch's drive and noise, from -1 to 1.
	Drive, Noise float64
	// Tune moves every note by semitones; Width adds to a pulse's
	// width, from -0.5 to 0.5; and FM turns an fm wave's index up by
	// octaves, or down below 0.
	Tune, Width, FM float64
	// LFORate speeds the LFOs up by octaves, or slows them below 0, and
	// LFODepth adds to their depth, by a part of it: -1 stills them, 1
	// doubles them.
	LFORate, LFODepth float64
	// Vowel, where set, is the vowel every note sings, a, e, i, o or u,
	// in place of the patch's.
	Vowel byte
	// Mod and Pressure stand for a mod wheel and a key's pressure on
	// every note, from 0 to 1, on top of each note's own.
	Mod, Pressure float64
}

// NewInstrument returns an instrument that plays p. A patch of a
// SoundFont bank plays the bank's samples, where it is registered, and
// else, as a song does where the bank is missing, the engine's own
// instruments. A kit of the engine's drums named to play on a bank plays
// the engine's drums.
func NewInstrument(p *Patch) (*Instrument, error) {
	src := *p
	sampled := src.Kind == "" && len(src.Osc) == 0 && src.Noise == 0 && p.SoundFont != ""
	if !sampled || SoundFontNamed(p.SoundFont) == nil {
		src.SoundFont = ""
		if sampled {
			gm := GMPatch(p.Program & 127)
			if p.Bank == DrumBank || p.FontKit {
				gm = GMKit(p.Program & 127)
			}
			if p.Gain != 0 {
				gm.Gain *= p.Gain
			}
			src = *gm
		}
	}
	in := &Instrument{orig: &src, src: src, level: 1}
	c, err := in.src.compile("instrument")
	if err != nil {
		return nil, err
	}
	in.p = c
	mod, press := in.src.Responses()
	c.live = &liveCtl{lfoRate: 1, lfoDepth: 1, modR: mod, pressR: press}
	in.ctx = renderCtx{beatHz: 2, carry: true, l: make([]float32, control), r: make([]float32, control)}
	if c.kind == kindFont {
		// A note may play a region for each side, and a layer or two:
		// room for them all, and for the notes ringing out.
		in.sf = newSFVoices(2 * max(c.poly, 16))
		in.sf.live = true
		in.regions = make([]soundfont.Region, 0, 16)
		return in, nil
	}
	if c.kind == kindDrums {
		in.hs = newHits(c.poly)
		// Each hit's noise is seeded now, so a note takes nothing new as
		// it starts, as a song's track seeds its first hit's, by the
		// hit's count from 1: a drum's first hit sounds as its first in
		// a song does.
		for i := range in.hs.hs {
			in.hs.hs[i].noise = newRand(uint64(i+1)*31 + 7)
		}
		for key := range in.kit {
			if name := kitDrum(in.src.Kit, key); name != "" {
				d := c.kit[name]
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
// kit has them by: each note's role, as the kick on 36, in the names of
// every drum that plays it, the built-in drum's first. A kit of SID
// drums names its kick sbd, a TR-909's kit bd9, a Simmons kit its toms
// syn1, the highest, to syn3.
var gmDrumNames = [128][]string{
	35: kicks, 36: kicks, 37: {"rim"}, 38: snares, 39: claps, 40: snares,
	41: lowToms, 42: hats, 43: lowToms, 44: hats, 45: midToms, 46: openHats,
	47: midToms, 48: highToms, 49: crashes, 50: highToms, 51: rides, 52: crashes,
	53: {"rd", "ride", "mtl", "metal", "nclk"}, 54: {"tamb", "tambourine"}, 55: crashes,
	56: {"cb", "cowbell"}, 57: crashes, 59: rides,
	69: shakers, 70: shakers, 75: {"rim"}, 82: shakers,
}

// The names of the drums of each role, for gmDrumNames.
var (
	kicks    = []string{"bd", "kick", "bd9", "kick909", "sbd", "nbd", "mbd"}
	snares   = []string{"sn", "snare", "ssn", "nsn", "msn"}
	claps    = []string{"cp", "clap", "scp", "ncp"}
	hats     = []string{"hh", "hat", "shh", "nhh", "mhh"}
	openHats = []string{"oh", "ohat", "soh", "noh", "moh"}
	lowToms  = []string{"lt", "syn3", "stom", "ntom", "mlt"}
	midToms  = []string{"mt", "tom", "syn2", "stom", "ntom", "mmt"}
	highToms = []string{"ht", "syn1", "stom", "ntom", "mht"}
	crashes  = []string{"cr", "crash", "mcr"}
	rides    = []string{"rd", "ride", "mrd"}
	shakers  = []string{"sh", "shaker"}
)

// kitDrum returns the name of the drum General MIDI's note key plays in
// a kit that lists kit: the drum named by the note's number, or the first
// of the note's names the kit lists, or the first of them, built in; or
// "" for a note that plays none.
func kitDrum(kit map[string]Drum, key int) string {
	if n := strconv.Itoa(key); hasDrum(kit, n) {
		return n
	}
	names := gmDrumNames[key]
	for _, n := range names {
		if hasDrum(kit, n) {
			return n
		}
	}
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// hasDrum reports whether kit lists a drum named name.
func hasDrum(kit map[string]Drum, name string) bool {
	_, ok := kit[name]
	return ok
}

// NoteOn starts note id at pitch, a MIDI note number, fractional where
// it falls between two, at velocity vel, from 0 to 1, in group. An id
// still held is let go first. It counts the note's turn itself, as
// NoteOnTurn does for a turn of 0.
func (in *Instrument) NoteOn(id int32, pitch, vel float64, group int32) {
	in.NoteOnTurn(id, pitch, vel, group, 0)
}

// NoteOnTurn is NoteOn for a note whose turn the host counts: its place
// among the notes of its lane, from 1 at the start of the music, as
// [plug.Event]'s Turn. A patch's Vowels sing by it, the vowel at turn
// less 1, round, so a note sings the same vowel however playing
// reaches it. A turn of 0 counts the notes struck since the last
// Silence instead, as for notes played live.
func (in *Instrument) NoteOnTurn(id int32, pitch, vel float64, group int32, turn int64) {
	vel = min(max(vel, 0), 1)
	if i := in.find(id); i >= 0 {
		in.let(i)
	}
	if in.n == maxLive {
		in.let(0)
	}
	n := note{pitch: float32(pitch), vel: float32(vel), res: -1}
	if vs := in.p.vowels; len(vs) > 0 {
		i := in.vowels
		if turn > 0 {
			i = int((turn - 1) % int64(len(vs)))
		} else {
			in.vowels++
		}
		n.vowel = vs[i%len(vs)]
	}
	var v *voice
	age := uint64(0)
	switch in.p.kind {
	case kindFont:
		in.notes++
		age = in.notes
		in.playFont(age, pitch, vel)
	case kindDrums:
		// A drum rings out by itself; the note is kept, voiceless, for
		// Pass to hand on.
		if key := int(math.Round(pitch)); key >= 0 && key <= 127 && in.kit[key] != nil {
			h := in.hs.play(*in.kit[key], float32(vel), 0, float32(pitch), rate/2, 0)
			h.part = drumParts[key]
		}
	default:
		v = in.vs.play(in.p, n)
		age = v.age
	}
	in.held[in.n] = liveNote{id: id, group: group, v: v, age: age, n: n, turn: turn}
	in.n++
}

// SetOffline says the instrument renders offline, as for an export,
// where a block may take as long as it needs: a note of a SoundFont
// bank waits for its samples where another goroutine is decoding them,
// as it may not on an audio thread, where it plays without them.
func (in *Instrument) SetOffline(on bool) {
	if in.sf != nil {
		in.sf.live = !on
	}
}

// playFont plays a SoundFont patch's note at pitch and velocity vel,
// from 0 to 1, as note: each region the bank plays for it, through the
// knobs as they stand.
func (in *Instrument) playFont(note uint64, pitch, vel float64) {
	key := int(math.Round(pitch))
	if key < 0 || key > 127 {
		return
	}
	v127 := min(max(int(vel*127+0.5), 1), 127)
	src, k := in.orig, &in.k
	in.regions = in.p.font.AppendRegions(in.regions[:0], src.Bank, src.Program, key, v127)
	for i := range in.regions {
		v := in.sf.play(&in.regions[i], key, v127, 0)
		v.note = note
		v.fine = pitch - float64(key)
		v.gainBy = float64(in.p.gain)
		reg := &v.reg
		if k.Attack != 0 || k.Decay != 0 || k.Release != 0 {
			reg.Vol.Attack *= math.Exp2(k.Attack)
			reg.Vol.Decay *= math.Exp2(k.Decay)
			reg.Vol.Release *= math.Exp2(k.Release)
		}
		reg.Vol.Sustain = min(max(reg.Vol.Sustain+k.Sustain, 0), 1)
		if k.Cutoff != 0 {
			reg.Cutoff = min(max(reg.Cutoff*math.Exp2(k.Cutoff), 20), 20000)
		}
		l, r := panGains(min(max(float32(reg.Pan+k.Pan), -1), 1))
		v.panL, v.panR = l, r
	}
}

// fontVoices calls fn with each voice that plays note, of a SoundFont
// patch's.
func (in *Instrument) fontVoices(note uint64, fn func(v *sfVoice)) {
	if in.sf == nil || note == 0 {
		return
	}
	for i := range in.sf.vs {
		if v := &in.sf.vs[i]; v.on && v.note == note {
			fn(v)
		}
	}
}

// Fade moves note id's level, a factor on its sound from 1 as it starts,
// to level in a straight line over frames of the engine's frames, or at
// once for none: for a swell into a note, or a fade out of one. A drums
// patch's notes ring at their own level.
func (in *Instrument) Fade(id int32, level float64, frames int) {
	i := in.find(id)
	if i < 0 {
		return
	}
	if h := &in.held[i]; h.v != nil && h.v.on && h.v.age == h.age {
		h.v.fade(float32(level), frames)
	}
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
	if in.p.poly == 1 && in.back(i) {
		return
	}
	in.let(i)
}

// back hands a mono patch's voice, playing the note held at i, back to
// the last note struck before it that is still held, as a mono synth
// does, and forgets the note at i. It reports whether there was one.
func (in *Instrument) back(i int) bool {
	h := &in.held[i]
	if h.v == nil || !h.v.on || h.v.age != h.age {
		return false
	}
	j := -1
	for k := in.n - 1; k >= 0 && j < 0; k-- {
		if k != i && !in.held[k].sustained {
			j = k
		}
	}
	if j < 0 {
		return false
	}
	to := &in.held[j]
	v := in.vs.play(in.p, to.n)
	v.bend, v.mod, v.press = to.bend, to.mod, to.press
	to.v, to.age = v, v.age
	copy(in.held[i:in.n], in.held[i+1:in.n])
	in.n--
	return true
}

// Bend bends note id by semis semitones from its pitch, up, or down
// below 0, from now on: the note held, or else the last of that ID the
// pedal holds after its NoteOff.
func (in *Instrument) Bend(id int32, semis float64) {
	if h := in.control(id); h != nil {
		h.bend = float32(semis)
		if h.v != nil && h.v.on && h.v.age == h.age {
			h.v.bend = h.bend
		}
		in.fontVoices(h.age, func(v *sfVoice) { v.bend = semis })
	}
}

// Mod moves note id's mod wheel to value, from 0 to 1, from now on, as
// Bend finds the note; the patch answers it as its Mod says.
func (in *Instrument) Mod(id int32, value float64) {
	if h := in.control(id); h != nil {
		h.mod = float32(min(max(value, 0), 1))
		if h.v != nil && h.v.on && h.v.age == h.age {
			h.v.mod = h.mod
		}
	}
}

// Pressure moves note id's pressure, how hard its key is pressed, to
// value, from 0 to 1, from now on, as Bend finds the note; the patch
// answers it as its Pressure says.
func (in *Instrument) Pressure(id int32, value float64) {
	if h := in.control(id); h != nil {
		h.press = float32(min(max(value, 0), 1))
		if h.v != nil && h.v.on && h.v.age == h.age {
			h.v.press = h.press
		}
	}
}

// control returns note id, for its controllers to move: the note held,
// or else the last of that ID the pedal holds after its NoteOff; nil
// for none.
func (in *Instrument) control(id int32) *liveNote {
	i := in.find(id)
	for j := in.n - 1; i < 0 && j >= 0; j-- {
		if in.held[j].id == id {
			i = j
		}
	}
	if i < 0 {
		return nil
	}
	return &in.held[i]
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

// find returns where note id is held, or -1. A note the pedal holds
// after its NoteOff has given its ID up, for a new note to take.
func (in *Instrument) find(id int32) int {
	for i := range in.n {
		if in.held[i].id == id && !in.held[i].sustained {
			return i
		}
	}
	return -1
}

// let lets go of the note held at i, unless its voice has been taken for
// another, and forgets it.
func (in *Instrument) let(i int) {
	if h := &in.held[i]; h.v != nil && h.v.on && h.v.age == h.age {
		h.v.release()
	} else if in.sf != nil {
		in.fontVoices(h.age, (*sfVoice).release)
	}
	copy(in.held[i:in.n], in.held[i+1:in.n])
	in.n--
}

// SetKnobs turns the patch by k, the notes sounding as well as those to
// come.
func (in *Instrument) SetKnobs(k Knobs) {
	in.k = k
	o := in.orig
	cut := o.Filter.Cutoff
	if cut <= 0 {
		cut = 20000
	}
	in.src.Filter.Cutoff = min(max(cut*math.Exp2(k.Cutoff), 20), 20000)
	in.src.Filter.Res = min(max(o.Filter.Res+k.Res, 0), 0.98)
	in.src.Filter.Env = o.Filter.Env + k.Env
	in.src.Amp.Attack = o.Amp.Attack * math.Exp2(k.Attack)
	in.src.Amp.Decay = o.Amp.Decay * math.Exp2(k.Decay)
	in.src.Amp.Sustain = min(max(o.Amp.Sustain+k.Sustain, 0), 1)
	in.src.Amp.Release = o.Amp.Release * math.Exp2(k.Release)
	in.src.Drive = min(max(o.Drive+k.Drive, 0), 2)
	in.src.Noise = min(max(o.Noise+k.Noise, 0), 1)
	lc := in.p.live
	lc.tune, lc.width, lc.fm, lc.pan = float32(k.Tune), float32(k.Width), float32(k.FM), float32(k.Pan)
	lc.lfoRate, lc.lfoDepth = float32(math.Exp2(k.LFORate)), float32(max(1+k.LFODepth, 0))
	lc.vowel = 0
	if _, ok := formants[k.Vowel]; ok {
		lc.vowel = k.Vowel
	}
	lc.mod, lc.press = float32(min(max(k.Mod, 0), 1)), float32(min(max(k.Pressure, 0), 1))
	in.level = float32(math.Pow(10, k.Gain/20))
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
func (in *Instrument) Render(l, r []float32) { in.RenderParts(l, r, nil) }

// Held returns how many notes the instrument holds: struck and not let
// go, or let go while the pedal holds them.
func (in *Instrument) Held() int { return in.n }

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
	if in.sf != nil {
		for i := range in.sf.vs {
			if in.sf.vs[i].on {
				return true
			}
		}
	}
	return false
}

// Tail returns how long, in seconds, the instrument rings once its
// notes are let go: its envelope's release, as the knobs stretch it, or
// a drums patch's longest drum.
// It counts the patch as it stands now, with the knobs as last set: a
// note started before a knob changed may ring longer.
func (in *Instrument) Tail() float64 {
	if in.p.kind == kindFont {
		// A bank's release is its regions' own, a second or two at most
		// for most; a cymbal or a held piano note rings longer.
		return fontTail * math.Exp2(in.k.Release)
	}
	if in.p.kind != kindDrums {
		return in.src.Amp.Release
	}
	// A crash rings longest, 6.6 s times its Decay.
	tail := 0.0
	for _, d := range in.p.kit {
		tail = max(tail, 6.6*max(d.Decay, 1))
	}
	return tail
}

// Pass hands what the player holds to instrument to, as a sound
// changes under the player's hands: each pedal down goes down on to,
// and each note held starts again on to, at its pitch, velocity, bend
// and group, the notes the pedal holds held there by it too. On in,
// every note lets go and rings out through its release. A drums patch
// keeps the notes passed to it without striking them, as a drum struck
// rings out by itself, and passes them on to the sound after it.
func (in *Instrument) Pass(to *Instrument) {
	for i := range in.pedals {
		if pd := &in.pedals[i]; pd.used && pd.down {
			to.Pedal(pd.group, 1)
		}
	}
	// held runs oldest first, so a note the pedal holds starts before a
	// newer one of its ID, which takes the ID.
	for i := range in.n {
		h := in.held[i]
		if to.p.kind == kindDrums {
			if to.n < maxLive {
				h.v, h.age = nil, 0
				to.held[to.n] = h
				to.n++
			}
			continue
		}
		to.NoteOnTurn(h.id, float64(h.n.pitch), float64(h.n.vel), h.group, h.turn)
		if h.bend != 0 {
			to.Bend(h.id, float64(h.bend))
		}
		if h.mod != 0 {
			to.Mod(h.id, float64(h.mod))
		}
		if h.press != 0 {
			to.Pressure(h.id, float64(h.press))
		}
		if h.sustained {
			to.NoteOff(h.id)
		}
	}
	in.Release()
}

// Release lets go of every note, and lifts every pedal.
func (in *Instrument) Release() {
	for in.n > 0 {
		in.let(in.n - 1)
	}
	in.pedals = [maxPedals]pedal{}
}

// Silence stops every note at once, and lifts every pedal; a patch's
// Vowels start again from the first.
func (in *Instrument) Silence() {
	in.Release()
	in.vowels = 0
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
	if in.sf != nil {
		in.sf.silence()
	}
}

// fontTail is how long, in seconds, a SoundFont patch is taken to ring
// once let go.
const fontTail = 3

// Retune plays next's patch in place of its own, the notes sounding as
// well as those to come, where the two patches are alike in shape, as a
// patch edited is to itself before: of one kind, other than drums, with
// as many voices, the same waves and as many copies of each, the same
// filter, as many LFOs, and the same vowel bank. The notes sounding
// play on, through their envelopes, at the knobs as last set. It reports
// whether it did; where it did not, the caller hands the notes on with
// Pass. It allocates nothing, so it runs on the audio thread, and next
// is used up: play it no more.
func (in *Instrument) Retune(next *Instrument) bool {
	if !in.p.sameShape(next.p) {
		return false
	}
	live := in.p.live
	live.modR, live.pressR = next.p.live.modR, next.p.live.pressR
	in.orig, in.src = next.orig, next.src
	*in.p = *next.p
	in.p.src, in.p.live = &in.src, live
	in.SetKnobs(in.k)
	if in.vs != nil {
		for _, v := range in.vs.vs {
			if v.on {
				v.fenv.set(in.src.FilterEnv)
			}
		}
	}
	return true
}

// sameShape reports whether p and q are alike enough in shape for a
// voice playing one to play on with the other, as Retune says.
func (p *patch) sameShape(q *patch) bool {
	if p.kind == kindFont && (p.font != q.font || p.src.Bank != q.src.Bank || p.src.Program != q.src.Program) {
		return false
	}
	if p.kind != q.kind || p.kind == kindDrums || p.poly != q.poly || len(p.osc) != len(q.osc) ||
		p.filter != q.filter || len(p.lfo) != len(q.lfo) || p.vocoder != q.vocoder || p.singer != q.singer {
		return false
	}
	for i := range p.osc {
		a, b := &p.osc[i], &q.osc[i]
		if a.wave != b.wave || a.unison != b.unison || a.Sync != b.Sync || a.Ring != b.Ring {
			return false
		}
	}
	return true
}

// Vowel returns the vowel note id sings, as a, or 0 where it sings
// none or is not held.
func (in *Instrument) Vowel(id int32) byte {
	if h := in.control(id); h != nil {
		return h.n.vowel
	}
	return 0
}
