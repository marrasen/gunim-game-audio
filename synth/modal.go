package synth

import "math"

// maxModes is how many modes a modelled drum rings with, at most.
const maxModes = len(membraneModes)

// membraneModes are a round drum head's lowest modes: each one's order and the zero of the Bessel function of that
// order, which set its pitch and how strongly a strike at a given radius drives it.
var membraneModes = [...]struct {
	order int
	zero  float64
}{
	{0, 2.4048}, {1, 3.8317}, {2, 5.1356}, {0, 5.5201}, {3, 6.3802}, {1, 7.0156}, {4, 7.5883},
	{2, 8.4172}, {0, 8.6537}, {5, 8.7715}, {3, 9.7610}, {6, 9.9361}, {1, 10.1735},
}

// head is a modelled drum's own sound:
//   - its lowest mode's pitch and how long that rings, in seconds, and how much faster each higher mode
//     dies;
//   - where it is struck, how far a hard hit bends its pitch up and how long that takes to settle, in seconds, how
//     long the stick or beater touches it at Tone 0, in seconds, and how hard it drives the output;
//   - its slap, the many modes above those it rings with, heard as noise: how loud, at what pitch, and how long it
//     rings;
//   - its click, the stick's tip, a burst of noise 1.5 ms long: how loud, and at what pitch;
//   - its buzz, noise as loud as its ringing, as a snare's wires or a cymbal's sizzle: how loud, at what pitch, and
//     how long it rattles on after the ringing falls, in seconds, 0 for 5 ms, and how much of it pulses with each
//     swing of the head, from 0 to 1;
//   - how hard the whole sound is squashed, 0 for not at all;
//   - and the pitch its sound is highpassed at, 0 for none, and how loud it is, 1 for as loud as it rings.
type head struct {
	hz, ring, damp, strike, bend, settle, contact, drive float32
	slap, slapHz, slapRing                               float32
	click, clickHz                                       float32
	buzz, buzzHz, buzzRing, rattle                       float32
	squash, hp, gain                                     float32
}

// heads are the modelled drums' heads, by kind.
var heads = map[int]head{
	drModalKick: {hz: 54, ring: 0.065, damp: 4.7, strike: 0.12, bend: 1.7, settle: 0.023, contact: 0.53e-3, drive: 1.8,
		slap: 1.57, slapHz: 1530, slapRing: 0.023, gain: 1},
	drModalTom: {hz: 112, ring: 0.18, damp: 9.45, strike: 0.0087, bend: 0.18, settle: 0.15, contact: 0.2e-3, drive: 1.2,
		slap: 1.14, slapHz: 920, slapRing: 0.0389, click: 1.5, clickHz: 4500, squash: 0.66, gain: 1.15},
	drModalSnare: {hz: 175, ring: 0.028, damp: 1.74, strike: 0.35, bend: 0.225, settle: 0.1125, contact: 0.3e-3, drive: 2.4,
		slap: 2.03, slapHz: 1667, slapRing: 0.0106, click: 2.5, clickHz: 5000, buzz: 0.89, buzzHz: 4000, buzzRing: 0.2,
		rattle: 0.6, squash: 1.125, hp: 12, gain: 1.25},
}

// isModal reports whether drum kind k is a modelled drum with a head.
func isModal(k int) bool { return k >= drModalKick && k <= drModalSnare }

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
	// click filters noise into the stick's click, as loud as clickLevel.
	click      svf
	clickLevel float32
	// buzz filters noise into the buzz, as loud as buzzLevel times follow, which follows how loud the drum rings and
	// keeps fall of itself a frame, with rattle of it pulsing with the head's swing instead.
	buzz                            svf
	buzzLevel, follow, fall, rattle float32
	// squash is how hard the whole sound is squashed, hp highpasses it where hasHP says, and gain sets how loud it is.
	squash float32
	hp     svf
	hasHP  bool
	gain   float32
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
	m.n = len(membraneModes)
	for i, md := range membraneModes {
		ratio := float32(md.zero / membraneModes[0].zero)
		m.hz[i] = hd.hz * tune * ratio * jitter(0.003)
		tau := hd.ring * decay / (1 + hd.damp*(ratio-1))
		m.keep[i] = float32(math.Exp(-1 / (float64(tau) * rate)))
		m.amp[i] = float32(math.Abs(math.Jn(md.order, md.zero*float64(strike)))) * jitter(0.05)
		top = max(top, m.amp[i])
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
	m.buzzLevel, m.follow, m.rattle = hd.buzz, 0, hd.rattle
	m.fall = followFall
	if hd.buzzRing > 0 {
		m.fall = float32(math.Exp(-1 / (float64(hd.buzzRing) * rate)))
	}
	m.click.reset()
	m.click.set(hd.clickHz*bright, 0.4)
	m.clickLevel = 20 * hd.click * h.vel * h.vel
	m.hp.reset()
	m.squash, m.gain = hd.squash, hd.gain
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

// followFall is how much of how loud the drum rang the buzz keeps a frame, as a snare's wires settle in 5 ms, unless
// its head says otherwise.
var followFall = float32(math.Exp(-1 / (0.005 * rate)))

// clickFade is how loud the click is, frame by frame, falling to nothing over 1.5 ms.
var clickFade = func() []float32 {
	f := make([]float32, int(0.0015*rate)+1)
	for i := range f {
		x := 1 - float32(i)/float32(len(f)-1)
		f[i] = x * x
	}
	return f
}()

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
		m.follow = max(m.follow*m.fall, abs32(s))
		_, bp, _ := m.buzz.step(noise.bipolar())
		s += bp * ((1-m.rattle)*m.follow + m.rattle*abs32(s)) * m.buzzLevel
	}
	_, bp, _ := m.slap.step(noise.bipolar())
	s += bp * m.level
	m.level *= m.fade
	if m.clickLevel > 0 {
		_, bp, _ := m.click.step(noise.bipolar())
		s += bp * m.clickLevel * clickFade[min(t, int64(len(clickFade)-1))]
	}
	if m.squash > 0 {
		s = softClip(s * m.squash)
	}
	if m.hasHP {
		_, _, s = m.hp.step(s)
	}
	return s * m.gain
}

// quiet reports whether every mode, the slap and the buzz have fallen past hearing, -54 dB.
func (m *membrane) quiet() bool {
	if m.level > 0.002 || m.follow*m.buzzLevel > 0.002 {
		return false
	}
	for i := range m.n {
		if abs32(m.re[i])+abs32(m.im[i]) > 0.002 {
			return false
		}
	}
	return true
}
