package synth

import (
	"math"
)

// glottalTable is a cycle of a voice's source: the flow of air through
// the vocal folds as Rosenberg modelled it, opening over 40% of the
// cycle and shutting over 16%, taken as its rate of change, which is
// what the ear hears of it; its spectrum falls as a voice's does.
var glottalTable = func() []float32 {
	const open, shut = 0.4, 0.16
	flow := func(ph float64) float64 {
		switch {
		case ph < open:
			return 0.5 * (1 - math.Cos(math.Pi*ph/open))
		case ph < open+shut:
			return math.Cos(math.Pi * (ph - open) / (2 * shut))
		}
		return 0
	}
	t := make([]float32, tableSize)
	var sum, peak float64
	for i := range t {
		ph := float64(i) / tableSize
		d := (flow(ph+0.5/tableSize) - flow(ph-0.5/tableSize)) * tableSize
		t[i] = float32(d)
		sum += d
	}
	mean := sum / tableSize
	for i := range t {
		t[i] -= float32(mean)
		peak = max(peak, math.Abs(float64(t[i])))
	}
	for i := range t {
		t[i] /= float32(peak)
	}
	return t
}()

// A singerVowel is a voice type's vowel: its five formants' centres, in
// hertz, levels, in decibels, and widths, in hertz.
type singerVowel struct {
	hz, db, bw [5]float32
}

// singers are the voice types' vowels, as the tables of singing
// formants give them.
var singers = map[string]map[byte]singerVowel{
	"bass": {
		'a': {[5]float32{600, 1040, 2250, 2450, 2750}, [5]float32{0, -7, -9, -9, -20}, [5]float32{60, 70, 110, 120, 130}},
		'e': {[5]float32{400, 1620, 2400, 2800, 3100}, [5]float32{0, -12, -9, -12, -18}, [5]float32{40, 80, 100, 120, 120}},
		'i': {[5]float32{250, 1750, 2600, 3050, 3340}, [5]float32{0, -30, -16, -22, -28}, [5]float32{60, 90, 100, 120, 120}},
		'o': {[5]float32{400, 750, 2400, 2600, 2900}, [5]float32{0, -11, -21, -20, -40}, [5]float32{40, 80, 100, 120, 120}},
		'u': {[5]float32{350, 600, 2400, 2675, 2950}, [5]float32{0, -20, -32, -28, -36}, [5]float32{40, 80, 100, 120, 120}},
	},
	"tenor": {
		'a': {[5]float32{650, 1080, 2650, 2900, 3250}, [5]float32{0, -6, -7, -8, -22}, [5]float32{80, 90, 120, 130, 140}},
		'e': {[5]float32{400, 1700, 2600, 3200, 3580}, [5]float32{0, -14, -12, -14, -20}, [5]float32{70, 80, 100, 120, 120}},
		'i': {[5]float32{290, 1870, 2800, 3250, 3540}, [5]float32{0, -15, -18, -20, -30}, [5]float32{40, 90, 100, 120, 120}},
		'o': {[5]float32{400, 800, 2600, 2800, 3000}, [5]float32{0, -10, -12, -12, -26}, [5]float32{40, 80, 100, 120, 120}},
		'u': {[5]float32{350, 600, 2700, 2900, 3300}, [5]float32{0, -20, -17, -14, -26}, [5]float32{40, 60, 100, 120, 120}},
	},
	"alto": {
		'a': {[5]float32{800, 1150, 2800, 3500, 4950}, [5]float32{0, -4, -20, -36, -60}, [5]float32{80, 90, 120, 130, 140}},
		'e': {[5]float32{400, 1600, 2700, 3300, 4950}, [5]float32{0, -24, -30, -35, -60}, [5]float32{60, 80, 120, 150, 200}},
		'i': {[5]float32{350, 1700, 2700, 3700, 4950}, [5]float32{0, -20, -30, -36, -60}, [5]float32{50, 100, 120, 150, 200}},
		'o': {[5]float32{450, 800, 2830, 3500, 4950}, [5]float32{0, -9, -16, -28, -55}, [5]float32{70, 80, 100, 130, 135}},
		'u': {[5]float32{325, 700, 2530, 3500, 4950}, [5]float32{0, -12, -30, -40, -64}, [5]float32{50, 60, 170, 180, 200}},
	},
	"soprano": {
		'a': {[5]float32{800, 1150, 2900, 3900, 4950}, [5]float32{0, -6, -32, -20, -50}, [5]float32{80, 90, 120, 130, 140}},
		'e': {[5]float32{350, 2000, 2800, 3600, 4950}, [5]float32{0, -20, -15, -40, -56}, [5]float32{60, 100, 120, 150, 200}},
		'i': {[5]float32{270, 2140, 2950, 3900, 4950}, [5]float32{0, -12, -26, -26, -44}, [5]float32{60, 90, 100, 120, 120}},
		'o': {[5]float32{450, 800, 2830, 3800, 4950}, [5]float32{0, -11, -22, -22, -50}, [5]float32{40, 80, 100, 120, 120}},
		'u': {[5]float32{325, 700, 2700, 3800, 4950}, [5]float32{0, -16, -35, -40, -60}, [5]float32{50, 60, 170, 180, 200}},
	},
}

