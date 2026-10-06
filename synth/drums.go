package synth

import (
	"math"
)

// metal are the frequencies of the six square waves a TR-808 sounds its
// hats and cymbals with, which clash into a metallic shimmer.
var metal = [6]float32{205.3, 304.4, 369.6, 522.7, 540, 800}

// timpaniPartials are a kettledrum's partials: each one's pitch, as a
// multiple of the note's, its level, and how long it rings, in seconds.
var timpaniPartials = [5][3]float32{{1, 1, 1.9}, {1.504, 0.55, 1.3}, {1.742, 0.35, 1.0}, {2, 0.25, 0.8}, {2.245, 0.15, 0.6}}

// hit is a drum sounding.
type hit struct {
	on   bool
	d    drum
	t    int64
	end  int64
	vel  float32
	pan  float32
	tune float32
	// pitch is a pitched drum's note, as a timpani's.
	pitch float32
	// dur is how long a riser or a down lasts, in frames.
	dur int64
	age uint64
	ph  [6]float32
	// env are the hit's envelopes, each falling from 1 by its mul every
	// frame: a multiply a frame in place of an exponential.
	env   [8]float32
	mul   [8]float32
	f1    svf
	hp    onePole
	noise *rng
	// fade falls from 1 once the hit is choked, as an open hat is by a
	// closed one.
	fade, choke float32
	// sid is a SID drum's noise, and sidVol its level.
	sid    lfsr
	nes    nesNoise
	sidVol float32
}

// decay starts envelope i falling from 1 to 1/e over tau seconds.
func (h *hit) decay(i int, tau float32) {
	h.env[i] = 1
	h.mul[i] = float32(math.Exp(-1 / (float64(max(tau, 1e-4)) * rate)))
}

func (h *hit) start(d drum, vel, pan, pitch float32, dur int64, tune float32, age uint64) {
	*h = hit{on: true, d: d, vel: vel, pan: pan + float32(d.Pan), pitch: pitch, dur: dur, age: age, noise: h.noise, fade: 1, tune: tune}
	if h.noise == nil {
		h.noise = newRand(age*31 + 7)
	}
	tone := float32(d.Tone)
	decay := float32(d.Decay)
	if decay <= 0 {
		decay = 1
	}
	secs := func(s float32) int64 { return int64(s * rate) }
	switch d.kind {
	case drKick:
		h.decay(0, 0.032)
		h.decay(1, 0.38*decay)
		h.decay(2, 0.0018)
		h.end = secs(1.6 * decay)
	case drSnare:
		h.f1.set(1200+3800*tone, 0.15)
		h.decay(0, 0.07)
		h.decay(1, 0.17*decay)
		h.end = secs(0.9 * decay)
	case drClap:
		h.f1.set(1100+500*tone, 0.45)
		h.decay(0, 0.0045)
		h.decay(1, 0.16*decay)
		h.end = secs(0.9 * decay)
	case drHat, drOHat:
		h.f1.set(9500+2000*tone, 0.25)
		h.hp.set(7000)
		length := float32(0.042)
		if d.kind == drOHat {
			length = 0.36
		}
		h.decay(0, length*decay)
		h.end = secs(length * 7 * decay)
	case drRim:
		h.f1.set(1700, 0.75)
		h.decay(0, 0.011)
		h.end = secs(0.12)
	case drTom:
		h.decay(0, 0.05)
		h.decay(1, 0.32*decay)
		h.decay(2, 0.004)
		h.end = secs(1.6 * decay)
	case drCrash, drRide:
		h.f1.set(6000+3000*tone, 0.2)
		h.hp.set(4200)
		length := float32(1.1)
		if d.kind == drRide {
			length = 0.7
		}
		h.decay(0, length*decay)
		h.end = secs(length * 6 * decay)
	case drShaker:
		h.f1.set(6500+2500*tone, 0.35)
		h.decay(0, 0.045*decay)
		h.end = secs(0.4 * decay)
	case drSnap:
		h.f1.set(2200+800*tone, 0.6)
		h.decay(0, 0.028)
		h.decay(1, 0.0015)
		h.end = secs(0.25)
	case drTimpani:
		h.f1.set(700+900*tone, 0)
		for k, p := range timpaniPartials {
			h.decay(k, p[2]*decay)
		}
		h.decay(5, 0.08)
		h.decay(6, 0.012)
		h.end = secs(6 * decay)
	case drBoom:
		h.f1.set(1800, 0)
		h.decay(0, 0.12)
		h.decay(1, 1.1*decay)
		h.decay(2, 0.09)
		h.end = secs(6 * decay)
	case drRiser, drDown:
		h.end = dur + secs(0.03)
	default:
		if isChipDrum(d.kind) {
			h.sid.reset()
			h.nes = nesNoise{}
			frames, fps := chipDrum(d.kind)
			h.end = int64(float32(len(frames))*decay*rate/fps) + secs(0.05)
		}
	}
	for i := range h.ph {
		h.ph[i] = float32(h.noise.float())
	}
}

