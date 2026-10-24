package synth

import "math"

// cymBands are the pitches of a modelled cymbal's bands, an octave apart.
var cymBands = [...]float32{250, 500, 1000, 2000, 4000, 8000, 16000}

// nBands counts a modelled cymbal's bands.
const nBands = len(cymBands)

// nPartials counts a modelled cymbal's metal partials.
const nPartials = 48

// cymPartials are a modelled cymbal's partials: each one's pitch, spread unevenly in pitch from 300 Hz to 15 kHz, the
// band whose envelope it follows, and how loud it is.
var cymPartials = func() (p [nPartials]struct {
	hz   float32
	band int
	lvl  float32
}) {
	g := newRand(1964)
	for i := range p {
		x := (float64(i) + g.float()) / nPartials
		hz := 300 * math.Pow(50, x)
		p[i].hz = float32(hz)
		p[i].band = min(max(int(math.Round(math.Log2(hz/float64(cymBands[0])))), 0), nBands-1)
		p[i].lvl = float32(0.5 + 0.5*g.float())
	}
	return p
}()

// cymbal is a modelled cymbal's own sound: how loud each of its bands is, how long each takes to swell, and how long
// each rings, in seconds; how loud its metal partials are against its noise; its splash, a burst over all its bands,
// how loud and how long it rings, in seconds; its stick's click, how loud and at what pitch; and how loud it is.
type cymbal struct {
	level, rise, ring  [nBands]float32
	metal              float32
	splash, splashRing float32
	click, clickHz     float32
	gain               float32
}

// cymbals are the modelled cymbals, by kind.
var cymbals = map[int]cymbal{
	drModalHat: {level: [nBands]float32{0.16, 0.22, 0.165, 0.075, 0.273, 0.625, 0.5},
		rise: [nBands]float32{1e-3, 1e-3, 1e-3, 1e-3, 1e-3, 0.0144, 0.5e-3},
		ring: [nBands]float32{0.06, 0.0545, 0.0364, 0.01, 0.032, 0.05, 0.02}, metal: 0.037, gain: 1.36},
	drModalOHat: {level: [nBands]float32{0.02, 0.66, 0.1, 0.273, 0.3, 0.5, 1},
		rise: [nBands]float32{1e-3, 0.125e-3, 1e-3, 1e-3, 1e-3, 1e-3, 3e-3},
		ring: [nBands]float32{0.25, 0.11, 1.5, 0.8, 1.2, 0.3, 0.05}, metal: 0.55, gain: 0.47},
	drModalCrash: {level: [nBands]float32{0.06, 0.25, 0.35, 0.3, 0.5, 0.5, 0.25},
		rise: [nBands]float32{1e-3, 0.167e-3, 2.5e-3, 0.0109, 0.0375, 0.0727, 1e-3},
		ring: [nBands]float32{0.54, 0.6, 0.7, 0.9, 0.8, 0.45, 0.25}, metal: 2, splash: 1, splashRing: 0.06,
		click: 0.3, clickHz: 6000, gain: 0.19},
	drModalRide: {level: [nBands]float32{0.0117, 0.267, 0.1125, 0.2, 0.15, 0.3, 0.675},
		rise: [nBands]float32{1e-3, 0.71e-3, 1e-3, 1e-3, 1e-3, 1e-3, 1e-3},
		ring: [nBands]float32{4.5, 0.45, 0.5, 0.48, 0.4, 0.0667, 0.02}, metal: 1, gain: 0.32},
}

// isCymbal reports whether drum kind k is a modelled cymbal.
func isCymbal(k int) bool { return k >= drModalHat && k <= drModalRide }

// plate is a modelled cymbal sounding: noise split into bands, and partials, each band and the partials in it
// following an envelope that swells and rings.
type plate struct {
	band [nBands]svf
	// Each band's envelope is lvl times a less b, where a keeps ka of itself a frame and b keeps kb.
	a, b, ka, kb, lvl [nBands]float32
	// Each partial turns by c and s a frame.
	re, im, c, s [nPartials]float32
	pl           [nPartials]float32
	metal        float32
	// The splash is as loud as splash, and keeps splashKeep of itself a frame.
	splash, splashKeep float32
	// click filters noise into the stick's click, as loud as clickLevel.
	click      svf
	clickLevel float32
	gain       float32
}