func init() {
	// A baritone sings between a bass and a tenor.
	bar := map[byte]singerVowel{}
	for v, b := range singers["bass"] {
		t := singers["tenor"][v]
		var s singerVowel
		for i := range 5 {
			s.hz[i] = (b.hz[i] + t.hz[i]) / 2
			s.db[i] = (b.db[i] + t.db[i]) / 2
			s.bw[i] = (b.bw[i] + t.bw[i]) / 2
		}
		bar[v] = s
	}
	singers["baritone"] = bar
}

// singer is a voice's mouth: five resonances, each a bandpass as wide as
// its formant, set to a voice type's vowel and gliding to the next as
// the mouth moves, so a source sings it.
type singer struct {
	// name is the voice's type, as baritone, and kind its vowels.
	name  string
	kind  map[byte]singerVowel
	f     [5]svf
	hz    [5]float32
	gain  [5]float32
	bw    [5]float32
	to    singerVowel
	ready bool
}

// set aims the mouth at vowel v.
func (s *singer) set(v byte) {
	sv, ok := s.kind[v]
	if !ok {
		sv = s.kind['a']
	}
	s.to = sv
	if !s.ready {
		for i := range 5 {
			s.hz[i], s.bw[i] = sv.hz[i], sv.bw[i]
			s.gain[i] = float32(dbGain(float64(sv.db[i])))
		}
		s.ready = true
		s.retune()
	}
}

// tune moves the mouth a control block's way toward its vowel, as a
// singer's moves, in about 20 ms.
func (s *singer) tune() {
	const k = 0.04
	moved := false
	for i := range 5 {
		g := float32(dbGain(float64(s.to.db[i])))
		if d := s.to.hz[i] - s.hz[i]; d > 0.5 || d < -0.5 {
			s.hz[i] += d * k
			s.bw[i] += (s.to.bw[i] - s.bw[i]) * k
			moved = true
		}
		s.gain[i] += (g - s.gain[i]) * k
	}
	if moved {
		s.retune()
	}
}

func (s *singer) retune() {
	for i := range 5 {
		// A bandpass bw wide at hz: its resonance is 1 less its width
		// over its centre, as the filter reads it.
		k := min(s.bw[i]/s.hz[i], 2)
		s.f[i].set(s.hz[i], (2-k)/1.96)
	}
}

// step returns x sung through the mouth.
func (s *singer) step(x float32) float32 {
	var y float32
	for i := range s.f {
		_, bp, _ := s.f[i].step(x)
		y += bp * s.f[i].k * s.gain[i]
	}
	return y * singerGain
}

// singerGain makes a glottal source as loud through the mouth as a saw
// through a voice's three formants.
const singerGain = 6
