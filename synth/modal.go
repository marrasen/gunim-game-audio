package synth

import "math"

// maxModes is how many modes a modelled drum rings with, at most.
const maxModes = 32

// membraneModes are a round drum head's lowest modes: each one's order and the zero of the Bessel function of that
// order, which set its pitch and how strongly a strike at a given radius drives it.
var membraneModes = [...]struct {
	order int
	zero  float64
}{
	{0, 2.4048}, {1, 3.8317}, {2, 5.1356}, {0, 5.5201}, {3, 6.3802}, {1, 7.0156}, {4, 7.5883},
	{2, 8.4172}, {0, 8.6537}, {5, 8.7715}, {3, 9.7610}, {6, 9.9361}, {1, 10.1735},
}

// plateRatios are a cymbal's modes, as multiples of its lowest, spread evenly in pitch over five and a half octaves
// and each moved a little off the even spread, and plateLevels how strongly a strike drives each.
var plateRatios, plateLevels = func() (r, l [maxModes]float32) {
	g := newRand(1964)
	for i := range r {
		r[i] = float32(math.Exp2((float64(i) + 0.8*(g.float()-0.5)) * 5.5 / maxModes))
		l[i] = float32(0.4 + 0.6*g.float())
	}
	return r, l
}()

// The shapes a modelled drum rings as.
const (
	shapeMembrane = iota
	shapePlate
)

// head is a modelled drum's own sound:
//   - its shape, its lowest mode's pitch and how long that rings, in seconds, and how much faster each higher mode
//     dies;
//   - where it is struck, how far a hard hit bends its pitch up and how long that takes to settle, in seconds, how
//     long the stick or beater touches it at Tone 0, in seconds, and how hard it drives the output;
//   - its slap, the many modes above those it rings with, heard as noise: how loud, at what pitch, and how long it
//     rings;
//   - its buzz, noise as loud as its ringing, as a snare's wires or a cymbal's sizzle: how loud, and at what pitch;
//   - and the pitch its sound is highpassed at, 0 for none, and how loud it is, 1 for as loud as it rings.
type head struct {
	shape                                                int
	hz, ring, damp, strike, bend, settle, contact, drive float32
	slap, slapHz, slapRing                               float32
	buzz, buzzHz                                         float32
	hp, gain                                             float32
}

// heads are the modelled drums' heads, by kind.
var heads = map[int]head{
	drModalKick: {hz: 54, ring: 0.045, damp: 4.7, strike: 0.12, bend: 1.7, settle: 0.023, contact: 0.53e-3, drive: 1.8,
		slap: 1.57, slapHz: 1530, slapRing: 0.023, gain: 1},
	drModalTom: {hz: 110, ring: 0.6, damp: 2.5, strike: 0.38, bend: 0.5, settle: 0.06, contact: 1.2e-3, drive: 0.6,
		slap: 0.3, slapHz: 1800, slapRing: 0.02, gain: 1},
	drModalSnare: {hz: 190, ring: 0.22, damp: 2, strike: 0.35, bend: 0.25, settle: 0.02, contact: 0.4e-3, drive: 0.9,
		slap: 0.6, slapHz: 4000, slapRing: 0.008, buzz: 2.2, buzzHz: 5000, hp: 90, gain: 0.8},
	drModalHat: {shape: shapePlate, hz: 420, ring: 0.05, damp: 1.5, strike: 0.6, contact: 0.3e-3, drive: 1.2,
		slap: 0.4, slapHz: 9000, slapRing: 0.004, buzz: 1.2, buzzHz: 9000, hp: 6000, gain: 0.7},
	drModalOHat: {shape: shapePlate, hz: 420, ring: 0.5, damp: 1, strike: 0.6, contact: 0.3e-3, drive: 1.2,
		slap: 0.4, slapHz: 9000, slapRing: 0.006, buzz: 1, buzzHz: 8500, hp: 6000, gain: 0.6},
	drModalCrash: {shape: shapePlate, hz: 360, ring: 1.6, damp: 0.6, strike: 0.7, contact: 0.5e-3, drive: 2,
		slap: 0.5, slapHz: 6000, slapRing: 0.02, buzz: 1.4, buzzHz: 7000, hp: 2500, gain: 0.8},
	drModalRide: {shape: shapePlate, hz: 420, ring: 2.2, damp: 0.8, strike: 0.5, contact: 0.3e-3, drive: 0.7,
		slap: 0.5, slapHz: 9000, slapRing: 0.005, buzz: 0.7, buzzHz: 9000, hp: 1500, gain: 0.45},
}

// isModal reports whether drum kind k is a modelled drum.
func isModal(k int) bool { return k >= drModalKick && k <= drModalRide }

// membrane is a modelled drum sounding: each mode a damped rotation that the stick's strike drives, and the noises
// the strike and the ringing make.
type membrane struct {
	re, im [maxModes]float32
	// c and s turn each mode on a frame and damp it, hz is its pitch before the bend, keep how much of it is left
	// after a frame, and amp how strongly the strike drives it; n counts the modes.
	c, s, hz, keep, amp [maxModes]float32
	n                   int
	// bend is how far the strike's stretch raises the drum's pitch now, settle how much of it is left after a
	// control step, pulse how many frames the stick touches the drum, and drive how hard it drives the output.
	bend, settle float32
	pulse        int64
	drive        float32
	// slap filters noise into the drum's slap, which is as loud as level and keeps fade of itself a frame.
	slap        svf
	level, fade float32
	// buzz filters noise into the buzz, as loud as buzzLevel times follow, which follows how loud the drum rings.
	buzz              svf
	buzzLevel, follow float32
	// hp highpasses the sound, where hasHP says, and gain sets how loud it is.
	hp    svf
	hasHP bool
	gain  float32
}

