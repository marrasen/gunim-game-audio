package synth

import "strconv"

// GMNames are the 128 instruments of General MIDI, by program number
// from 0, as the standard names them.
var GMNames = [128]string{
	"Acoustic Grand Piano", "Bright Acoustic Piano", "Electric Grand Piano", "Honky-tonk Piano",
	"Electric Piano 1", "Electric Piano 2", "Harpsichord", "Clavi",
	"Celesta", "Glockenspiel", "Music Box", "Vibraphone",
	"Marimba", "Xylophone", "Tubular Bells", "Dulcimer",
	"Drawbar Organ", "Percussive Organ", "Rock Organ", "Church Organ",
	"Reed Organ", "Accordion", "Harmonica", "Tango Accordion",
	"Acoustic Guitar (nylon)", "Acoustic Guitar (steel)", "Electric Guitar (jazz)", "Electric Guitar (clean)",
	"Electric Guitar (muted)", "Overdriven Guitar", "Distortion Guitar", "Guitar Harmonics",
	"Acoustic Bass", "Electric Bass (finger)", "Electric Bass (pick)", "Fretless Bass",
	"Slap Bass 1", "Slap Bass 2", "Synth Bass 1", "Synth Bass 2",
	"Violin", "Viola", "Cello", "Contrabass",
	"Tremolo Strings", "Pizzicato Strings", "Orchestral Harp", "Timpani",
	"String Ensemble 1", "String Ensemble 2", "Synth Strings 1", "Synth Strings 2",
	"Choir Aahs", "Voice Oohs", "Synth Voice", "Orchestra Hit",
	"Trumpet", "Trombone", "Tuba", "Muted Trumpet",
	"French Horn", "Brass Section", "Synth Brass 1", "Synth Brass 2",
	"Soprano Sax", "Alto Sax", "Tenor Sax", "Baritone Sax",
	"Oboe", "English Horn", "Bassoon", "Clarinet",
	"Piccolo", "Flute", "Recorder", "Pan Flute",
	"Blown Bottle", "Shakuhachi", "Whistle", "Ocarina",
	"Lead 1 (square)", "Lead 2 (sawtooth)", "Lead 3 (calliope)", "Lead 4 (chiff)",
	"Lead 5 (charang)", "Lead 6 (voice)", "Lead 7 (fifths)", "Lead 8 (bass + lead)",
	"Pad 1 (new age)", "Pad 2 (warm)", "Pad 3 (polysynth)", "Pad 4 (choir)",
	"Pad 5 (bowed)", "Pad 6 (metallic)", "Pad 7 (halo)", "Pad 8 (sweep)",
	"FX 1 (rain)", "FX 2 (soundtrack)", "FX 3 (crystal)", "FX 4 (atmosphere)",
	"FX 5 (brightness)", "FX 6 (goblins)", "FX 7 (echoes)", "FX 8 (sci-fi)",
	"Sitar", "Banjo", "Shamisen", "Koto",
	"Kalimba", "Bag pipe", "Fiddle", "Shanai",
	"Tinkle Bell", "Agogo", "Steel Drums", "Woodblock",
	"Taiko Drum", "Melodic Tom", "Synth Drum", "Reverse Cymbal",
	"Guitar Fret Noise", "Breath Noise", "Seashore", "Bird Tweet",
	"Telephone Ring", "Helicopter", "Applause", "Gunshot",
}

// GMPatch returns General MIDI's instrument program, from 0 to 127, as
// a patch of this engine's. Each call makes it afresh, so the patch is
// the caller's own.
func GMPatch(program int) *Patch {
	p := gmPatch(program & 127)
	if p.Poly == 0 {
		p.Poly = 16
	}
	if p.Gain == 0 {
		p.Gain = 1
	}
	p.Gain *= dbGain(gmLevels[program&127])
	return p
}

// gmLevels are how many decibels each instrument is turned up or down,
// measured so that each plays about as loud as the others: a chord at
// velocity 100, held for a second and a half, half by its loudness and
// half by its peak, so a plucked or struck note, which dies away, is
// not turned up as far as its loudness alone asks.
var gmLevels = [128]float64{
	2, 1, 1.5, 2, 2.5, 4, 9, 4,
	2.5, 4.5, 3, 1.5, 3.5, 3.5, 1, 9,
	-4.5, -3.5, -4.5, -1.5, -0.5, -2.5, -0.5, -2.5,
	10.5, 9.5, 17.5, 10, 18, 1.5, -0.5, 6,
	18, 17.5, 14, 4.5, 3, 2, 2.5, 3,
	-1, -1, -1, -0.5, -0.5, 10.5, 9.5, 6.5,
	1, 1.5, 1, -1.5, -3.5, -4, 4, 3.5,
	1.5, 2, 3.5, 7.5, 4, 1.5, 2, 0.5,
	-3, -3, -2.5, -2.5, 1, 0.5, 1.5, -1.5,
	-0.5, -0.5, -1.5, 0.5, -3, 0, 0.5, 0,
	-3, 0, 0.5, 1, 1, -3.5, 0.5, 0.5,
	-0.5, 1, -2, 2, 1.5, 1, 4.5, 6,
	4, 1, 1, 2.5, 0.5, 1, 0.5, 1.5,
	9, 9, 9.5, 10, 3.5, -3.5, -1, 2,
	3, 4, 3.5, 6, 7.5, 7.5, 4.5, 3,
	17.5, 15.5, 10, -1, -7, 12, 12, 13.5,
}

// adsr is an envelope's shape, written short.
func adsr(a, d, s, r float64) Env { return Env{Attack: a, Decay: d, Sustain: s, Release: r} }

// vibrato is a pitch LFO of depth semitones at hz, fading in after
// delay seconds, as a player's vibrato does.
func vibrato(hz, depth, delay float64) LFO {
	return LFO{To: "pitch", Wave: "sine", Hz: hz, Depth: depth, Delay: delay}
}

// pluck is a plucked string's patch.
func pluck(decay, bright, body, cutoff float64) *Patch {
	p := &Patch{Kind: "pluck", Pluck: Pluck{Decay: decay, Bright: bright, Body: body}, Amp: adsr(0.001, 1, 1, 0.15)}
	if cutoff > 0 {
		p.Filter = Filter{Type: "lp", Cutoff: cutoff, Key: 0.3}
	}
	return p
}

