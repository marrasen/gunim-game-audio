package synth

import "math"

// circuit is the state a circuit-modelled drum needs past a hit's own: two more filters and three more phases.
type circuit struct {
	f2, f3 svf
	ph     [3]float32
}

// isCircuit reports whether drum kind k is modelled on a drum machine's circuits, its cymbals aside.
func isCircuit(k int) bool { return k >= drTR808Kick && k <= drSDSTom && !isCymbal(k) }

// pulseLen is how long a drum machine's trigger pulse lasts, 1 ms.
const pulseLen = rate / 1000

// adv steps phase p a frame at hz and returns it.
func adv(p *float32, hz float32) float32 {
	*p += hz / rate
	*p -= float32(int(*p))
	return *p
}

// tri is a triangle wave at phase p, rising from 0 as a sine does.
func tri(p float32) float32 {
	p += 0.25
	p -= float32(int(p))
	return 1 - 4*abs32(p-0.5)
}

// blep smooths a wave's step at phase p, stepping dt a frame, so it does not alias.
func blep(p, dt float32) float32 {
	if p < dt {
		p /= dt
		return p + p - p*p - 1
	}
	if p > 1-dt {
		p = (p - 1) / dt
		return p*p + p + p + 1
	}
	return 0
}

// square is a square wave at phase p, stepping dt a frame, without aliasing.
func square(p, dt float32) float32 {
	v := float32(1)
	if p >= 0.5 {
		v = -1
	}
	q := p + 0.5
	q -= float32(int(q))
	return v + blep(p, dt) - blep(q, dt)
}

// metal808 steps the six squares a TR-808 sounds its hats and cymbal with, at their frequencies times mul, and returns
// their sum, from about -1 to 1.
func (h *hit) metal808(mul float32) float32 {
	var m float32
	for k, f := range metal {
		dt := f * mul / rate
		m += square(adv(&h.ph[k], f*mul), dt)
	}
	return m / 6
}

// circuitStart readies a circuit-modelled drum, tuned by tune, as bright as tone says, ringing for decay times its own
// length.
func (h *hit) circuitStart(tune, tone, decay float32) {
	c := &h.cc
	secs := func(s float32) int64 { return int64(s * rate) }
	h.ph[0], h.ph[1], c.ph = 0, 0, [3]float32{}
	switch h.d.kind {
	case drTR808Kick:
		// The pitch bump of the pulse, the boom, and the click.
		h.decay(0, 0.012)
		h.decay(1, 0.36*decay)
		h.f1.set(250+3500*tone, 0)
		h.end = secs(3 * decay)
	case drTR808Snare:
		h.decay(0, 0.06)
		h.decay(1, 0.04)
		h.decay(2, 0.14*decay)
		h.decay(3, 0.006)
		h.f1.set(1800, 0)
		h.end = secs(1.2 * decay)
	case drTR808Tom, drTR909Tom, drSDSTom:
		tc := toms[h.d.kind]
		h.decay(0, tc.bendRing)
		h.decay(1, tc.ring*decay/float32(math.Sqrt(float64(tune))))
		h.decay(2, tc.noiseRing)
		h.decay(3, 0.0012)
		h.decay(4, tc.sweepRing)
		h.f1.set(tc.noiseHz*(0.7+0.6*tone), 0)
		h.end = secs(tc.ring * 9 * decay / float32(math.Sqrt(float64(tune))))
	case drTR808Conga:
		h.decay(0, 0.01)
		h.decay(1, 0.13*decay)
		h.end = secs(1 * decay)
	case drTR808Rim:
		h.decay(0, 0.009)
		h.decay(1, 0.005)
		h.f1.set(900, 0)
		h.end = secs(0.15)
	case drTR808Clap:
		h.f1.set(1000*tune*(0.85+0.3*tone), 0.45)
		h.decay(1, 0.12*decay)
		h.end = secs(1 * decay)
	case drTR808Clave:
		h.decay(0, 0.017)
		h.end = secs(0.2)
	case drTR808Maracas:
		h.f1.set(5500, 0)
		h.decay(0, 0.014*decay)
		h.end = secs(0.2 * decay)
	case drTR808Cowbell:
		h.f1.set(1300*tune, 0.3)
		c.f2.set(350, 0)
		h.decay(0, 0.005)
		h.decay(1, 0.09*decay)
		h.end = secs(0.8 * decay)
	case drTR808Hat, drTR808OHat:
		h.f1.set(7100*(0.9+0.3*tone), 0.3)
		c.f2.set(6500, 0)
		length := float32(0.035)
		if h.d.kind == drTR808OHat {
			length = 0.28
		}
		h.decay(0, length*decay)
		h.end = secs(length * 8 * decay)
	case drTR808Cymbal:
		h.f1.set(3440, 0.3)
		c.f2.set(7100, 0.3)
		c.f3.set(9500, 0)
		h.decay(0, 0.6*decay)
		h.decay(1, 0.3*decay)
		h.decay(2, 0.09*decay)
		h.end = secs(5 * decay)
	case drTR909Kick:
		// The pitch's sweep, the body, and the attack's click.
		h.decay(0, 0.014)
		h.decay(1, 0.24*decay)
		h.decay(2, 0.0025)
		h.f1.set(4500, 0)
		h.end = secs(2 * decay)
	case drTR909Snare:
		h.decay(0, 0.012)
		h.decay(1, 0.05)
		h.decay(2, 0.035)
		h.decay(3, 0.12*decay)
		h.f1.set(4500+6000*tone, 0)
		c.f2.set(600, 0)
		h.end = secs(1.2 * decay)
	case drTR909Rim:
		h.decay(0, 0.007)
		h.decay(1, 0.004)
		h.decay(2, 0.002)
		h.f1.set(1100, 0)
		h.end = secs(0.15)
	case drTR909Clap:
		h.f1.set(1200*tune*(0.85+0.3*tone), 0.4)
		c.f2.set(500, 0)
		h.decay(1, 0.2*decay)
		h.end = secs(1.4 * decay)
	case drSDSKick:
		// The bend, the body, the noise, and the stick's click.
		h.decay(0, 0.045*(1.5-0.5*h.vel))
		h.decay(1, 0.3*decay)
		h.decay(2, 0.025)
		h.decay(3, 0.0012)
		h.f1.set(1500+3000*tone, 0.3)
		h.end = secs(2.4 * decay)
	case drSDSSnare:
		h.decay(0, 0.05)
		h.decay(1, 0.13*decay)
		h.decay(2, 0.2*decay)
		h.decay(3, 0.0012)
		h.f1.set(2500+5000*tone, 0.35)
		c.f2.set(400, 0)
		h.end = secs(1.6 * decay)
	}
}

