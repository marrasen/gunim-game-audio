package synth

import (
	"fmt"
	"math"
	"strings"
)

// A Patch is an instrument: how a note sounds.
//
// A synth patch is subtractive, as an analogue synth: its oscillators,
// layered, run through a filter, shaped by an envelope, and moved by
// LFOs. A pluck patch plucks a string, as Karplus and Strong did, and
// then runs through the same filter. A drums patch is a kit of drums,
// each made in code, played by name.
type Patch struct {
	// Kind is synth, the default, pluck or drums.
	Kind string
	// Osc are the oscillators, layered.
	Osc []Osc
	// Noise is how much white noise joins them.
	Noise float64
	// Filter shapes the oscillators' sound.
	Filter Filter
	// Amp shapes the note's level, and FilterEnv its filter, by
	// Filter.Env octaves at its peak.
	Amp, FilterEnv Env
	LFO            []LFO
	// Poly is how many notes play at once; 1 plays one at a time,
	// gliding from note to note over Glide seconds.
	Poly  int
	Glide float64
	// Drive saturates the oscillators before the filter, from 0.
	Drive float64
	// Drift detunes each note by up to that many cents, at random, as
	// an analogue synth's oscillators drift: a chord's notes beat
	// against each other, and no two notes are quite alike.
	Drift float64
	// Vowel makes the patch sing a vowel, a, e, i, o or u, as a voice
	// does; a track's vowel parameter changes it note by note.
	Vowel string
	// Vocoder sings the vowel through a vocoder's ten bands, as a
	// Roland VP-330's, in place of a voice's three resonances: the
	// robot's voice of electronic pop, which a track's vowels make speak.
	Vocoder bool `json:",omitempty"`
	// Pluck is how a pluck patch's string sounds.
	Pluck Pluck
	// Arpeggio plays a chord as one voice, its notes in turn, fast, as a
	// Commodore 64's tunes do.
	Arpeggio *Arpeggio
	// Chip steps the level as a console does, and Bend slides each note
	// in from off its pitch.
	Chip *Chip
	Bend *Bend
	// Kit is a drums patch's drums, by the names a pattern plays them
	// by. A name the kit leaves out is a drum of the same name, as bd,
	// sn, cp, hh, oh, rim, lt, mt, ht, cr, rd, sh, snap, tim, boom,
	// riser, down, the syn-toms syn1 to syn3, or cb, a cowbell; see Drum.
	Kit map[string]Drum
	// Gain is the patch's level, 1 by default.
	Gain float64
}

// Osc is one of a synth patch's oscillators.
type Osc struct {
	// Wave is saw, square, pulse, tri, sine, or fm, a sine whose phase
	// another sine moves, as a bell or an electric piano. A Commodore
	// 64's SID chip gives noise, pitched by the note as its noise is, and
	// the waves it makes of two at once, sawtri, pulsetri and pulsesaw,
	// thin and buzzing.
	Wave string
	// The Nintendo chips give nespulse, a pulse snapped to the widths of
	// 12.5, 25, 50 and 75% they have; nestri, the NES's stepped triangle;
	// nesnoise and nesmetal, the NES's noise in its long mode and its
	// short, metallic one; and gbwave, the Game Boy's wave channel,
	// playing Table.
	//
	// Table is a gbwave's wave: 32 steps of 0 to 15, GBWave where empty.
	Table []int
	// Sync restarts it each time the oscillator before it starts a
	// cycle, and Ring turns it over each half cycle of that one, as the
	// SID's hard sync and ring modulation do.
	Sync, Ring bool
	// Octave and Semi move it from the note, and Detune by cents.
	Octave int
	Semi   float64
	Detune float64
	// Level is how loud it is in the layer, 1 by default.
	Level float64
	// Unison is how many copies of it play, each Spread cents apart,
	// spread between the speakers, as a supersaw: 7 copies 30 cents
	// apart sound huge.
	Unison int
	Spread float64
	// Width is a pulse's width, from 0 to 1; 0.5 is a square.
	Width float64
	// Ratio is an fm wave's modulator's frequency, a multiple of the
	// note's, and Index how far it moves the phase, falling to
	// Index*Sustain over Decay seconds.
	Ratio, Index, Decay, Sustain float64
}

// Arpeggio is a patch's fast arpeggio, as a Commodore 64 plays a chord
// on one voice.
type Arpeggio struct {
	// Steps are the semitones above the note it steps through, as 0 4 7.
	Steps []int
	// Chord plays a track's chords, ch, so, their notes for steps.
	Chord bool
	// Hz is how many steps a second, 50 by default, a PAL machine's
	// frames.
	Hz float64
}