// bell is a struck, FM patch: a sine and a modulator of ratio, struck
// to index, ringing for decay seconds.
func bell(ratio, index, strike, decay float64) *Patch {
	return &Patch{
		Osc: []Osc{
			{Wave: "fm", Ratio: ratio, Index: index, Decay: strike, Sustain: 0.1},
			{Wave: "sine", Level: 0.5},
		},
		Amp: adsr(0.001, decay, 0, decay/3+0.1),
	}
}

// bowed is a bowed string's patch, cut at cutoff.
func bowed(cutoff, attack float64) *Patch {
	return &Patch{
		Osc:       []Osc{{Wave: "saw"}, {Wave: "saw", Detune: 7, Level: 0.6}},
		Filter:    Filter{Type: "lp", Cutoff: cutoff, Res: 0.1, Env: 0.6, Key: 0.3, Vel: 0.8},
		FilterEnv: adsr(attack, 0.4, 0.6, 0.3),
		Amp:       adsr(attack, 0.3, 0.9, 0.3),
		LFO:       []LFO{vibrato(5.5, 0.12, 0.3)},
		Drift:     3,
	}
}

// ensemble is a section of strings, cut at cutoff.
func ensemble(cutoff, attack, release float64) *Patch {
	return &Patch{
		Osc:    []Osc{{Wave: "saw", Unison: 3, Spread: 14}, {Wave: "saw", Octave: 1, Level: 0.25}},
		Filter: Filter{Type: "lp", Cutoff: cutoff, Res: 0.05, Key: 0.3, Vel: 0.6},
		Amp:    adsr(attack, 1, 0.9, release),
		Drift:  4,
	}
}

// brass is a brass instrument's patch, its filter opening by env
// octaves from cutoff as it is blown.
func brass(cutoff, env, attack float64) *Patch {
	return &Patch{
		Osc:       []Osc{{Wave: "saw"}, {Wave: "saw", Detune: 5, Level: 0.4}},
		Filter:    Filter{Type: "lp", Cutoff: cutoff, Res: 0.1, Env: env, Key: 0.5, Vel: 1.2},
		FilterEnv: adsr(attack*2, 0.3, 0.55, 0.2),
		Amp:       adsr(attack, 0.2, 0.85, 0.15),
		LFO:       []LFO{vibrato(5.5, 0.1, 0.4)},
		Drift:     3,
	}
}

// sax is a reed's patch, cut at cutoff.
func sax(cutoff float64) *Patch {
	return &Patch{
		Osc:       []Osc{{Wave: "pulse", Width: 0.35}, {Wave: "saw", Level: 0.5}},
		Noise:     0.03,
		Filter:    Filter{Type: "lp", Cutoff: cutoff, Res: 0.2, Env: 1.4, Key: 0.5, Vel: 1},
		FilterEnv: adsr(0.03, 0.3, 0.5, 0.2),
		Amp:       adsr(0.02, 0.2, 0.85, 0.12),
		LFO:       []LFO{vibrato(5, 0.12, 0.35)},
		Drive:     0.2,
		Drift:     3,
	}
}

// pipe is a blown pipe's patch: a sine and a triangle, with breath.
func pipe(tri, breath, cutoff float64) *Patch {
	return &Patch{
		Osc:    []Osc{{Wave: "sine"}, {Wave: "tri", Level: tri}},
		Noise:  breath,
		Filter: Filter{Type: "lp", Cutoff: cutoff, Key: 0.8},
		Amp:    adsr(0.03, 0.2, 0.9, 0.1),
		LFO:    []LFO{vibrato(5.5, 0.1, 0.3)},
	}
}