// clapBurst is how far apart a clap's bursts are, and how many it has, by kind.
var clapBurst = map[int]struct {
	gap   int64
	count int64
}{drTR808Clap: {rate * 11 / 1000, 3}, drTR909Clap: {rate * 9 / 1000, 4}}

// circuitStep returns a circuit-modelled drum's next frame, tuned by tune, as bright as tone says.
func (h *hit) circuitStep(tune, tone float32) float32 {
	c := &h.cc
	e := &h.env
	pulse := float32(0)
	if h.t < pulseLen {
		pulse = 1
	}
	switch h.d.kind {
	case drTR808Kick:
		s := sin1(adv(&h.ph[0], 49*tune*(1+0.4*e[0]))) * e[1]
		lp, _, _ := h.f1.step(pulse)
		return softClip(s*1.2 + lp*0.6*(0.3+tone))
	case drTR808Snare:
		f := tune * (1 + 0.08*e[3])
		body := sin1(adv(&h.ph[0], 180*f))*e[0]*(1-0.5*tone) + sin1(adv(&h.ph[1], 330*f))*e[1]*(0.5+0.5*tone)
		_, _, hp := h.f1.step(h.noise.bipolar())
		return softClip(body*0.9 + hp*e[2]*0.75)
	case drTR808Tom, drTR909Tom, drSDSTom:
		return h.tomStep(toms[h.d.kind], tune, tone)
	case drTR808Conga:
		return softClip(sin1(adv(&h.ph[0], 250*tune*(1+0.08*e[0]))) * e[1] * 1.1)
	case drTR808Rim:
		s := sin1(adv(&h.ph[0], 455*tune))*e[0] + 0.7*sin1(adv(&h.ph[1], 1667*tune))*e[1]
		_, _, hp := h.f1.step(s)
		return softClip(hp*3) * 0.6
	case drTR808Clap, drTR909Clap:
		// Bursts of noise a few milliseconds apart, as the hands clap, then a tail of noise as their echo.
		b := clapBurst[h.d.kind]
		if h.t < b.count*b.gap && h.t%b.gap == 0 {
			h.decay(0, 0.0035)
		}
		a := e[0]
		if h.t < b.count*b.gap {
			e[1] = 1
		} else {
			a = max(a, 0.6*e[1])
		}
		_, bp, _ := h.f1.step(h.noise.bipolar())
		if h.d.kind == drTR909Clap {
			_, _, bp = c.f2.step(bp)
		}
		return softClip(bp * a * 3.2)
	case drTR808Clave:
		return sin1(adv(&h.ph[0], 2500*tune)) * e[0] * 0.8
	case drTR808Maracas:
		a := e[0]
		if rise := float32(rate * 2 / 1000); float32(h.t) < rise {
			a = float32(h.t) / rise
			e[0] = 1
		}
		_, _, hp := h.f1.step(h.noise.bipolar())
		return hp * a
	case drTR808Cowbell:
		var m float32
		for k, f := range [2]float32{540, 800} {
			m += square(adv(&h.ph[k], f*tune), f*tune/rate)
		}
		_, bp, _ := h.f1.step(m * 0.5)
		_, _, hp := c.f2.step(bp*1.4 + m*0.15)
		return hp * (0.65*e[0] + 0.35*e[1])
	case drTR808Hat, drTR808OHat:
		_, bp, _ := h.f1.step(h.metal808(tune))
		_, _, hp := c.f2.step(bp)
		return hp * e[0] * 8.5
	case drTR808Cymbal:
		m := h.metal808(tune)
		_, lo, _ := h.f1.step(m)
		_, mid, _ := c.f2.step(m)
		_, _, hi := c.f3.step(m)
		return (lo*e[0]*(1-0.5*tone) + mid*e[1] + hi*e[2]*(0.6+0.8*tone)) * 1.3
	case drTR909Kick:
		// A triangle bent towards a sine, sweeping down fast from about four times its pitch.
		p := adv(&h.ph[0], 52*tune*(1+3.2*e[0]))
		s := (0.75*sin1(p) + 0.25*tri(p)) * e[1]
		lp, _, _ := h.f1.step(h.noise.bipolar()*0.5 + pulse)
		return softClip(s*1.5 + lp*e[2]*(0.3+0.9*tone))
	case drTR909Snare:
		f := tune * (1 + 0.25*e[0])
		body := tri(adv(&h.ph[0], 180*f))*e[1] + 0.6*tri(adv(&h.ph[1], 330*f))*e[2]
		lp, _, _ := h.f1.step(h.noise.bipolar())
		_, _, hp := c.f2.step(lp)
		return softClip(body*0.8 + hp*(e[3]*0.8+e[1]*0.3))
	case drTR909Rim:
		s := sin1(adv(&h.ph[0], 500*tune))*e[0] + 0.8*sin1(adv(&h.ph[1], 1800*tune))*e[1] + h.noise.bipolar()*e[2]*0.4
		_, _, hp := h.f1.step(s)
		return softClip(hp*3.5) * 0.6
	case drSDSKick:
		bend := 2.5 * h.vel
		s := sin1(adv(&h.ph[0], 55*tune*(1+bend*e[0]))) * e[1]
		lp, _, _ := h.f1.step(h.noise.bipolar())
		return softClip(s*1.5 + lp*e[2]*0.4 + h.noise.bipolar()*e[3]*0.8)
	case drSDSSnare:
		s := tri(adv(&h.ph[0], 180*tune*(1+0.8*h.vel*e[0]))) * e[1]
		lp, _, _ := h.f1.step(h.noise.bipolar())
		_, _, hp := c.f2.step(lp)
		return softClip(s*0.8 + hp*e[2]*1.1 + h.noise.bipolar()*e[3]*0.7)
	}
	return 0
}