// Filter is a synth patch's filter.
type Filter struct {
	// Type is lp, a lowpass of 12 dB an octave; lp24, of 24; hp; bp; or
	// none. sidlp, sidbp, sidhp and sidnotch are the SID chip's filter,
	// of 12 dB an octave, its resonance rough and its sound driven, as
	// the 6581's is.
	Type string
	// Cutoff is where it cuts, in hertz, and Res its resonance, from 0
	// to 1.
	Cutoff, Res float64
	// Env is how many octaves the filter envelope opens it by, and Key
	// how far it follows the note, from 0 to 1, and Vel how many octaves
	// a note's velocity opens it by.
	Env, Key, Vel float64
}

// Pluck is how a pluck patch's string sounds.
type Pluck struct {
	// Decay is how long the string rings, in seconds, and Bright how
	// bright its pluck is, from 0 to 1.
	Decay, Bright float64
	// Body is how much the string sounds through a body, as a violin's
	// or a guitar's, from 0 to 1.
	Body float64
}

// Drum is a drum of a kit.
type Drum struct {
	// Type is what it is: kick, snare, clap, hat, ohat, rim, tom,
	// crash, ride, shaker, snap, timpani, boom, riser or down; syntom, an
	// electronic tom as a Simmons drum's, its pitch diving; cowbell, a
	// TR-808's; or the
	// SID's, built a frame at a time as a Commodore 64's drums are:
	// sidkick, sidsnare, sidclap, sidhat, sidohat, sidtom or sidzap; or
	// the NES's and Game Boy's, at 60 frames a second: neskick, nessnare,
	// neshat, nesohat, nestom or nesmetal. A drum named for its type needs
	// no Type.
	Type string
	// Tune moves its pitch in semitones, Decay stretches how long it
	// rings, 1 by default, and Tone brightens it, from 0 to 1.
	Tune, Decay, Tone float64
	// Gain is its level, 1 by default, and Pan its place between the
	// speakers.
	Gain, Pan float64
}

// drumNames are the drums a kit has by name.
var drumNames = map[string]string{
	"bd": "kick", "kick": "kick",
	"sn": "snare", "snare": "snare",
	"cp": "clap", "clap": "clap",
	"hh": "hat", "hat": "hat",
	"oh": "ohat", "ohat": "ohat",
	"rim": "rim",
	"lt":  "tom", "mt": "tom", "ht": "tom", "tom": "tom",
	"cr": "crash", "crash": "crash",
	"rd": "ride", "ride": "ride",
	"sh": "shaker", "shaker": "shaker",
	"snap": "snap",
	"tim":  "timpani", "timpani": "timpani",
	"boom":  "boom",
	"riser": "riser",
	"down":  "down",
	"sbd":   "sidkick", "ssn": "sidsnare", "scp": "sidclap", "shh": "sidhat", "soh": "sidohat",
	"stom": "sidtom", "szap": "sidzap",
	"nbd": "neskick", "nsn": "nessnare", "nhh": "neshat", "noh": "nesohat", "ntom": "nestom", "nclk": "nesmetal",
	"ncp":  "nesclap",
	"syn1": "syntom", "syn2": "syntom", "syn3": "syntom", "syntom": "syntom",
	"cb": "cowbell", "cowbell": "cowbell",
}

// tomTune tunes the low and high toms either side of the middle one,
// and the syn-toms, syn1 the highest, as a kit numbers its toms.
var tomTune = map[string]float64{"lt": -5, "ht": 5, "syn1": 5, "syn3": -5}

// The types of drum.
const (
	drKick = iota
	drSnare
	drClap
	drHat
	drOHat
	drRim
	drTom
	drCrash
	drRide
	drShaker
	drSnap
	drTimpani
	drBoom
	drRiser
	drDown
	drSIDKick
	drSIDSnare
	drSIDClap
	drSIDHat
	drSIDOHat
	drSIDTom
	drSIDZap
	drNESKick
	drNESSnare
	drNESHat
	drNESOHat
	drNESTom
	drNESMetal
	drNESClap
	drSynTom
	drCowbell
)

var drumTypes = map[string]int{
	"kick": drKick, "snare": drSnare, "clap": drClap, "hat": drHat, "ohat": drOHat, "rim": drRim,
	"tom": drTom, "crash": drCrash, "ride": drRide, "shaker": drShaker, "snap": drSnap,
	"timpani": drTimpani, "boom": drBoom, "riser": drRiser, "down": drDown,
	"sidkick": drSIDKick, "sidsnare": drSIDSnare, "sidclap": drSIDClap, "sidhat": drSIDHat, "sidohat": drSIDOHat,
	"sidtom": drSIDTom, "sidzap": drSIDZap,
	"neskick": drNESKick, "nessnare": drNESSnare, "neshat": drNESHat, "nesohat": drNESOHat, "nestom": drNESTom, "nesmetal": drNESMetal,
	"nesclap": drNESClap,
	"syntom":  drSynTom, "cowbell": drCowbell,
}

