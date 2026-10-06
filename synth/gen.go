package synth

import (
	"math"
)

// melodyVariants is how many melodies an evolving Melody writes for each
// progression, to take in turn.
const melodyVariants = 4

// writeMelodies writes m's melodies over each progression: one each, or
// melodyVariants each where m evolves. Each is a slice of notes a bar of
// its progression.
func writeMelodies(m *Melody, k key, progs [][]Chord, chordBars float64, oct int) [][][]mnote {
	variants := 1
	if m.Evolve > 0 {
		variants = melodyVariants
	}
	out := make([][][]mnote, 0, variants*len(progs))
	for pi, prog := range progs {
		for v := range variants {
			out = append(out, writeMelody(m, k, prog, chordBars, oct, m.Seed+uint64(pi)*7349+uint64(v)*104729))
		}
	}
	return out
}

// melodyVariant returns which of a track's melodies plays in song bar
// bar: its progression's, in turn as it evolves.
func (c *compiled) melodyVariant(ct *ctrack, bar int) int {
	variants := 1
	if ct.t.Melody.Evolve > 0 {
		variants = melodyVariants
	}
	prog := 0
	if c.evolve > 0 && len(c.progs) > 1 {
		prog = (bar / (c.phraseBars * c.evolve)) % len(c.progs)
	}
	v := 0
	if variants > 1 {
		v = (bar / (c.phraseBars * ct.t.Melody.Evolve)) % variants
	}
	return (prog*variants + v) % len(ct.mel)
}

// writeMelody writes a melody over prog, chosen by seed. It is built as
// a hook is: a motif's rhythm in the first bar, the same rhythm again in
// the second over its chord, a new one in the third, and the first again
// in the fourth, landing long on a chord tone; and so on in fours.
func writeMelody(m *Melody, k key, prog []Chord, chordBars float64, oct int, seed uint64) [][]mnote {
	bars := max(int(math.Ceil(float64(len(prog))*chordBars-1e-9)), 1)
	density := m.Density
	if density <= 0 {
		density = 0.5
	}
	lo, hi := m.Low, m.High
	if hi <= lo {
		lo, hi = 0, 9
	}
	scale := k
	if m.Penta {
		scale.scale = Scales["majorPenta"]
		if k.scale[2] == 3 {
			scale.scale = Scales["minorPenta"]
		}
	}
	r := newRand(seed)
	rhythms := [2][]float64{rhythm(r, density), rhythm(r, density)}
	out := make([][]mnote, bars)
	deg := (lo + hi) / 2
	dir := 1
	for b := range bars {
		form := b % 4
		rh := rhythms[0]
		if form == 2 {
			rh = rhythms[1]
		}
		// The bar's own choices: a repeat of a motif varies only where
		// its chord makes it.
		pr := newRand(seed ^ uint64(form%2+1)*0x9e37)
		last := b == bars-1 || form == 3
		for i, at := range rh {
			end := 1.0
			if i+1 < len(rh) {
				end = rh[i+1]
			}
			dur := end - at
			if last && i == len(rh)-1 {
				dur = 1 - at
			}
			chord, _ := chordOf(prog, chordBars, float64(b)+at)
			strong := math.Mod(at*4, 1) < 1e-6 && (at == 0 || at == 0.5)
			step := 1
			if pr.float() < 0.25 {
				step = 2
			}
			if pr.float() < 0.2 {
				dir = -dir
			}
			deg += dir * step
			if deg > hi {
				deg, dir = hi-1, -1
			}
			if deg < lo {
				deg, dir = lo+1, 1
			}
			if strong || last && i == len(rh)-1 {
				deg = nearestTone(scale, chord, deg, oct, lo, hi)
			}
			vel := float32(0.82)
			if strong {
				vel = 0.95
			}
			out[b] = append(out[b], mnote{at: at, dur: dur * 0.92, pitch: scale.degree(deg, oct), vel: vel})
		}
	}
	return out
}

// rhythm returns where a bar's notes start, in eighths, or sixteenths
// where it is busy: the downbeat always, the other beats often, and the
// offbeats by density.
func rhythm(r *rng, density float64) []float64 {
	steps := 8
	if density > 0.65 {
		steps = 16
	}
	var out []float64
	for s := range steps {
		at := float64(s) / float64(steps)
		p := density * 0.6
		switch {
		case s == 0:
			p = 1
		case math.Mod(at*4, 1) < 1e-9:
			p = 0.45 + density*0.4
		}
		if r.float() < p {
			out = append(out, at)
		}
	}
	if len(out) < 3 {
		out = []float64{0, 0.375, 0.5}
	}
	return out
}

// chordOf returns the chord of prog at pos, in bars.
func chordOf(prog []Chord, chordBars, pos float64) (ch Chord, index int) {
	i := mod(int(math.Floor(pos/chordBars+1e-9)), len(prog))
	return prog[i], i
}

// nearestTone returns the degree of scale nearest deg, within lo and
// hi, whose note is a tone of chord: deg itself where none is.
func nearestTone(scale key, chord Chord, deg, oct, lo, hi int) int {
	for d := range 5 {
		for _, c := range []int{deg + d, deg - d} {
			if c >= lo && c <= hi && chord.fits(scale.degree(c, oct)) {
				return c
			}
		}
	}
	return deg
}
