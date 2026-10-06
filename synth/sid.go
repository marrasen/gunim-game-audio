package synth

import "math"

// The Commodore 64's sound chip, the SID, as its tunes use it: noise
// pitched by the note, waves made of two at once, a rough filter, and
// drums built a frame at a time.

// lfsr is the SID's noise: a 23-bit shift register, shifted as the
// oscillator runs, eight of its bits read as the sound.
type lfsr struct {
	reg uint32
	out float32
	// step is which sixteenth of a cycle it last shifted in.
	step int
}

func (n *lfsr) reset() { n.reg, n.out, n.step = 0x7ffff8, 0, -1 }

// at returns the noise at phase ph, in cycles of the oscillator: it
// shifts sixteen times a cycle, as the SID's does, so its noise is
// pitched by the note, a hiss high up and a rumble low down.
func (n *lfsr) at(ph float32) float32 {
	if n.reg == 0 {
		n.reset()
	}
	if s := int(ph * 16); s != n.step {
		n.step = s
		bit := ((n.reg >> 22) ^ (n.reg >> 17)) & 1
		n.reg = (n.reg<<1 | bit) & 0x7fffff
		r := n.reg
		// The bits the SID reads out, high to low.
		v := (r>>20&1)<<7 | (r>>18&1)<<6 | (r>>14&1)<<5 | (r>>11&1)<<4 |
			(r>>9&1)<<3 | (r>>5&1)<<2 | (r>>2&1)<<1 | r&1
		n.out = float32(v)/127.5 - 1
	}
	return n.out
}

// tableSize is how many steps a combined wave's cycle has.
const tableSize = 2048