// start strikes the drum of hit h, of kind k, tuned by tune, as hard and as bright as vel and tone say, ringing for
// decay times its own length.
func (m *membrane) start(h *hit, k int, tune, tone, decay float32) {
	hd := heads[k]
	jitter := func(x float32) float32 { return 1 + x*float32(2*h.noise.float()-1) }
	strike := hd.strike
	if h.d.Strike > 0 {
		strike = float32(min(h.d.Strike, 1))
	}
	strike = min(strike*jitter(0.1), 1)
	m.bend = hd.bend
	if h.d.Bend > 0 {
		m.bend = float32(h.d.Bend)
	}
	m.bend *= h.vel * h.vel
	m.settle = float32(math.Exp(-control / (float64(hd.settle) * rate)))
	m.drive = hd.drive
	var top float32
	if hd.shape == shapePlate {
		// A cymbal struck near its bell rings brighter, and near its edge darker.
		m.n = maxModes
		tilt := 1.5 * (0.6 - strike)
		for i, r := range plateRatios {
			m.hz[i] = hd.hz * tune * r * jitter(0.002)
			tau := hd.ring * decay / (1 + hd.damp*float32(math.Log2(float64(r))))
			m.keep[i] = float32(math.Exp(-1 / (float64(tau) * rate)))
			m.amp[i] = plateLevels[i] * float32(math.Pow(float64(r), float64(tilt))) * jitter(0.05)
			top = max(top, m.amp[i])
		}
	} else {
		m.n = len(membraneModes)
		for i, md := range membraneModes {
			ratio := float32(md.zero / membraneModes[0].zero)
			m.hz[i] = hd.hz * tune * ratio * jitter(0.003)
			tau := hd.ring * decay / (1 + hd.damp*(ratio-1))
			m.keep[i] = float32(math.Exp(-1 / (float64(tau) * rate)))
			m.amp[i] = float32(math.Abs(math.Jn(md.order, md.zero*float64(strike)))) * jitter(0.05)
			top = max(top, m.amp[i])
		}
	}
	for i := range m.n {
		m.amp[i] /= top
	}
	contact := hd.contact * (1 - 0.8*tone) * (1.4 - 0.7*h.vel)
	m.pulse = max(int64(contact*rate), 2)
	bright := (0.7 + 0.6*tone) * (0.8 + 0.4*h.vel)
	m.slap.reset()
	m.slap.set(hd.slapHz*bright, 0.3)
	m.level = hd.slap * h.vel
	m.fade = float32(math.Exp(-1 / (float64(hd.slapRing) * rate)))
	m.buzz.reset()
	m.buzz.set(hd.buzzHz*bright, 0.2)
	m.buzzLevel, m.follow = hd.buzz, 0
	m.hp.reset()
	m.gain = hd.gain
	m.hasHP = hd.hp > 0
	if m.hasHP {
		m.hp.set(hd.hp, 0.1)
	}
	m.re, m.im = [maxModes]float32{}, [maxModes]float32{}
	m.retune()
}

// retune sets each mode's turn for the drum's pitch now, raised by the strike's stretch as it settles. Modes past
// hearing are let go.
func (m *membrane) retune() {
	up := 1 + m.bend
	m.bend *= m.settle
	for i := range m.n {
		w := 2 * math.Pi * float64(m.hz[i]*up) / rate
		if w > 0.85*math.Pi {
			m.c[i], m.s[i] = 0, 0
			continue
		}
		m.c[i] = m.keep[i] * float32(math.Cos(w))
		m.s[i] = m.keep[i] * float32(math.Sin(w))
		// Denormal numbers slow the processor down a hundredfold, so a mode past hearing stops at 0.
		if abs32(m.re[i])+abs32(m.im[i]) < 1e-9 {
			m.re[i], m.im[i] = 0, 0
		}
	}
}

// followFall is how much of how loud the drum rang the buzz keeps a frame, as a snare's wires settle in 5 ms.
var followFall = float32(math.Exp(-1 / (0.005 * rate)))

// step returns the drum's next frame, t frames after the strike, with noise for its slap and its buzz.
func (m *membrane) step(t int64, noise *rng) float32 {
	var in float32
	if t < m.pulse {
		// A raised cosine, its area 1.
		x := sin1(float32(t) / float32(2*m.pulse))
		in = 2 * x * x / float32(m.pulse)
	}
	var s float32
	for i := range m.n {
		re, im := m.re[i], m.im[i]
		m.re[i] = m.c[i]*re - m.s[i]*im + in*m.amp[i]
		m.im[i] = m.s[i]*re + m.c[i]*im
		s += m.im[i]
	}
	s = softClip(s * m.drive)
	if m.buzzLevel > 0 {
		m.follow = max(m.follow*followFall, abs32(s))
		_, bp, _ := m.buzz.step(noise.bipolar())
		s += bp * m.follow * m.buzzLevel
	}
	_, bp, _ := m.slap.step(noise.bipolar())
	s += bp * m.level
	m.level *= m.fade
	if m.hasHP {
		_, _, s = m.hp.step(s)
	}
	return s * m.gain
}

// quiet reports whether every mode has fallen past hearing, -54 dB.
func (m *membrane) quiet() bool {
	for i := range m.n {
		if abs32(m.re[i])+abs32(m.im[i]) > 0.002 {
			return false
		}
	}
	return true
}