// render adds the hit's sound to l and r.
func (h *hit) render(l, r []float32) {
	d := &h.d
	tune := exp2((float32(d.Tune) + h.tune) / 12)
	tone := float32(d.Tone)
	g := h.vel * d.gain
	pl, pr := panGains(h.pan)
	e := &h.env
	for i := range l {
		var s float32
		switch d.kind {
		case drKick:
			f := (46 + 110*e[0]) * tune
			h.ph[0] += f / rate
			if h.ph[0] >= 1 {
				h.ph[0]--
			}
			s = sin1(h.ph[0])*e[1] + h.noise.bipolar()*e[2]*(0.25+0.5*tone)
			s = softClip(s * 1.6)
		case drSnare:
			h.ph[0] += 185 * tune / rate
			h.ph[1] += 330 * tune / rate
			h.ph[0] -= float32(int(h.ph[0]))
			h.ph[1] -= float32(int(h.ph[1]))
			body := (sin1(h.ph[0]) + 0.6*sin1(h.ph[1])) * e[0] * 0.6
			_, _, hp := h.f1.step(h.noise.bipolar())
			s = body + hp*e[1]*0.85
		case drClap:
			// Four bursts a hundredth of a second apart, then the tail.
			if h.t < 4*clapGap && h.t%clapGap == 0 {
				e[0] = 1
			}
			a := e[0]
			if h.t >= 3*clapGap {
				a = max(a, 0.7*e[1])
			} else {
				h.env[1] = 1
			}
			_, bp, _ := h.f1.step(h.noise.bipolar())
			s = bp * a * 2.4
		case drHat, drOHat:
			m := h.metal(1.55 * tune)
			m = m/6 + 0.35*h.noise.bipolar()
			_, bp, _ := h.f1.step(m)
			s = h.hp.hp(bp) * e[0] * 4
		case drRim:
			h.ph[0] += 1700 * tune / rate
			h.ph[0] -= float32(int(h.ph[0]))
			_, bp, _ := h.f1.step(h.noise.bipolar())
			s = (sin1(h.ph[0])*0.6 + bp) * e[0]
		case drTom:
			f := 125 * tune * (1 + 0.55*e[0])
			h.ph[0] += f / rate
			h.ph[0] -= float32(int(h.ph[0]))
			s = sin1(h.ph[0])*e[1] + h.noise.bipolar()*e[2]*0.2
			s = softClip(s * 1.3)
		case drCrash, drRide:
			m := h.metal(2.3 * tune)
			noise := float32(0.8)
			if d.kind == drRide {
				noise = 0.25
			}
			m = m/6*(1.2-noise) + noise*h.noise.bipolar()
			_, bp, _ := h.f1.step(m)
			s = h.hp.hp(bp+m*0.3) * e[0] * 2.6
		case drShaker:
			_, bp, _ := h.f1.step(h.noise.bipolar())
			a := e[0]
			if h.t < shakerRise {
				a = float32(h.t) / shakerRise
				h.env[0] = 1
			}
			s = bp * a * 1.6
		case drSnap:
			_, bp, _ := h.f1.step(h.noise.bipolar())
			s = bp*e[0]*2 + h.noise.bipolar()*e[1]*0.3
		case drTimpani:
			s = h.timpani(tune)
		case drBoom:
			f := (36 + 90*e[0]) * tune
			h.ph[0] += f / rate
			h.ph[0] -= float32(int(h.ph[0]))
			lp, _, _ := h.f1.step(h.noise.bipolar())
			s = sin1(h.ph[0])*e[1] + lp*e[2]*0.9
			s = softClip(s * 1.4)
		case drRiser, drDown:
			s = h.sweep(tone)
		default:
			if isChipDrum(d.kind) {
				dec := float32(d.Decay)
				if dec <= 0 {
					dec = 1
				}
				s = h.sidDrum(tune, max(dec, 0.1), tone)
			}
		}
		for k := range e {
			e[k] *= h.mul[k]
		}
		if h.t%control == 0 {
			// An envelope left to fall forever reaches the denormal
			// numbers, which the processor works on a hundred times
			// slower; past hearing, it stops at 0.
			for k := range e {
				if e[k] < 1e-7 {
					e[k] = 0
				}
			}
		}
		if h.choke > 0 {
			h.fade *= h.choke
		}
		s *= g * h.fade
		l[i] += s * pl
		r[i] += s * pr
		h.t++
	}
	if h.t > h.end || h.fade < 1e-4 || h.silent() {
		h.on = false
	}
}

