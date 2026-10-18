package plugfx

import "math"

// crusher is a bit crusher, as an old sampler sounds: it holds each
// sample it takes for as long as a slower rate leaves between two, and
// keeps it to fewer bits. A low-pass before the hold, Smooth, takes
// away the highs the slower rate cannot carry, which would otherwise
// fold back down as the old machines' bright aliases.
//
// Both sides are held on the same frames. The dither's noise comes from
// a generator that starts again as the crusher is cleared, so a sound
// crushes the same each time.
type crusher struct {
	sr float64
	// step is how far through a held sample each frame moves, the rate
	// over sr, and phase how far it has moved: a frame takes a new
	// sample where phase has reached 1.
	step, phase float64
	// levels is how many steps the bits leave on each side of zero, and
	// sign says the crusher keeps one bit alone: each sample's sign.
	levels float32
	sign   bool
	dither bool
	smooth float32
	// held is the sample held on each side.
	held [2]float32
	// lp is each side's low-pass, two sections of a fourth-order
	// Butterworth, set at 0.45 of the rate it holds at.
	lp   [2][2]lowPass
	seed uint32
}

// crushSeed is where the dither's noise starts.
const crushSeed = 0x9e3779b9

// newCrusher returns a crusher for sound at sr frames a second: 12 bits
// at 26.04 kHz, an SP-1200, with no smoothing and no dither.
func newCrusher(sr float64) *crusher {
	c := &crusher{sr: sr}
	c.set(12, 26040, 0, false)
	c.clear()
	return c
}

// set sets the bits it keeps, from 1 to 16; the rate it holds samples
// at, in hertz, which at sr or over holds none; how far the low-pass
// takes the highs away before the hold, from 0 to 1; and whether it
// dithers.
func (c *crusher) set(bits int, rate, smooth float64, dither bool) {
	bits = min(max(bits, 1), 16)
	c.sign = bits == 1
	c.levels = float32(int(1) << (bits - 1))
	rate = min(max(rate, 1), c.sr)
	c.step = rate / c.sr
	c.smooth = float32(min(max(smooth, 0), 1))
	c.dither = dither
	cut := 0.45 * rate
	for ch := range c.lp {
		c.lp[ch][0].set(cut, c.sr, 0.5412)
		c.lp[ch][1].set(cut, c.sr, 1.3066)
	}
}

// process crushes the sound in l and r in place.
func (c *crusher) process(l, r []float32) {
	for i := range l {
		x := [2]float32{l[i], r[i]}
		for ch := range x {
			s := c.lp[ch][1].step(c.lp[ch][0].step(x[ch]))
			x[ch] += (s - x[ch]) * c.smooth
		}
		if c.phase >= 1 {
			c.phase -= math.Floor(c.phase)
			c.held = [2]float32{c.quantize(x[0]), c.quantize(x[1])}
		}
		c.phase += c.step
		l[i], r[i] = c.held[0], c.held[1]
	}
	// What silence leaves in the filters is let go of before it is too
	// small for a float to hold quickly.
	for ch := range c.lp {
		c.lp[ch][0].flush()
		c.lp[ch][1].flush()
	}
}

// quantize keeps x to the crusher's bits, within -1 and 1, dithered
// where it dithers.
func (c *crusher) quantize(x float32) float32 {
	if c.sign {
		switch {
		case x > 0:
			return 0.5
		case x < 0:
			return -0.5
		}
		return 0
	}
	v := x * c.levels
	if c.dither {
		// Triangular noise of a step either way, which leaves no trace of
		// the sound in the error it makes.
		v += c.noise() - c.noise()
	}
	v = float32(math.Floor(float64(v) + 0.5))
	return min(max(v, -c.levels), c.levels-1) / c.levels
}

// noise returns the next of the dither's noise, from 0 to 1.
func (c *crusher) noise() float32 {
	c.seed ^= c.seed << 13
	c.seed ^= c.seed >> 17
	c.seed ^= c.seed << 5
	return float32(c.seed>>8) / (1 << 24)
}

// clear forgets the sound held, and starts the noise again.
func (c *crusher) clear() {
	c.phase, c.held, c.seed = 1, [2]float32{}, crushSeed
	for ch := range c.lp {
		c.lp[ch][0].reset()
		c.lp[ch][1].reset()
	}
}

// lowPass is a two-pole low-pass, as RBJ's cookbook gives it.
type lowPass struct {
	b0, b1, b2, a1, a2 float32
	z1, z2             float32
}

// set tunes it to hz, for sound at sr frames a second, q sharp.
func (f *lowPass) set(hz, sr, q float64) {
	w := 2 * math.Pi * min(max(hz, 10), sr*0.45) / sr
	cw, alpha := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + alpha
	f.b0 = float32((1 - cw) / 2 / a0)
	f.b1 = float32((1 - cw) / a0)
	f.b2 = f.b0
	f.a1 = float32(-2 * cw / a0)
	f.a2 = float32((1 - alpha) / a0)
}

func (f *lowPass) step(x float32) float32 {
	y := f.b0*x + f.z1
	f.z1 = f.b1*x - f.a1*y + f.z2
	f.z2 = f.b2*x - f.a2*y
	return y
}

func (f *lowPass) reset() { f.z1, f.z2 = 0, 0 }

// flush lets go of what the filter holds where it is next to nothing.
func (f *lowPass) flush() {
	if f.z1 > -1e-20 && f.z1 < 1e-20 && f.z2 > -1e-20 && f.z2 < 1e-20 {
		f.reset()
	}
}