// gmPatch makes the patch of program.
func gmPatch(program int) *Patch {
	switch program {
	// Pianos.
	case 0, 1, 2:
		cut := []float64{1800, 3500, 2600}[program]
		return &Patch{
			Osc: []Osc{
				{Wave: "fm", Ratio: 1, Index: 1.3, Decay: 1, Sustain: 0.15},
				{Wave: "fm", Ratio: 1, Index: 0.6, Decay: 0.8, Sustain: 0.1, Detune: 3, Level: 0.6},
				{Wave: "fm", Ratio: 4, Index: 0.8, Decay: 0.04, Sustain: 0, Level: 0.2},
			},
			Filter:    Filter{Type: "lp", Cutoff: cut, Env: 1.5, Key: 0.8, Vel: 1.5},
			FilterEnv: adsr(0.001, 1.5, 0.3, 0.4),
			Amp:       adsr(0.002, 5, 0, 0.4),
			Drift:     2,
		}
	case 3:
		return &Patch{
			Osc: []Osc{
				{Wave: "fm", Ratio: 1, Index: 1.3, Decay: 1, Sustain: 0.15},
				{Wave: "fm", Ratio: 1, Index: 1, Decay: 1, Sustain: 0.15, Detune: 16, Level: 0.8},
			},
			Filter:    Filter{Type: "lp", Cutoff: 3000, Env: 1, Key: 0.8, Vel: 1.2},
			FilterEnv: adsr(0.001, 1, 0.3, 0.4),
			Amp:       adsr(0.002, 4, 0, 0.4),
		}
	case 4:
		return &Patch{
			Osc: []Osc{
				{Wave: "fm", Ratio: 1, Index: 0.9, Decay: 0.8, Sustain: 0.15},
				{Wave: "fm", Ratio: 1, Index: 0.3, Decay: 1, Sustain: 0, Detune: 4, Level: 0.6},
			},
			Amp: adsr(0.002, 3, 0, 0.3),
			LFO: []LFO{{To: "amp", Wave: "sine", Hz: 4.5, Depth: 0.12}},
		}
	case 5:
		return &Patch{
			Osc: []Osc{
				{Wave: "fm", Ratio: 1, Index: 1, Decay: 0.6, Sustain: 0.25},
				{Wave: "fm", Ratio: 14, Index: 2, Decay: 0.03, Sustain: 0, Level: 0.35},
			},
			Amp: adsr(0.001, 2, 0, 0.25),
		}
	case 6:
		return pluck(1.6, 1, 0.15, 0)
	case 7:
		return &Patch{
			Osc:       []Osc{{Wave: "pulse", Width: 0.25}},
			Filter:    Filter{Type: "lp", Cutoff: 1800, Res: 0.35, Env: 2, Key: 0.5, Vel: 1},
			FilterEnv: adsr(0.001, 0.15, 0.2, 0.1),
			Amp:       adsr(0.001, 1.2, 0, 0.08),
		}

	// Chromatic percussion.
	case 8:
		return bell(3.5, 0.9, 0.15, 1.5)
	case 9:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 3.01, Index: 1.4, Decay: 0.2, Sustain: 0.1}, {Wave: "tri", Level: 0.4}},
			Filter: Filter{Type: "lp", Cutoff: 6000},
			Amp:    adsr(0.001, 1.6, 0, 0.5),
		}
	case 10:
		return bell(5.4, 0.8, 0.05, 1.5)
	case 11:
		p := bell(4, 0.5, 0.1, 3)
		p.LFO = []LFO{{To: "amp", Wave: "sine", Hz: 5, Depth: 0.3}}
		return p
	case 12:
		return bell(4, 1, 0.02, 0.6)
	case 13:
		p := bell(3, 2, 0.02, 0.35)
		p.Amp.Release = 0.1
		return p
	case 14:
		return &Patch{
			Osc: []Osc{{Wave: "fm", Ratio: 3.5, Index: 2.2, Decay: 1.2, Sustain: 0.2}, {Wave: "sine", Level: 0.4}},
			Amp: adsr(0.001, 3, 0, 1.5),
		}
	case 15:
		return pluck(2, 0.9, 0.5, 0)

	// Organs.
	case 16:
		return &Patch{
			Osc: []Osc{
				{Wave: "sine"}, {Wave: "sine", Octave: 1, Level: 0.6},
				{Wave: "sine", Octave: 1, Semi: 7, Level: 0.4}, {Wave: "sine", Octave: 2, Level: 0.3},
			},
			Amp: adsr(0.005, 0.1, 1, 0.05),
			LFO: []LFO{{To: "amp", Wave: "sine", Hz: 6.5, Depth: 0.08}},
		}
	case 17:
		return &Patch{
			Osc: []Osc{
				{Wave: "sine"}, {Wave: "sine", Octave: 1, Level: 0.5},
				{Wave: "fm", Ratio: 3, Index: 1.5, Decay: 0.15, Sustain: 0, Level: 0.5},
			},
			Amp: adsr(0.003, 0.1, 1, 0.05),
		}
	case 18:
		return &Patch{
			Osc: []Osc{
				{Wave: "sine"}, {Wave: "sine", Octave: 1, Level: 0.8},
				{Wave: "sine", Octave: 1, Semi: 7, Level: 0.6}, {Wave: "square", Octave: 2, Level: 0.1},
			},
			Amp:   adsr(0.003, 0.1, 1, 0.05),
			LFO:   []LFO{{To: "amp", Wave: "sine", Hz: 6.8, Depth: 0.12}},
			Drive: 0.5,
		}
	case 19:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}, {Wave: "saw", Octave: 1, Level: 0.5}, {Wave: "sine", Octave: -1, Level: 0.6}},
			Filter: Filter{Type: "lp", Cutoff: 2500, Key: 0.4},
			Amp:    adsr(0.06, 0.1, 1, 0.8),
		}
	case 20:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.3}},
			Filter: Filter{Type: "lp", Cutoff: 1800, Key: 0.5},
			Amp:    adsr(0.04, 0.1, 1, 0.1),
		}
	case 21, 23:
		spread := []float64{10, 22}[program/23]
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: spread}, {Wave: "pulse", Width: 0.3, Level: 0.6}},
			Filter: Filter{Type: "lp", Cutoff: 2500, Res: 0.1, Key: 0.5},
			Amp:    adsr(0.03, 0.1, 1, 0.1),
			LFO:    []LFO{{To: "amp", Wave: "sine", Hz: 5, Depth: 0.06}},
		}
	case 22:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.3}},
			Noise:  0.05,
			Filter: Filter{Type: "lp", Cutoff: 2200, Res: 0.25, Key: 0.5, Vel: 1},
			Amp:    adsr(0.03, 0.2, 0.9, 0.1),
			LFO:    []LFO{vibrato(5, 0.12, 0.3)},
			Vowel:  "e",
		}

	// Guitars.
	case 24:
		return pluck(1.6, 0.45, 0.7, 0)
	case 25:
		return pluck(2.2, 0.75, 0.5, 0)
	case 26:
		return pluck(1.4, 0.35, 0.4, 1500)
	case 27:
		return pluck(2, 0.6, 0.2, 0)
	case 28:
		p := pluck(0.25, 0.4, 0.3, 1500)
		p.Amp.Release = 0.05
		return p
	case 29, 30:
		p := &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 8}, {Wave: "square", Octave: -1, Level: 0.3}},
			Filter: Filter{Type: "lp", Cutoff: 2500, Res: 0.2, Key: 0.3},
			Amp:    adsr(0.002, 1.5, 0.6, 0.15),
			Drive:  0.6,
			Gain:   0.7,
		}
		if program == 30 {
			p.Osc[0].Unison, p.Osc[0].Spread = 3, 12
			p.Drive, p.Filter.Cutoff, p.Amp.Sustain = 1, 3000, 0.8
		}
		return p
	case 31:
		return &Patch{
			Osc: []Osc{{Wave: "sine", Octave: 1}, {Wave: "fm", Octave: 1, Ratio: 2, Index: 0.3, Decay: 0.1, Level: 0.3}},
			Amp: adsr(0.002, 2, 0, 0.3),
		}

	// Basses.
	case 32:
		return pluck(1.2, 0.3, 0.6, 900)
	case 33:
		return pluck(1.5, 0.4, 0.3, 1400)
	case 34:
		return pluck(1.5, 0.7, 0.2, 2500)
	case 35:
		return &Patch{
			Osc:    []Osc{{Wave: "tri"}, {Wave: "saw", Level: 0.2}},
			Filter: Filter{Type: "lp", Cutoff: 1200, Key: 0.3},
			Amp:    adsr(0.01, 1.5, 0.5, 0.2),
		}
	case 36, 37:
		p := &Patch{
			Osc: []Osc{
				{Wave: "fm", Ratio: 1, Index: 2.4, Decay: 0.12, Sustain: 0.2},
				{Wave: "fm", Ratio: 3, Index: 1.2, Decay: 0.03, Sustain: 0, Level: 0.3},
			},
			Amp: adsr(0.001, 0.5, 0.4, 0.08),
		}
		if program == 37 {
			p.Osc[0].Index, p.Osc[1].Level = 3.2, 0.5
		}
		return p
	case 38:
		return &Patch{
			Osc:       []Osc{{Wave: "saw"}, {Wave: "saw", Detune: -6}, {Wave: "saw", Octave: -1, Level: 0.6}},
			Filter:    Filter{Type: "lp24", Cutoff: 260, Res: 0.25, Env: 2.2, Key: 0.3, Vel: 0.5},
			FilterEnv: adsr(0.002, 0.25, 0.2, 0.08),
			Amp:       adsr(0.003, 0.3, 0.8, 0.08),
			Drive:     0.4,
			Drift:     4,
		}
	case 39:
		return &Patch{
			Osc:       []Osc{{Wave: "saw"}, {Wave: "square", Octave: -1, Level: 0.6}},
			Filter:    Filter{Type: "lp24", Cutoff: 400, Res: 0.35, Env: 3, Key: 0.3, Vel: 0.8},
			FilterEnv: adsr(0.001, 0.2, 0.1, 0.1),
			Amp:       adsr(0.002, 0.4, 0.7, 0.08),
		}

	// Strings.
	case 40:
		return bowed(3500, 0.08)
	case 41:
		return bowed(2800, 0.09)
	case 42:
		p := bowed(2200, 0.1)
		p.LFO = []LFO{vibrato(5, 0.1, 0.3)}
		return p
	case 43:
		return bowed(1500, 0.12)
	case 44:
		p := ensemble(3200, 0.1, 0.4)
		p.LFO = []LFO{{To: "amp", Wave: "sine", Hz: 7, Depth: 0.4}}
		return p
	case 45:
		return pluck(0.4, 0.5, 0.8, 0)
	case 46:
		return pluck(2.2, 0.75, 0.3, 0)
	case 47:
		return &Patch{
			Osc:       []Osc{{Wave: "sine"}, {Wave: "fm", Ratio: 1.5, Index: 1, Decay: 0.1, Level: 0.4}},
			Noise:     0.1,
			Filter:    Filter{Type: "lp", Cutoff: 600, Env: 2, Key: 0.5},
			FilterEnv: adsr(0.001, 0.08, 0, 0.5),
			Amp:       adsr(0.002, 1.8, 0, 1),
		}

	// Ensembles.
	case 48:
		return ensemble(3500, 0.15, 0.6)
	case 49:
		return ensemble(2800, 0.5, 0.9)
	case 50:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 3, Spread: 12}, {Wave: "saw", Octave: 1, Level: 0.4}},
			Filter: Filter{Type: "lp", Cutoff: 3800, Res: 0.05},
			Amp:    adsr(0.35, 1, 0.9, 1),
			Drift:  4,
		}
	case 51:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 10}, {Wave: "pulse", Width: 0.4, Level: 0.6}},
			Filter: Filter{Type: "lp24", Cutoff: 1500, Res: 0.15},
			Amp:    adsr(0.4, 1, 0.85, 1.2),
			LFO:    []LFO{{To: "cutoff", Wave: "tri", Hz: 0.25, Depth: 0.5}, {To: "width", Wave: "sine", Hz: 0.7, Depth: 0.15}},
			Drift:  4,
		}
	case 52, 53:
		vowel := []string{"a", "u"}[program-52]
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 3, Spread: 18}, {Wave: "glottal", Level: 0.6}},
			Amp:    adsr(0.4, 1, 0.9, 1),
			Vowel:  vowel,
			Singer: "alto",
			LFO:    []LFO{{To: "pitch", Wave: "sine", Hz: 4.5, Depth: 0.06}},
			Drift:  6,
		}
	case 54:
		return &Patch{
			Osc:   []Osc{{Wave: "saw", Unison: 3, Spread: 14}, {Wave: "pulse", Width: 0.4, Octave: -1, Level: 0.3}},
			Amp:   adsr(0.3, 1, 0.85, 0.8),
			Vowel: "a",
			Drift: 4,
		}
	case 55:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 3, Spread: 18}, {Wave: "pulse", Width: 0.5, Octave: -1, Level: 0.6}},
			Noise:     0.15,
			Filter:    Filter{Type: "lp", Cutoff: 1800, Res: 0.1, Env: 1.6},
			FilterEnv: adsr(0.001, 0.25, 0, 0.2),
			Amp:       adsr(0.001, 0.4, 0, 0.2),
		}

	// Brass.
	case 56:
		return brass(1200, 2.5, 0.02)
	case 57:
		return brass(800, 2.2, 0.03)
	case 58:
		return brass(450, 1.8, 0.04)
	case 59:
		p := brass(1400, 1, 0.02)
		p.Filter.Type, p.Filter.Res = "bp", 0.4
		return p
	case 60:
		p := brass(600, 1.4, 0.05)
		p.Osc[1] = Osc{Wave: "tri", Level: 0.5}
		return p
	case 61:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 3, Spread: 14}, {Wave: "saw", Octave: -1, Level: 0.3}},
			Filter:    Filter{Type: "lp", Cutoff: 1100, Res: 0.1, Env: 2.2, Key: 0.4, Vel: 1},
			FilterEnv: adsr(0.04, 0.35, 0.5, 0.2),
			Amp:       adsr(0.02, 0.3, 0.85, 0.2),
			Drift:     4,
		}
	case 62:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 3, Spread: 14}, {Wave: "saw", Octave: -1, Level: 0.4}},
			Filter:    Filter{Type: "lp24", Cutoff: 1100, Res: 0.1, Env: 2, Key: 0.4, Vel: 0.6},
			FilterEnv: adsr(0.04, 0.35, 0.5, 0.2),
			Amp:       adsr(0.01, 0.3, 0.8, 0.2),
			Drift:     4,
		}
	case 63:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 3, Spread: 12}, {Wave: "pulse", Width: 0.4, Level: 0.4}},
			Filter:    Filter{Type: "lp24", Cutoff: 700, Res: 0.15, Env: 2.2, Key: 0.3},
			FilterEnv: adsr(0.2, 0.6, 0.6, 0.4),
			Amp:       adsr(0.08, 0.5, 0.85, 0.4),
			Drift:     4,
		}

	// Reeds.
	case 64:
		return sax(2500)
	case 65:
		return sax(1800)
	case 66:
		return sax(1300)
	case 67:
		return sax(900)
	case 68, 69:
		p := &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.18}},
			Filter: Filter{Type: "lp", Cutoff: 3000, Res: 0.2, Key: 0.5},
			Amp:    adsr(0.03, 0.2, 0.9, 0.1),
			LFO:    []LFO{vibrato(5.2, 0.1, 0.3)},
			Vowel:  "e",
		}
		if program == 69 {
			p.Vowel, p.Filter.Cutoff = "o", 2200
		}
		return p
	case 70:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.25}, {Wave: "saw", Level: 0.4}},
			Filter: Filter{Type: "lp", Cutoff: 900, Res: 0.15, Key: 0.5},
			Amp:    adsr(0.03, 0.2, 0.9, 0.1),
			Vowel:  "o",
		}
	case 71:
		return &Patch{
			Osc:    []Osc{{Wave: "square"}, {Wave: "tri", Level: 0.3}},
			Noise:  0.02,
			Filter: Filter{Type: "lp", Cutoff: 2200, Res: 0.1, Key: 0.5, Vel: 0.8},
			Amp:    adsr(0.03, 0.2, 0.9, 0.12),
			LFO:    []LFO{vibrato(5, 0.08, 0.4)},
		}

	// Pipes.
	case 72:
		return pipe(0.3, 0.04, 6000)
	case 73:
		return pipe(0.3, 0.06, 4000)
	case 74:
		p := pipe(0.6, 0.03, 4000)
		p.LFO = nil
		return p
	case 75:
		p := pipe(0.1, 0.15, 3000)
		p.Amp.Attack = 0.05
		return p
	case 76:
		p := pipe(0, 0.2, 1500)
		p.Filter.Res, p.LFO = 0.3, nil
		return p
	case 77:
		p := pipe(0.2, 0.15, 2500)
		p.LFO = []LFO{vibrato(5, 0.2, 0.4)}
		p.Bend = &Bend{Semis: -0.5, Time: 0.1}
		return p
	case 78:
		return &Patch{
			Osc:  []Osc{{Wave: "sine"}},
			Amp:  adsr(0.02, 0.1, 0.95, 0.08),
			LFO:  []LFO{vibrato(6, 0.1, 0.2)},
			Bend: &Bend{Semis: -1, Time: 0.05},
		}
	case 79:
		p := pipe(0.15, 0.02, 3000)
		p.Amp = adsr(0.02, 0.1, 0.95, 0.08)
		return p

	// Synth leads.
	case 80:
		return &Patch{
			Osc:       []Osc{{Wave: "pulse", Width: 0.5}, {Wave: "pulse", Width: 0.5, Detune: 8, Level: 0.5}},
			Filter:    Filter{Type: "lp24", Cutoff: 2400, Res: 0.1, Env: 0.8, Key: 0.4},
			FilterEnv: adsr(0.005, 0.3, 0.5, 0.2),
			Amp:       adsr(0.005, 0.3, 0.85, 0.2),
			LFO:       []LFO{{To: "pitch", Wave: "tri", Hz: 5.2, Depth: 0.12, Delay: 0.25}},
			Drift:     3,
		}
	case 81:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 2, Spread: 10}},
			Filter:    Filter{Type: "lp", Cutoff: 3500, Res: 0.2, Env: 1, Key: 0.4},
			FilterEnv: adsr(0.005, 0.3, 0.5, 0.2),
			Amp:       adsr(0.005, 0.3, 0.85, 0.2),
			LFO:       []LFO{vibrato(5.2, 0.1, 0.3)},
		}
	case 82:
		return &Patch{
			Osc:    []Osc{{Wave: "tri"}, {Wave: "sine", Octave: 1, Level: 0.4}},
			Noise:  0.08,
			Filter: Filter{Type: "lp", Cutoff: 3500, Key: 0.5},
			Amp:    adsr(0.02, 0.1, 0.9, 0.15),
			LFO:    []LFO{vibrato(6, 0.15, 0.1)},
		}
	case 83:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 1, Index: 2, Decay: 0.05, Sustain: 0.2}, {Wave: "tri", Level: 0.5}},
			Noise:  0.05,
			Filter: Filter{Type: "lp", Cutoff: 3000, Key: 0.5},
			Amp:    adsr(0.005, 0.2, 0.85, 0.15),
		}
	case 84:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}, {Wave: "pulse", Width: 0.3, Detune: 6, Level: 0.6}},
			Filter: Filter{Type: "lp", Cutoff: 3000, Res: 0.15, Key: 0.3},
			Amp:    adsr(0.003, 0.5, 0.75, 0.15),
			Drive:  0.8,
			Gain:   0.7,
		}
	case 85:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}, {Wave: "glottal", Level: 0.7}},
			Amp:    adsr(0.03, 0.2, 0.9, 0.2),
			Vowel:  "a",
			Singer: "tenor",
			LFO:    []LFO{vibrato(5.5, 0.12, 0.3)},
		}
	case 86:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}, {Wave: "saw", Semi: 7, Level: 0.7}},
			Filter: Filter{Type: "lp", Cutoff: 3000, Res: 0.1, Key: 0.4},
			Amp:    adsr(0.005, 0.3, 0.85, 0.2),
		}
	case 87:
		return &Patch{
			Osc:       []Osc{{Wave: "saw"}, {Wave: "saw", Octave: -1, Level: 0.7}},
			Filter:    Filter{Type: "lp", Cutoff: 2000, Res: 0.2, Env: 1.2, Key: 0.4},
			FilterEnv: adsr(0.003, 0.3, 0.4, 0.2),
			Amp:       adsr(0.003, 0.3, 0.85, 0.15),
		}

	// Pads.
	case 88:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 2, Index: 1.2, Decay: 2, Sustain: 0.5}, {Wave: "saw", Unison: 2, Spread: 8, Level: 0.4}},
			Filter: Filter{Type: "lp", Cutoff: 4500, Res: 0.2},
			Amp:    adsr(0.25, 1.5, 0.8, 1.5),
		}
	case 89:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 8}, {Wave: "tri", Level: 0.5}},
			Filter: Filter{Type: "lp", Cutoff: 900, Res: 0.1, Key: 0.3},
			Amp:    adsr(0.4, 1, 0.9, 1.2),
			Drift:  4,
		}
	case 90:
		return &Patch{
			Osc:       []Osc{{Wave: "saw", Unison: 2, Spread: 10}, {Wave: "pulse", Width: 0.4, Level: 0.6}},
			Filter:    Filter{Type: "lp24", Cutoff: 2000, Res: 0.1, Env: 1.5, Key: 0.3},
			FilterEnv: adsr(0.01, 0.6, 0.3, 0.5),
			Amp:       adsr(0.01, 0.5, 0.8, 0.5),
			Drift:     4,
		}
	case 91:
		return &Patch{
			Osc:   []Osc{{Wave: "saw", Unison: 3, Spread: 14}, {Wave: "pulse", Width: 0.4, Octave: -1, Level: 0.3}},
			Amp:   adsr(0.5, 1, 0.85, 1.2),
			Vowel: "o",
			Drift: 4,
		}
	case 92:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 8}},
			Filter: Filter{Type: "lp", Cutoff: 1000, Res: 0.15, Key: 0.3},
			Amp:    adsr(0.6, 1, 0.9, 1.2),
			LFO:    []LFO{{To: "cutoff", Wave: "sine", Hz: 0.3, Depth: 0.4}},
		}
	case 93:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 3.5, Index: 1.2, Decay: 3, Sustain: 0.5}, {Wave: "saw", Level: 0.3}},
			Filter: Filter{Type: "lp", Cutoff: 5000},
			Amp:    adsr(0.3, 1, 0.85, 1.5),
		}
	case 94:
		return &Patch{
			Osc:   []Osc{{Wave: "saw", Unison: 3, Spread: 16}},
			Amp:   adsr(0.6, 1, 0.9, 2),
			Vowel: "o",
		}
	case 95:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 3, Spread: 20}},
			Filter: Filter{Type: "lp24", Cutoff: 600, Res: 0.5},
			Amp:    adsr(0.3, 1, 0.9, 1.5),
			LFO:    []LFO{{To: "cutoff", Wave: "tri", Hz: 0.15, Depth: 2.2}},
		}

	// Effects.
	case 96:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 7.3, Index: 2, Decay: 0.05}},
			Noise:  0.2,
			Filter: Filter{Type: "hp", Cutoff: 1500},
			Amp:    adsr(0.001, 0.8, 0, 0.6),
		}
	case 97:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 10}, {Wave: "fm", Ratio: 2, Index: 0.8, Level: 0.5}},
			Filter: Filter{Type: "lp24", Cutoff: 1400, Res: 0.2},
			Amp:    adsr(0.8, 1, 0.85, 1.5),
			LFO:    []LFO{{To: "cutoff", Wave: "sine", Hz: 0.2, Depth: 0.8}},
		}
	case 98:
		return &Patch{
			Osc: []Osc{{Wave: "fm", Ratio: 5, Index: 2, Decay: 0.5, Sustain: 0.3}, {Wave: "sine", Octave: 1, Level: 0.4}},
			Amp: adsr(0.001, 3, 0.2, 1.5),
		}
	case 99:
		return &Patch{
			Osc:    []Osc{{Wave: "fm", Ratio: 2, Index: 1, Decay: 0.3, Sustain: 0.3}, {Wave: "saw", Unison: 3, Spread: 12, Level: 0.4}},
			Filter: Filter{Type: "lp", Cutoff: 2000},
			Amp:    adsr(0.05, 2, 0.6, 1.5),
		}
	case 100:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 3, Spread: 18, Octave: 1}, {Wave: "sine"}},
			Filter: Filter{Type: "lp", Cutoff: 6000},
			Amp:    adsr(0.2, 1, 0.85, 1.5),
			Gain:   0.7,
		}
	case 101:
		return &Patch{
			Osc:    []Osc{{Wave: "saw", Unison: 2, Spread: 12}},
			Filter: Filter{Type: "lp", Cutoff: 800, Res: 0.6},
			Amp:    adsr(1, 1, 0.9, 1.5),
			LFO:    []LFO{{To: "cutoff", Wave: "random", Hz: 3, Depth: 1.5}},
		}
	case 102:
		return &Patch{
			Osc: []Osc{{Wave: "fm", Ratio: 2, Index: 1, Decay: 0.5, Sustain: 0.4}},
			Amp: adsr(0.01, 1.5, 0.5, 2),
			LFO: []LFO{{To: "amp", Wave: "sine", Hz: 3, Depth: 0.4}},
		}
	case 103:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}},
			Filter: Filter{Type: "lp", Cutoff: 1500, Res: 0.7},
			Amp:    adsr(0.05, 1, 0.8, 1),
			LFO:    []LFO{{To: "cutoff", Wave: "sine", Hz: 5, Depth: 2}, {To: "pitch", Wave: "sine", Hz: 0.5, Depth: 0.2}},
		}

	// Ethnic.
	case 104:
		p := pluck(2.5, 0.95, 0.2, 0)
		p.Bend = &Bend{Semis: 0.3, Time: 0.15}
		return p
	case 105:
		return pluck(0.7, 0.85, 0.9, 0)
	case 106:
		return pluck(0.6, 0.9, 0.6, 0)
	case 107:
		return pluck(1.6, 0.7, 0.4, 0)
	case 108:
		return bell(6, 1, 0.02, 0.9)
	case 109:
		return &Patch{
			Osc:    []Osc{{Wave: "saw"}, {Wave: "pulse", Width: 0.2, Level: 0.5}},
			Filter: Filter{Type: "lp", Cutoff: 3000, Res: 0.2, Key: 0.3},
			Amp:    adsr(0.02, 0.1, 1, 0.05),
			Vowel:  "e",
			Drive:  0.3,
		}
	case 110:
		p := bowed(4500, 0.04)
		p.LFO = []LFO{vibrato(6, 0.15, 0.15)}
		return p
	case 111:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.15}},
			Filter: Filter{Type: "lp", Cutoff: 3500, Key: 0.4},
			Amp:    adsr(0.02, 0.2, 0.9, 0.1),
			LFO:    []LFO{vibrato(6, 0.15, 0.2)},
			Vowel:  "e",
		}

	// Percussive.
	case 112:
		return bell(4.2, 2, 0.1, 1.2)
	case 113:
		return bell(2.4, 1.5, 0.05, 0.5)
	case 114:
		return &Patch{
			Osc: []Osc{{Wave: "fm", Ratio: 2, Index: 1.2, Decay: 0.1, Sustain: 0.1}, {Wave: "sine", Octave: 1, Level: 0.3}},
			Amp: adsr(0.001, 1.2, 0, 0.3),
		}
	case 115:
		p := bell(2.7, 1, 0.01, 0.12)
		p.Amp.Release = 0.05
		return p
	case 116:
		return &Patch{
			Osc:    []Osc{{Wave: "sine", Octave: -1}},
			Noise:  0.4,
			Filter: Filter{Type: "lp", Cutoff: 600},
			Amp:    adsr(0.001, 0.7, 0, 0.5),
			Bend:   &Bend{Semis: 4, Time: 0.05},
		}
	case 117:
		return &Patch{
			Osc:    []Osc{{Wave: "sine"}},
			Noise:  0.1,
			Filter: Filter{Type: "lp", Cutoff: 2000},
			Amp:    adsr(0.001, 0.5, 0, 0.3),
			Bend:   &Bend{Semis: 5, Time: 0.1},
		}
	case 118:
		return &Patch{
			Osc:  []Osc{{Wave: "sine"}, {Wave: "tri", Level: 0.4}},
			Amp:  adsr(0.001, 0.4, 0, 0.2),
			Bend: &Bend{Semis: 12, Time: 0.15},
		}
	case 119:
		return &Patch{
			Noise:  1,
			Filter: Filter{Type: "hp", Cutoff: 3000},
			Amp:    adsr(1.5, 0.02, 0, 0.05),
			Gain:   0.6,
		}

	// Sound effects.
	case 120:
		return &Patch{
			Noise:     0.6,
			Filter:    Filter{Type: "bp", Cutoff: 2500, Res: 0.5, Env: 1},
			FilterEnv: adsr(0.001, 0.05, 0, 0.05),
			Amp:       adsr(0.001, 0.08, 0, 0.05),
		}
	case 121:
		return &Patch{
			Noise:  1,
			Filter: Filter{Type: "bp", Cutoff: 1500, Res: 0.3, Key: 0.5},
			Amp:    adsr(0.1, 0.3, 0.5, 0.3),
			Gain:   0.6,
		}
	case 122:
		return &Patch{
			Noise:  1,
			Filter: Filter{Type: "lp", Cutoff: 800},
			Amp:    adsr(1, 1, 0.9, 2),
			LFO:    []LFO{{To: "cutoff", Wave: "sine", Hz: 0.12, Depth: 2}, {To: "amp", Wave: "sine", Hz: 0.1, Depth: 0.5}},
			Gain:   0.6,
		}
	case 123:
		return &Patch{
			Osc:  []Osc{{Wave: "sine", Octave: 1}},
			Amp:  adsr(0.005, 0.1, 0.8, 0.05),
			LFO:  []LFO{{To: "pitch", Wave: "saw", Hz: 9, Depth: 3}, {To: "amp", Wave: "square", Hz: 9, Depth: 0.6}},
			Bend: &Bend{Semis: 7, Time: 0.05},
		}
	case 124:
		return &Patch{
			Osc: []Osc{{Wave: "fm", Ratio: 3.5, Index: 2.5, Decay: 0.3, Sustain: 0.4}, {Wave: "sine", Octave: -1, Level: 0.4}},
			Amp: adsr(0.001, 0.1, 1, 0.15),
			LFO: []LFO{{To: "amp", Wave: "square", Hz: 20, Depth: 0.9}},
		}
	case 125:
		return &Patch{
			Noise:  1,
			Filter: Filter{Type: "lp", Cutoff: 500, Res: 0.2},
			Amp:    adsr(0.5, 1, 0.9, 1),
			LFO:    []LFO{{To: "amp", Wave: "square", Hz: 12, Depth: 0.8}},
		}
	case 126:
		return &Patch{
			Noise:  1,
			Filter: Filter{Type: "bp", Cutoff: 2000, Res: 0.1},
			Amp:    adsr(0.3, 1, 0.9, 1.5),
			LFO:    []LFO{{To: "amp", Wave: "random", Hz: 30, Depth: 0.6}},
			Gain:   0.6,
		}
	default: // 127, the gunshot.
		return &Patch{
			Noise:     1,
			Filter:    Filter{Type: "lp", Cutoff: 600, Env: 3},
			FilterEnv: adsr(0.001, 0.15, 0, 0.3),
			Amp:       adsr(0.001, 0.5, 0, 0.3),
		}
	}
}