// silent reports whether every envelope of the hit has fallen past
// hearing, -54 dB, so it can end before its time.
func (h *hit) silent() bool {
	if h.d.kind == drRiser || h.d.kind == drDown || isChipDrum(h.d.kind) || h.t < 4*clapGap {
		return false
	}
	for k := range h.env {
		if h.mul[k] > 0 && h.env[k] > 0.002 {
			return false
		}
	}
	return true
}

const (
	clapGap    = rate * 105 / 10000
	shakerRise = rate * 12 / 1000
)

// metal steps the six squares at their frequencies times mul, and
// returns their sum, from -6 to 6.
func (h *hit) metal(mul float32) float32 {
	var m float32
	for k, f := range metal {
		h.ph[k] += f * mul / rate
		h.ph[k] -= float32(int(h.ph[k]))
		if h.ph[k] < 0.5 {
			m++
		} else {
			m--
		}
	}
	return m
}

// timpani is a kettledrum, tuned to its note: a membrane's partials,
// each dying at its own pace, the pitch sagging a little as the mallet
// lands, and the mallet's thud.
func (h *hit) timpani(tune float32) float32 {
	pitch := h.pitch
	if pitch == 0 {
		pitch = 41
	}
	f := float32(noteHz(float64(pitch))) * tune * (1 + 0.02*h.env[5])
	var s float32
	for k, p := range timpaniPartials {
		h.ph[k] += f * p[0] / rate
		h.ph[k] -= float32(int(h.ph[k]))
		s += sin1(h.ph[k]) * p[1] * h.env[k]
	}
	lp, _, _ := h.f1.step(h.noise.bipolar())
	s += lp * h.env[6] * 1.2
	return softClip(s*0.9) * min(float32(h.t)/(0.002*rate), 1)
}

// sweep is a riser, or a down: noise through a resonant filter and a
// tone, both sweeping up as the sound swells, or down as it fades, over
// the hit's length.
func (h *hit) sweep(tone float32) float32 {
	dur := float32(max(h.dur, 1))
	x := min(float32(h.t)/dur, 1)
	if h.d.kind == drDown {
		x = 1 - x
	}
	if h.t%control == 0 {
		h.f1.set(250*exp2(x*5.2), 0.55)
	}
	_, bp, _ := h.f1.step(h.noise.bipolar())
	f := 180 * exp2(x*3.2)
	h.ph[0] += f / rate
	h.ph[0] -= float32(int(h.ph[0]))
	h.ph[1] += f * 1.007 / rate
	h.ph[1] -= float32(int(h.ph[1]))
	saw := (2*h.ph[0] - 1 + 2*h.ph[1] - 1) * 0.25
	a := x * x
	if h.d.kind == drDown {
		a = x * x * x
	}
	// A quick fade at the end, so it stops without a click.
	if left := dur - float32(h.t); left < 0.03*rate {
		a *= max(left/(0.03*rate), 0)
	}
	return (bp*1.8 + saw*(0.15+0.6*tone)) * a
}

// hits are a track's drums sounding.
type hits struct {
	hs  []hit
	age uint64
}

func newHits(n int) *hits { return &hits{hs: make([]hit, n)} }

// play sounds drum d. A closed hat chokes an open one.
func (hh *hits) play(d drum, vel, pan, pitch float32, dur int64, tune float32) {
	hh.age++
	if d.kind == drHat {
		for i := range hh.hs {
			if hh.hs[i].on && hh.hs[i].d.kind == drOHat {
				hh.hs[i].choke = 0.997
			}
		}
	}
	best := 0
	for i := range hh.hs {
		if !hh.hs[i].on {
			best = i
			break
		}
		if hh.hs[i].age < hh.hs[best].age {
			best = i
		}
	}
	hh.hs[best].start(d, vel, pan, pitch, dur, tune, hh.age)
}

func (hh *hits) render(l, r []float32) bool {
	sounding := false
	for i := range hh.hs {
		if hh.hs[i].on {
			hh.hs[i].render(l, r)
			sounding = true
		}
	}
	return sounding
}