// The waves of an oscillator.
const (
	oscSaw = iota
	oscSquare
	oscTri
	oscSine
	oscFM
	oscNoise
	oscSawTri
	oscPulseTri
	oscPulseSaw
	oscTable
	oscNESNoise
)

// The types of filter.
const (
	filterNone = iota
	filterLP
	filterLP24
	filterHP
	filterBP
	filterSIDLP
	filterSIDBP
	filterSIDHP
	filterSIDNotch
)

// The kinds of patch.
const (
	kindSynth = iota
	kindPluck
	kindDrums
)

// patch is a Patch made ready to play: its names read into numbers.
type patch struct {
	src *Patch
	// arpHz is how many notes a second an arpeggio steps through, its
	// steps the semitones it steps through, and arpChord says it plays a
	// track's chords so.
	arpHz    float64
	arpSteps []float32
	arpChord bool
	// chipFrame is how many samples a chip patch's frame lasts, and
	// chipLevels how many steps its level has.
	chipFrame  int64
	chipLevels int
	kind       int
	osc        []osc
	filter     int
	lfo        []lfo
	vowel      byte
	vocoder    bool
	poly       int
	kit        map[string]drum
	gain       float32
}

type osc struct {
	Osc
	wave  int
	level float32
	// ratio is the oscillator's frequency as a multiple of the note's.
	ratio  float32
	unison int
	// table is a cycle of a combined wave.
	table []float32
	// detunes are each copy's frequency multiple, and gl and gr its
	// gains in the left and right channels, its level and place in them.
	detunes []float32
	gl, gr  []float32
}

type lfo struct {
	LFO
	to, wave int
}

// The targets of an LFO.
const (
	toPitch = iota
	toCutoff
	toAmp
	toWidth
	toPan
)

type drum struct {
	Drum
	// name is the drum's name in its kit, as bd.
	name string
	kind int
	gain float32
}