// start strikes the cymbal of hit h, of kind k, tuned by tune, as hard and as bright as vel and tone say, ringing for
// decay times its own length.
func (m *plate) start(h *hit, k int, tune, tone, decay float32) {
	cy := cymbals[k]
	jitter := func(x float32) float32 { return 1 + x*float32(2*h.noise.float()-1) }
	for b := range nBands {
		m.band[b].reset()
		m.band[b].set(cymBands[b]*tune, 0.3)
		// A harder or brighter hit sounds the higher bands louder.
		tilt := float32(math.Pow(float64(h.vel), 0.8*float64(b)/float64(nBands-1))) * exp2(1.5*tone*float32(b-3)/3)
		m.lvl[b] = cy.level[b] * h.vel * tilt * jitter(0.08)
		m.ka[b] = float32(math.Exp(-1 / (float64(cy.ring[b]*decay) * rate)))
		m.kb[b] = float32(math.Exp(-1 / (float64(max(cy.rise[b], 1e-4)) * rate)))
		m.a[b], m.b[b] = 1, 1
	}
	for i, p := range cymPartials {
		w := 2 * math.Pi * float64(p.hz*tune*jitter(0.003)) / rate
		if w > 0.85*math.Pi {
			m.c[i], m.s[i], m.pl[i] = 0, 0, 0
			continue
		}
		m.c[i], m.s[i] = float32(math.Cos(w)), float32(math.Sin(w))
		ph := 2 * math.Pi * h.noise.float()
		m.re[i], m.im[i] = float32(math.Cos(ph)), float32(math.Sin(ph))
		m.pl[i] = p.lvl * jitter(0.1)
	}
	m.metal = cy.metal
	m.splash, m.splashKeep = cy.splash*h.vel, 0
	if cy.splashRing > 0 {
		m.splashKeep = float32(math.Exp(-1 / (float64(cy.splashRing*decay) * rate)))
	}
	m.click.reset()
	m.click.set(cy.clickHz*tune, 0.4)
	m.clickLevel = 20 * cy.click * h.vel * h.vel
	m.gain = cy.gain
}

// renorm keeps each partial turning at an even level.
func (m *plate) renorm() {
	for i := range nPartials {
		r := m.re[i]*m.re[i] + m.im[i]*m.im[i]
		k := (3 - r) / 2
		m.re[i] *= k
		m.im[i] *= k
	}
}

// splashShape is how loud a cymbal's splash is in each band.
var splashShape = [nBands]float32{0.4, 0.7, 1, 1, 1, 1, 1}

// step returns the cymbal's next frame, t frames after the strike, with noise for its bands and its click.
func (m *plate) step(t int64, noise *rng) float32 {
	var env [nBands]float32
	n := noise.bipolar()
	var s float32
	for b := range nBands {
		env[b] = m.lvl[b] * (m.a[b] - m.b[b])
		env[b] += m.splash * splashShape[b]
		m.a[b] *= m.ka[b]
		m.b[b] *= m.kb[b]
		_, bp, _ := m.band[b].step(n)
		s += bp * env[b]
	}
	m.splash *= m.splashKeep
	var p float32
	for i := range nPartials {
		re, im := m.re[i], m.im[i]
		m.re[i] = m.c[i]*re - m.s[i]*im
		m.im[i] = m.s[i]*re + m.c[i]*im
		p += m.im[i] * m.pl[i] * env[cymPartials[i].band]
	}
	s += p * m.metal / 4
	if m.clickLevel > 0 {
		_, bp, _ := m.click.step(noise.bipolar())
		s += bp * m.clickLevel * clickFade[min(t, int64(len(clickFade)-1))]
	}
	return s * m.gain
}

// quiet reports whether every band and the splash have fallen past hearing, -66 dB.
func (m *plate) quiet() bool {
	if m.splash > 0.0005 {
		return false
	}
	for b := range nBands {
		if m.lvl[b]*m.a[b] > 0.0005 {
			return false
		}
	}
	return true
}

// longest returns how long the longest of kind k's bands rings, in seconds.
func longest(k int) float32 {
	var r float32
	for _, x := range cymbals[k].ring {
		r = max(r, x)
	}
	return r
}
