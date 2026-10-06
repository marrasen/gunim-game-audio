package calls

import "math"

// rng is a small random source, the same numbers for the same seed, so
// a take is heard again from its seed.
type rng struct{ s uint64 }

func newRNG(seed uint64) *rng { return &rng{s: seed*0x9e3779b97f4a7c15 + 0x2545f4914f6cdd1d} }

// next returns 64 random bits, by splitmix64.
func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	return z ^ z>>31
}

// float returns a number from 0 up to 1.
func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

// noise returns a number from -1 up to 1.
func (r *rng) noise() float64 { return 2*r.float() - 1 }

// norm returns a number of a normal spread, its deviation 1.
func (r *rng) norm() float64 {
	u := max(r.float(), 1e-12)
	return math.Sqrt(-2*math.Log(u)) * math.Cos(2*math.Pi*r.float())
}

// drift is noise that wanders slowly, as a voice's pitch and level do:
// white noise smoothed to change about hz times a second, its
// deviation about 1.
type drift struct {
	r    *rng
	a, y float64
	gain float64
}

func newDrift(r *rng, hz float64) *drift {
	a := 1 - math.Exp(-2*math.Pi*hz/rate)
	// The smoothing narrows the noise; gain brings its deviation back
	// to about 1.
	return &drift{r: r, a: a, gain: math.Sqrt((2 - a) / a)}
}

func (d *drift) step() float64 {
	d.y += d.a * (d.r.norm() - d.y)
	return d.y * d.gain
}

// biquad is a filter of two poles and two zeros, in direct form 1.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *biquad) step(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1, f.y2, f.y1 = f.x1, x, f.y1, y
	return y
}

// highPass returns a Butterworth section passing above hz, its
// sharpness q.
func highPass(hz, q float64) biquad {
	w := 2 * math.Pi * hz / rate
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	return biquad{b0: (1 + cw) / 2 / a0, b1: -(1 + cw) / a0, b2: (1 + cw) / 2 / a0, a1: -2 * cw / a0, a2: (1 - al) / a0}
}

// lowPass returns a Butterworth section passing below hz, its
// sharpness q.
func lowPass(hz, q float64) biquad {
	w := 2 * math.Pi * min(hz, 0.45*rate) / rate
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	return biquad{b0: (1 - cw) / 2 / a0, b1: (1 - cw) / a0, b2: (1 - cw) / 2 / a0, a1: -2 * cw / a0, a2: (1 - al) / a0}
}

// peaking returns a filter lifting a band about hz by db, its sharpness
// q, as RBJ's cookbook gives it.
func peaking(hz, q, db float64) biquad {
	w := 2 * math.Pi * hz / rate
	a := math.Pow(10, db/40)
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al/a
	return biquad{b0: (1 + al*a) / a0, b1: -2 * cw / a0, b2: (1 - al*a) / a0, a1: -2 * cw / a0, a2: (1 - al/a) / a0}
}

// bandPass sets f to pass a band about hz, bw wide, its peak at 1,
// keeping what it holds, so the band glides as a voice's resonances do.
func (f *biquad) bandPass(hz, bw float64) {
	hz = min(max(hz, 20), 0.45*rate)
	w := 2 * math.Pi * hz / rate
	q := max(hz/max(bw, 1), 0.3)
	cw, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	f.b0, f.b1, f.b2, f.a1, f.a2 = al/a0, 0, -al/a0, -2*cw/a0, (1-al)/a0
}

// smooth eases 0 to 1 in and out, and holds outside them.
func smooth(x float64) float64 {
	x = min(max(x, 0), 1)
	return x * x * (3 - 2*x)
}

// semis returns the ratio of n semitones.
func semis(n float64) float64 { return math.Exp2(n / 12) }

// dB returns the gain of n decibels.
func dB(n float64) float64 { return math.Pow(10, n/20) }

// toDB returns g in decibels.
func toDB(g float64) float64 { return 20 * math.Log10(max(g, 1e-9)) }

// room is a small space a call is heard in: a few early echoes and a
// short tail, as a room's walls give.
type room struct {
	combs [4]comb
	aps   [2]allpass
}

type comb struct {
	buf      []float64
	at       int
	fb, damp float64
	z        float64
}

func (c *comb) step(x float64) float64 {
	y := c.buf[c.at]
	c.z += (y - c.z) * c.damp
	c.buf[c.at] = x + c.z*c.fb
	c.at = (c.at + 1) % len(c.buf)
	return y
}

type allpass struct {
	buf []float64
	at  int
}

func (a *allpass) step(x float64) float64 {
	y := a.buf[a.at]
	out := y - 0.5*x
	a.buf[a.at] = x + 0.5*y
	a.at = (a.at + 1) % len(a.buf)
	return out
}

// newRoom returns a room whose tail lasts about size seconds.
func newRoom(size float64) *room {
	r := &room{}
	for i, ms := range [4]float64{23.3, 26.9, 30.1, 33.7} {
		n := int(ms / 1000 * rate)
		// Each comb falls 60 dB over the tail.
		fb := math.Pow(10, -3*ms/1000/max(size, 0.05))
		r.combs[i] = comb{buf: make([]float64, n), fb: fb, damp: 0.45}
	}
	for i, ms := range [2]float64{5.1, 1.7} {
		r.aps[i] = allpass{buf: make([]float64, int(ms/1000*rate))}
	}
	return r
}

func (r *room) step(x float64) float64 {
	var y float64
	for i := range r.combs {
		y += r.combs[i].step(x)
	}
	y /= 4
	for i := range r.aps {
		y = r.aps[i].step(y)
	}
	return y
}