// tomCircuit is a circuit-modelled tom's sound:
//   - its pitch, how far above it a hard hit starts, and how long that bend takes to fall, in seconds;
//   - how long its body rings at its own pitch, in seconds, its wave, from 0 for a sine to 1 for a triangle, and how
//     hard that drives the output;
//   - its noise, through a lowpass: how loud, at what pitch, and how long it rings;
//   - its click, the trigger heard as a burst of noise, how loud;
//   - a lowpass over body and noise, at cutoff times its pitch, opened sweep times further by a hit and closing over
//     sweepRing seconds, 0 for none;
//   - and how loud it is.
type tomCircuit struct {
	hz, bend, bendRing        float32
	ring, wave, drive         float32
	noise, noiseHz, noiseRing float32
	click                     float32
	cutoff, sweep, sweepRing  float32
	gain                      float32
}

// toms are the circuit-modelled toms, by kind.
var toms = map[int]tomCircuit{
	drTR808Tom: {hz: 130, bend: 0.03, bendRing: 0.006, ring: 0.065, drive: 1.3, noise: 0.5, noiseHz: 3000, noiseRing: 0.02,
		click: 0.5, gain: 0.9},
	drTR909Tom: {hz: 150, bend: 0.5, bendRing: 0.035, ring: 0.07, wave: 0.6, drive: 1.6, noise: 2.2, noiseHz: 14000,
		noiseRing: 0.025, click: 1.5, gain: 0.9},
	drSDSTom: {hz: 120, bend: 0.5, bendRing: 0.08, ring: 0.13, wave: 1, drive: 1.6, noise: 3, noiseHz: 14000,
		noiseRing: 0.045, click: 1.5, cutoff: 30, sweep: 3, sweepRing: 0.04, gain: 0.9},
}

// tomStep returns the next frame of a tom of sound tc, tuned by tune, as bright as tone says.
func (h *hit) tomStep(tc tomCircuit, tune, tone float32) float32 {
	c, e := &h.cc, &h.env
	hz := tc.hz * tune
	p := adv(&h.ph[0], hz*(1+tc.bend*(0.4+0.6*h.vel)*e[0]))
	body := softClip(((1-tc.wave)*sin1(p)+tc.wave*tri(p))*tc.drive) * e[1]
	lp, _, _ := h.f1.step(h.noise.bipolar())
	s := body + lp*e[2]*tc.noise*h.vel
	if tc.cutoff > 0 {
		if h.t%control == 0 {
			c.f2.set(hz*tc.cutoff*(0.7+0.6*tone)*(1+tc.sweep*h.vel*e[4]), 0.2)
		}
		s, _, _ = c.f2.step(s)
	}
	s += h.noise.bipolar() * e[3] * tc.click * h.vel
	return softClip(s) * tc.gain
}