// compile readies p, named name for its errors.
func (p *Patch) compile(name string) (*patch, error) {
	c := &patch{src: p, gain: 1, poly: p.Poly}
	if p.Gain != 0 {
		c.gain = float32(p.Gain)
	}
	switch p.Kind {
	case "", "synth":
		c.kind = kindSynth
	case "pluck":
		c.kind = kindPluck
	case "drums":
		c.kind = kindDrums
	default:
		return nil, fmt.Errorf("synth: patch %s is of kind %q, not synth, pluck or drums", name, p.Kind)
	}
	if c.poly == 0 {
		c.poly = 8
		if c.kind == kindDrums {
			c.poly = 16
		}
	}
	c.poly = min(c.poly, 32)
	for i, o := range p.Osc {
		co := osc{Osc: o, level: 1, unison: max(o.Unison, 1)}
		if o.Level != 0 {
			co.level = float32(o.Level)
		}
		switch o.Wave {
		case "saw", "":
			co.wave = oscSaw
		case "square", "pulse":
			co.wave = oscSquare
			if co.Width == 0 {
				co.Width = 0.5
			}
		case "tri", "triangle":
			co.wave = oscTri
		case "sine":
			co.wave = oscSine
		case "fm":
			co.wave = oscFM
			if co.Ratio == 0 {
				co.Ratio = 1
			}
		case "noise":
			co.wave = oscNoise
		case "nespulse":
			co.wave = oscSquare
			co.Width = snapDuty(co.Width)
		case "nestri":
			co.wave, co.table = oscTable, stepped(nesTriangle)
		case "gbwave":
			steps := o.Table
			if len(steps) == 0 {
				steps = GBWave
			}
			for _, v := range steps {
				if v < 0 || v > 15 {
					return nil, fmt.Errorf("synth: patch %s, oscillator %d, has a step %d in its table, not 0 to 15", name, i+1, v)
				}
			}
			co.wave, co.table = oscTable, stepped(steps)
		case "nesnoise", "nesmetal":
			co.wave = oscNESNoise
		case "sawtri", "pulsetri", "pulsesaw":
			co.wave = map[string]int{"sawtri": oscSawTri, "pulsetri": oscPulseTri, "pulsesaw": oscPulseSaw}[o.Wave]
			if co.Width == 0 {
				co.Width = 0.5
			}
			co.table = combined(co.wave, co.Width)
		default:
			return nil, fmt.Errorf("synth: patch %s, oscillator %d, has wave %q, not saw, pulse, tri, sine, fm, noise, sawtri, pulsetri, pulsesaw, nespulse, nestri, nesnoise, nesmetal or gbwave", name, i+1, o.Wave)
		}
		if (o.Sync || o.Ring) && i == 0 {
			return nil, fmt.Errorf("synth: patch %s syncs or rings its first oscillator, which has none before it", name)
		}
		co.ratio = exp2(float32(float64(o.Octave) + o.Semi/12 + o.Detune/1200))
		for u := range co.unison {
			spread, pan := float32(0), float32(0)
			if co.unison > 1 {
				x := float32(u)/float32(co.unison-1)*2 - 1
				spread = x * float32(o.Spread) / 2
				pan = x
				if u%2 == 1 {
					pan = -x
				}
				pan *= 0.9
			}
			co.detunes = append(co.detunes, exp2(spread/1200))
			l, r := panGains(pan)
			co.gl = append(co.gl, l)
			co.gr = append(co.gr, r)
		}
		co.level /= float32(math.Sqrt(float64(co.unison)))
		for u := range co.gl {
			co.gl[u] *= co.level
			co.gr[u] *= co.level
		}
		c.osc = append(c.osc, co)
	}
	if c.kind == kindSynth && len(c.osc) == 0 && p.Noise == 0 {
		return nil, fmt.Errorf("synth: patch %s has no oscillators and no noise", name)
	}
	switch p.Filter.Type {
	case "", "none":
		c.filter = filterNone
	case "lp":
		c.filter = filterLP
	case "lp24":
		c.filter = filterLP24
	case "hp":
		c.filter = filterHP
	case "bp":
		c.filter = filterBP
	case "sidlp":
		c.filter = filterSIDLP
	case "sidbp":
		c.filter = filterSIDBP
	case "sidhp":
		c.filter = filterSIDHP
	case "sidnotch":
		c.filter = filterSIDNotch
	default:
		return nil, fmt.Errorf("synth: patch %s has filter %q, not lp, lp24, hp, bp, sidlp, sidbp, sidhp, sidnotch or none", name, p.Filter.Type)
	}
	if ch := p.Chip; ch != nil {
		hz := ch.Hz
		if hz <= 0 {
			hz = 60
		}
		c.chipFrame = max(int64(rate/hz), 1)
		c.chipLevels = ch.Levels
		if c.chipLevels <= 0 {
			c.chipLevels = 16
		}
	}
	if a := p.Arpeggio; a != nil {
		c.arpHz = a.Hz
		if c.arpHz <= 0 {
			c.arpHz = 50
		}
		c.arpChord = a.Chord
		for _, s := range a.Steps {
			c.arpSteps = append(c.arpSteps, float32(s))
		}
		if !a.Chord && len(c.arpSteps) == 0 {
			return nil, fmt.Errorf("synth: patch %s arpeggiates neither its chords nor steps", name)
		}
	}
	for _, l := range p.LFO {
		cl := lfo{LFO: l, wave: lfoWave(l.Wave)}
		switch l.To {
		case "pitch":
			cl.to = toPitch
		case "cutoff":
			cl.to = toCutoff
		case "amp":
			cl.to = toAmp
		case "width":
			cl.to = toWidth
		case "pan":
			cl.to = toPan
		default:
			return nil, fmt.Errorf("synth: patch %s has an LFO to %q, not pitch, cutoff, amp, width or pan", name, l.To)
		}
		c.lfo = append(c.lfo, cl)
	}
	if p.Vowel != "" {
		if _, ok := formants[p.Vowel[0]]; !ok {
			return nil, fmt.Errorf("synth: patch %s sings vowel %q, not a, e, i, o or u", name, p.Vowel)
		}
		c.vowel = p.Vowel[0]
	}
	c.vocoder = p.Vocoder
	if c.kind == kindDrums {
		c.kit = map[string]drum{}
		for n, t := range drumNames {
			c.kit[n] = drum{Drum: Drum{Type: t, Tune: tomTune[n]}, name: n, kind: drumTypes[t], gain: 1}
		}
		for n, d := range p.Kit {
			t := d.Type
			if t == "" {
				t = drumNames[n]
			}
			k, ok := drumTypes[t]
			if !ok {
				return nil, fmt.Errorf("synth: patch %s has drum %s of type %q, which is none of %s", name, n, t, strings.Join(sortedKeys(drumTypes), ", "))
			}
			if d.Tune == 0 {
				d.Tune = tomTune[n]
			}
			d.Type = t
			cd := drum{Drum: d, name: n, kind: k, gain: 1}
			if d.Gain != 0 {
				cd.gain = float32(d.Gain)
			}
			c.kit[n] = cd
		}
	}
	return c, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}
