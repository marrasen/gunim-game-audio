package synth

// The Nintendo chips, as their tunes use them: the NES's 2A03, with its
// pulses of four widths, its stepped triangle and its noise of two
// modes, and the Game Boy's, which adds a channel playing a wave of 32
// steps of its own.

// nesNoise is the NES's noise: a 15-bit shift register, fed back from
// its second bit, or in its short mode its seventh, which loops in 93
// steps and rings with a metallic tone. Its output is one bit.
type nesNoise struct {
	reg   uint16
	out   float32
	step  int
	short bool
}

// at returns the noise at phase ph, in cycles of the oscillator: it
// shifts sixteen times a cycle, so the note pitches it.
func (n *nesNoise) at(ph float32) float32 {
	if n.reg == 0 {
		n.reg, n.step = 1, -1
	}
	if s := int(ph * 16); s != n.step {
		n.step = s
		tap := uint16(1)
		if n.short {
			tap = 6
		}
		bit := (n.reg ^ n.reg>>tap) & 1
		n.reg = n.reg>>1 | bit<<14
		n.out = 1
		if n.reg&1 == 1 {
			n.out = -1
		}
	}
	return n.out
}

// nesDuties are the widths a NES or Game Boy pulse may have.
var nesDuties = []float64{0.125, 0.25, 0.5, 0.75}

// snapDuty returns the NES's width nearest w.
func snapDuty(w float64) float64 {
	best := nesDuties[2]
	for _, d := range nesDuties {
		if abs64(d-w) < abs64(best-w) {
			best = d
		}
	}
	return best
}

// GBWave is the Game Boy wave channel's wave a patch plays where its
// oscillator gives none: 32 steps of 0 to 15, a soft, round wave many
// Game Boy tunes play their bass with.
var GBWave = []int{8, 10, 12, 13, 14, 15, 15, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7}

// stepped returns a cycle of 32 steps of 0 to 15, as a table to play: a
// wave such as the Game Boy's wave channel plays, centred.
func stepped(steps []int) []float32 {
	t := make([]float32, tableSize)
	var sum float64
	for i := range t {
		v := float32(steps[i*len(steps)/tableSize]) / 7.5
		t[i] = v
		sum += float64(v)
	}
	mean := float32(sum / tableSize)
	for i := range t {
		t[i] -= mean
	}
	return t
}

// nesTriangle is the NES's triangle: 32 steps up and down, which gives
// it a buzz a smooth triangle lacks.
var nesTriangle = func() []int {
	s := make([]int, 32)
	for i := range 16 {
		s[i], s[31-i] = 15-i, 15-i
	}
	// Down from 15 to 0, then up again.
	return s
}()

// Chip makes a patch's level move as a console's does: Levels steps of
// it, 16 on the NES and the Game Boy, set Hz times a second, 60 for the
// NES's frames.
type Chip struct {
	Levels int
	Hz     float64
}

// Bend starts a note Semis semitones off, and slides it to the note over
// Time seconds, as a chip tune's blips and drops do.
type Bend struct {
	Semis, Time float64
}

// The waves of a chip drum's frames, after the SID's.
const (
	sfNESNoise = sfTri + 1 + iota
	sfNESMetal
	sfTri4
)

// nesDrums are the NES's drums, and the Game Boy's, a frame each 60th of
// a second: the triangle's falling kick and tom, the noise's snare and
// hats, and its short mode's metallic click.
var nesDrums = map[int][]sidFrame{
	drNESKick: {{sfTri4, 58, 1}, {sfTri4, 51, 1}, {sfTri4, 46, 0.95}, {sfTri4, 42, 0.85}, {sfTri4, 39, 0.7},
		{sfTri4, 37, 0.5}, {sfTri4, 36, 0.3}, {sfTri4, 35, 0.15}},
	drNESSnare: {{sfTri4, 62, 1}, {sfNESNoise, 95, 0.9}, {sfNESNoise, 93, 0.75}, {sfNESNoise, 93, 0.6}, {sfNESNoise, 93, 0.47},
		{sfNESNoise, 93, 0.35}, {sfNESNoise, 93, 0.25}, {sfNESNoise, 93, 0.15}, {sfNESNoise, 93, 0.07}},
	drNESHat:  {{sfNESNoise, 118, 0.6}, {sfNESNoise, 118, 0.3}, {sfNESNoise, 118, 0.1}},
	drNESOHat: {{sfNESNoise, 116, 0.55}, {sfNESNoise, 116, 0.47}, {sfNESNoise, 116, 0.4}, {sfNESNoise, 116, 0.33}, {sfNESNoise, 116, 0.26}, {sfNESNoise, 116, 0.2}, {sfNESNoise, 116, 0.14}, {sfNESNoise, 116, 0.08}},
	drNESTom: {{sfTri4, 56, 1}, {sfTri4, 53, 0.95}, {sfTri4, 51, 0.85}, {sfTri4, 49, 0.7}, {sfTri4, 47, 0.55},
		{sfTri4, 46, 0.4}, {sfTri4, 45, 0.25}, {sfTri4, 44, 0.12}},
	drNESMetal: {{sfNESMetal, 100, 0.8}, {sfNESMetal, 100, 0.5}, {sfNESMetal, 100, 0.25}, {sfNESMetal, 100, 0.1}},
}

// chipDrum returns a chip drum's frames, and how many frames a second.
func chipDrum(kind int) (frames []sidFrame, fps float32) {
	if f, ok := sidDrums[kind]; ok {
		return f, 50
	}
	return nesDrums[kind], 60
}

// isChipDrum reports whether a drum's kind is a chip's, built a frame at
// a time.
func isChipDrum(kind int) bool { return kind >= drSIDKick && kind <= drNESMetal }