// combined returns a cycle of a combined wave: two of the SID's waves at
// once, as the chip makes them, their bits ANDed, which leaves a thin,
// buzzing wave; it is centred and brought up to full scale. width is a
// pulse's width.
func combined(wave int, width float64) []float32 {
	t := make([]float32, tableSize)
	var sum float64
	var top float32
	for i := range t {
		ph := float64(i) / tableSize
		saw := uint32(ph * 4095)
		tri := uint32((1 - 2*abs64(ph-0.5)) * 4095)
		var pulse uint32
		if ph < width {
			pulse = 4095
		}
		var v uint32
		switch wave {
		case oscSawTri:
			// The triangle's bits are the saw's moved up one.
			v = saw & (tri << 1 & 4095)
		case oscPulseTri:
			v = pulse & tri
		default:
			v = pulse & saw
		}
		t[i] = float32(v) / 4095
		sum += float64(t[i])
	}
	mean := float32(sum / tableSize)
	for i := range t {
		t[i] -= mean
		top = max(top, abs32(t[i]))
	}
	if top > 0 {
		for i := range t {
			t[i] /= top
		}
	}
	return t
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// sidStep runs one sample through a filter set up as the SID's: its
// input driven, so loud chords break up, and its resonance saturating,
// so it squeals rather than rings, returning the mode's sound.
func sidStep(f *svf, x float32, mode int) float32 {
	lp, bp, hp := f.step(softClip(x * 1.25))
	// The 6581's resonance runs out of room, roughly.
	f.ic1 = softClip(f.ic1*0.9) / 0.9
	var y float32
	switch mode {
	case filterSIDLP:
		y = lp
	case filterSIDBP:
		y = bp
	case filterSIDHP:
		y = hp
	default:
		y = lp + hp
	}
	return softClip(y*1.1) / 1.1
}

// sidFrame is a frame of a SID drum: the wave it plays, its note and its
// level.
type sidFrame struct {
	wave int
	note float32
	vol  float32
}

// The waves of a SID drum's frames.
const (
	sfNoise = iota
	sfPulse
	sfTri
)

// sidDrums are the SID's drums, a frame each 50th of a second, as a
// Commodore 64's player switched a voice's wave and pitch frame by
// frame: a burst of noise to strike, then a falling tone.
var sidDrums = map[int][]sidFrame{
	drSIDKick: {{sfNoise, 96, 1}, {sfPulse, 50, 1}, {sfPulse, 44, 0.9}, {sfPulse, 40, 0.8}, {sfTri, 36, 0.7},
		{sfTri, 33, 0.55}, {sfTri, 31, 0.4}, {sfTri, 30, 0.28}, {sfTri, 29, 0.16}, {sfTri, 28, 0.07}},
	drSIDSnare: {{sfNoise, 100, 1}, {sfPulse, 55, 0.9}, {sfNoise, 98, 0.85}, {sfNoise, 96, 0.7}, {sfNoise, 94, 0.55},
		{sfNoise, 92, 0.42}, {sfNoise, 90, 0.3}, {sfNoise, 88, 0.2}, {sfNoise, 86, 0.12}, {sfNoise, 84, 0.05}},
	drSIDClap: {{sfNoise, 98, 1}, {sfNoise, 98, 0.15}, {sfNoise, 98, 0.95}, {sfNoise, 97, 0.15}, {sfNoise, 96, 0.8},
		{sfNoise, 95, 0.55}, {sfNoise, 94, 0.35}, {sfNoise, 93, 0.2}, {sfNoise, 92, 0.08}},
	drSIDHat:  {{sfNoise, 112, 0.75}, {sfNoise, 112, 0.35}, {sfNoise, 112, 0.12}},
	drSIDOHat: {{sfNoise, 110, 0.7}, {sfNoise, 110, 0.6}, {sfNoise, 110, 0.5}, {sfNoise, 110, 0.42}, {sfNoise, 110, 0.34}, {sfNoise, 110, 0.26}, {sfNoise, 110, 0.18}, {sfNoise, 110, 0.12}, {sfNoise, 110, 0.06}},
	drSIDTom: {{sfNoise, 96, 0.8}, {sfTri, 57, 1}, {sfTri, 55, 0.92}, {sfTri, 53, 0.8}, {sfTri, 51, 0.66},
		{sfTri, 49, 0.5}, {sfTri, 48, 0.36}, {sfTri, 47, 0.22}, {sfTri, 46, 0.1}},
	drSIDZap: {{sfPulse, 96, 1}, {sfPulse, 89, 0.92}, {sfPulse, 82, 0.84}, {sfPulse, 75, 0.74}, {sfPulse, 68, 0.62},
		{sfPulse, 61, 0.48}, {sfPulse, 55, 0.34}, {sfPulse, 50, 0.2}, {sfPulse, 46, 0.08}},
}

// isSID reports whether a drum's kind is one of the SID's.
func isSID(kind int) bool { return kind >= drSIDKick && kind <= drSIDZap }

// sidDrum makes the next sample of a SID drum: tune moves its notes,
// decay stretches its frames, and tone narrows its pulse.
func (h *hit) sidDrum(tune, decay, tone float32) float32 {
	frames := sidDrums[h.d.kind]
	frame := float32(rate/50) * decay
	i := int(float32(h.t) / frame)
	if i >= len(frames) {
		h.sidVol *= 0.99
		return 0
	}
	f := frames[i]
	hz := float32(noteHz(float64(f.note))) * tune
	h.ph[0] += hz / rate
	h.ph[0] -= float32(int(h.ph[0]))
	var s float32
	switch f.wave {
	case sfNoise:
		s = h.sid.at(h.ph[0])
	case sfPulse:
		s = -1
		if h.ph[0] < 0.5-0.4*tone {
			s = 1
		}
	default:
		s = 1 - 4*abs32(h.ph[0]-0.5)
	}
	// The level glides a little between frames, as the SID's envelope
	// does, so the steps do not click.
	h.sidVol += (f.vol - h.sidVol) * 0.02
	return s * h.sidVol * 0.8
}

// OscCycle returns a cycle of o's wave as n points, for a tool to draw:
// a combined wave as the SID makes it, and noise as a stretch of it.
func OscCycle(o Osc, n int) []float32 {
	out := make([]float32, n)
	width := o.Width
	if width == 0 {
		width = 0.5
	}
	var noise lfsr
	var tab []float32
	switch o.Wave {
	case "sawtri":
		tab = combined(oscSawTri, width)
	case "pulsetri":
		tab = combined(oscPulseTri, width)
	case "pulsesaw":
		tab = combined(oscPulseSaw, width)
	}
	ratio := o.Ratio
	if ratio == 0 {
		ratio = 1
	}
	for i := range out {
		ph := float64(i) / float64(n)
		var v float64
		switch o.Wave {
		case "square", "pulse":
			v = -1
			if ph < width {
				v = 1
			}
		case "tri", "triangle":
			v = 1 - 4*abs64(ph-0.5)
		case "sine":
			v = math.Sin(2 * math.Pi * ph)
		case "fm":
			v = math.Sin(2*math.Pi*ph + o.Index*math.Sin(2*math.Pi*ratio*ph))
		case "noise":
			// Noise drawn over four of its cycles, as it steps.
			v = float64(noise.at(float32(math.Mod(ph*4, 1))))
		case "sawtri", "pulsetri", "pulsesaw":
			v = float64(tab[int(ph*tableSize)%tableSize])
		default:
			v = 2*ph - 1
		}
		out[i] = float32(v * 0.9)
	}
	return out
}