// gmDrum is a drum of General MIDI's percussion key map: its name, as
// the standard gives it, and the drum this engine plays for it.
type gmDrum struct {
	name string
	Drum
}

// gmDrums are General MIDI's percussion, by key, from 27 to 87, with
// the extras of Roland's GS; a key it leaves out is silent.
var gmDrums = map[int]gmDrum{
	27: {"High Q", Drum{Type: "snap", Tone: 1}},
	28: {"Slap", Drum{Type: "clap", Decay: 0.4}},
	31: {"Sticks", Drum{Type: "rim", Tune: 5}},
	32: {"Square Click", Drum{Type: "rim", Tune: 12, Gain: 0.6}},
	33: {"Metronome Click", Drum{Type: "rim", Tune: -3, Gain: 0.7}},
	34: {"Metronome Bell", Drum{Type: "cowbell", Tune: 7, Gain: 0.6}},
	35: {"Acoustic Bass Drum", Drum{Type: "kick", Tune: -2, Decay: 0.8}},
	36: {"Bass Drum 1", Drum{Type: "kick", Decay: 0.7}},
	37: {"Side Stick", Drum{Type: "rim", Gain: 0.8}},
	38: {"Acoustic Snare", Drum{Type: "snare", Tone: 0.4}},
	39: {"Hand Clap", Drum{Type: "clap"}},
	40: {"Electric Snare", Drum{Type: "snare", Tune: 2, Tone: 0.8}},
	41: {"Low Floor Tom", Drum{Type: "tom", Tune: -7, Pan: -0.4}},
	42: {"Closed Hi-Hat", Drum{Type: "hat", Gain: 0.7, Pan: 0.3}},
	43: {"High Floor Tom", Drum{Type: "tom", Tune: -4, Pan: -0.3}},
	44: {"Pedal Hi-Hat", Drum{Type: "hat", Decay: 0.7, Tone: 0.2, Gain: 0.6, Pan: 0.3}},
	45: {"Low Tom", Drum{Type: "tom", Tune: -1, Pan: -0.15}},
	46: {"Open Hi-Hat", Drum{Type: "ohat", Gain: 0.6, Pan: 0.3}},
	47: {"Low-Mid Tom", Drum{Type: "tom", Tune: 2}},
	48: {"Hi-Mid Tom", Drum{Type: "tom", Tune: 5, Pan: 0.15}},
	49: {"Crash Cymbal 1", Drum{Type: "crash", Gain: 0.6, Pan: -0.3}},
	50: {"High Tom", Drum{Type: "tom", Tune: 8, Pan: 0.3}},
	51: {"Ride Cymbal 1", Drum{Type: "ride", Gain: 0.5, Pan: 0.35}},
	52: {"Chinese Cymbal", Drum{Type: "crash", Tune: 3, Tone: 1, Decay: 0.6, Gain: 0.5, Pan: 0.4}},
	53: {"Ride Bell", Drum{Type: "metal", Tune: 7, Decay: 0.6, Gain: 0.4, Pan: 0.35}},
	54: {"Tambourine", Drum{Type: "shaker", Decay: 3, Tone: 1, Gain: 0.6, Pan: 0.2}},
	55: {"Splash Cymbal", Drum{Type: "crash", Tune: 4, Decay: 0.35, Gain: 0.5, Pan: -0.2}},
	56: {"Cowbell", Drum{Type: "cowbell", Gain: 0.6, Pan: 0.2}},
	57: {"Crash Cymbal 2", Drum{Type: "crash", Tune: -2, Tone: 0.8, Decay: 1.2, Gain: 0.6, Pan: 0.3}},
	58: {"Vibraslap", Drum{Type: "shaker", Decay: 8, Tone: 0.3, Gain: 0.5}},
	59: {"Ride Cymbal 2", Drum{Type: "ride", Tune: -2, Tone: 0.3, Gain: 0.5, Pan: -0.35}},
	60: {"Hi Bongo", Drum{Type: "tom", Tune: 20, Decay: 0.3, Pan: 0.3}},
	61: {"Low Bongo", Drum{Type: "tom", Tune: 15, Decay: 0.35, Pan: 0.3}},
	62: {"Mute Hi Conga", Drum{Type: "tom", Tune: 12, Decay: 0.15, Pan: -0.3}},
	63: {"Open Hi Conga", Drum{Type: "tom", Tune: 12, Decay: 0.5, Pan: -0.3}},
	64: {"Low Conga", Drum{Type: "tom", Tune: 7, Decay: 0.5, Pan: -0.3}},
	65: {"High Timbale", Drum{Type: "tom", Tune: 17, Decay: 0.4, Pan: 0.2}},
	66: {"Low Timbale", Drum{Type: "tom", Tune: 12, Decay: 0.45, Pan: 0.2}},
	67: {"High Agogo", Drum{Type: "cowbell", Tune: 7, Gain: 0.5, Pan: -0.3}},
	68: {"Low Agogo", Drum{Type: "cowbell", Tune: 2, Gain: 0.5, Pan: -0.3}},
	69: {"Cabasa", Drum{Type: "shaker", Tone: 0.7, Gain: 0.6, Pan: 0.3}},
	70: {"Maracas", Drum{Type: "shaker", Decay: 0.6, Gain: 0.6, Pan: -0.3}},
	73: {"Short Guiro", Drum{Type: "shaker", Decay: 0.8, Tone: 0.2, Gain: 0.5, Pan: 0.3}},
	74: {"Long Guiro", Drum{Type: "shaker", Decay: 3, Tone: 0.2, Gain: 0.5, Pan: 0.3}},
	75: {"Claves", Drum{Type: "rim", Tune: 7, Gain: 0.7, Pan: -0.2}},
	76: {"Hi Wood Block", Drum{Type: "rim", Tune: 2, Gain: 0.7, Pan: 0.3}},
	77: {"Low Wood Block", Drum{Type: "rim", Tune: -4, Gain: 0.7, Pan: 0.3}},
	78: {"Mute Cuica", Drum{Type: "tom", Tune: 24, Decay: 0.15, Pan: -0.2}},
	79: {"Open Cuica", Drum{Type: "tom", Tune: 20, Decay: 0.3, Pan: -0.2}},
	80: {"Mute Triangle", Drum{Type: "metal", Tune: 24, Decay: 0.15, Gain: 0.4, Pan: 0.4}},
	81: {"Open Triangle", Drum{Type: "metal", Tune: 24, Decay: 1.2, Gain: 0.4, Pan: 0.4}},
	82: {"Shaker", Drum{Type: "shaker", Gain: 0.6, Pan: 0.3}},
	83: {"Jingle Bell", Drum{Type: "shaker", Decay: 2, Tone: 1, Gain: 0.5}},
	85: {"Castanets", Drum{Type: "snap", Gain: 0.7}},
	86: {"Mute Surdo", Drum{Type: "tom", Tune: -10, Decay: 0.3}},
	87: {"Open Surdo", Drum{Type: "tom", Tune: -10, Decay: 1}},
}

// GMDrumName returns the name of General MIDI's percussion on key, or
// "" where none sounds there.
func GMDrumName(key int) string { return gmDrums[key].name }

// GMKit returns General MIDI's drum kit as a drums patch: each drum
// named by its key, as "36" for the bass drum. kit is the drum
// channel's program: 24, Electronic, plays syn-toms, and 25, TR-808, a
// long, booming kick and ringing toms, as Roland's GS kits do; any
// other plays the standard kit.
func GMKit(kit int) *Patch {
	p := &Patch{Kind: "drums", Kit: map[string]Drum{}, Poly: 24}
	for k, d := range gmDrums {
		dr := d.Drum
		switch kit {
		case 24:
			if dr.Type == "tom" && k <= 50 {
				dr.Type = "syntom"
			}
			if k == 38 || k == 40 {
				dr.Tone = 1
			}
		case 25:
			switch {
			case k == 35 || k == 36:
				dr.Decay = 1.6
			case dr.Type == "tom" && k <= 50:
				dr.Decay = 0.6
			case k == 39:
				dr.Decay = 0.8
			}
		}
		p.Kit[strconv.Itoa(k)] = dr
	}
	return p
}
