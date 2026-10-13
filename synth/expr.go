package synth

import "math"

// liveCtl is how an Instrument moves its patch as it plays: by the
// knobs that need more than the patch's own fields rewritten, and by the
// answers to each note's mod wheel and pressure. A song's patch has
// none.
type liveCtl struct {
	// mod and press are the mod wheel and the pressure every note takes
	// on top of its own, from 0 to 1; modR and pressR how the patch
	// answers them.
	mod, press   float32
	modR, pressR Response
	// tune moves every note, in semitones; width moves a pulse's width;
	// fm turns an fm wave's index up by octaves; pan moves the notes
	// between the speakers.
	tune, width, fm, pan float32
	// lfoRate and lfoDepth scale the LFOs' speed and depth, 1 as made.
	lfoRate, lfoDepth float32
	// vowel, where not 0, is the vowel every note sings, whatever the
	// patch's.
	vowel byte
}

// vibratoHz is how fast the controllers' vibrato swings.
const vibratoHz = 5.5

// vowelOrder is the order the controllers move a vowel along.
const vowelOrder = "aeiou"

// shiftVowel returns vowel v moved by steps along a, e, i, o and u,
// stopping at either end.
func shiftVowel(v byte, steps float32) byte {
	i := -1
	for k := range len(vowelOrder) {
		if vowelOrder[k] == v {
			i = k
		}
	}
	if i < 0 || steps == 0 {
		return v
	}
	j := i + int(math.Round(float64(steps)))
	return vowelOrder[min(max(j, 0), len(vowelOrder)-1)]
}

// express works out what the controllers do to the voice this control
// step: the semitones the vibrato swings it by, the octaves it
// brightens by, the factor on its level, and the steps its vowel moves.
func (v *voice) express(lc *liveCtl) (pitch, bright, level, vowel float32) {
	mod := min(max(v.mod+lc.mod, 0), 1)
	press := min(max(v.press+lc.press, 0), 1)
	if mod == 0 && press == 0 {
		return 0, 0, 1, 0
	}
	m, p := &lc.modR, &lc.pressR
	if depth := float32(m.Vibrato)*mod + float32(p.Vibrato)*press; depth != 0 {
		v.vibP += vibratoHz * control / rate
		if v.vibP >= 1 {
			v.vibP--
		}
		pitch = depth * sin1(v.vibP)
	}
	bright = float32(m.Bright)*mod + float32(p.Bright)*press
	level = float32(dbGain(m.Level*float64(mod) + p.Level*float64(press)))
	vowel = float32(m.Vowel)*mod + float32(p.Vowel)*press
	return pitch, bright, level, vowel
}
