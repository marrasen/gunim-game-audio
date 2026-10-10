package synth

import (
	"math"
)

// rate is the sample rate the engine runs at, as package audio's.
const rate = 48000

// control is how many frames the engine steps a modulation by: filter
// cutoffs, pitches and the like move once a control block, 1.5 kHz, and
// sound moves every frame.
const control = 32

// sineTable holds a cycle of a sine, with one more entry to read past
// the end without wrapping.
var sineTable = func() [sineSize + 1]float32 {
	var t [sineSize + 1]float32
	for i := range t {
		t[i] = float32(math.Sin(2 * math.Pi * float64(i) / sineSize))
	}
	return t
}()

const sineSize = 4096

// sin1 returns the sine of a phase in cycles, from 0 to 1, read from the
// table between its entries.
func sin1(phase float32) float32 {
	x := phase * sineSize
	i := int(x)
	f := x - float32(i)
	i &= sineSize - 1
	return sineTable[i] + (sineTable[i+1]-sineTable[i])*f
}

// wrap returns phase in cycles taken back into 0 to 1.
func wrap(phase float32) float32 {
	return phase - float32(math.Floor(float64(phase)))
}

// polyBLEP is the correction that takes the step out of a waveform's
// jump, phase t into a cycle that steps dt a frame: it rounds off the
// jump over a frame either side, which keeps a saw or a square from
// aliasing.
func polyBLEP(t, dt float32) float32 {
	switch {
	case t < dt:
		t /= dt
		return t + t - t*t - 1
	case t > 1-dt:
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

// softClip is a smooth saturation, tanh's shape near 0, flat at ±1.
func softClip(x float32) float32 {
	if x > 3 {
		return 1
	}
	if x < -3 {
		return -1
	}
	x2 := x * x
	return x * (27 + x2) / (27 + 9*x2)
}

// dbGain returns the gain of a level in decibels.
func dbGain(db float64) float64 { return math.Pow(10, db/20) }

// svf is a state-variable filter, as Andrew Simper's trapezoidal one:
// stable under fast modulation, and its lowpass, bandpass and highpass
// come together.
type svf struct {
	ic1, ic2   float32
	a1, a2, a3 float32
	k          float32
}

// set tunes the filter to cutoff hz with resonance res, from 0 to 1.
func (f *svf) set(hz, res float32) {
	hz = min(max(hz, 16), rate*0.45)
	g := float32(math.Tan(math.Pi * float64(hz) / rate))
	f.k = 2 - 1.96*min(max(res, 0), 1)
	f.a1 = 1 / (1 + g*(g+f.k))
	f.a2 = g * f.a1
	f.a3 = g * f.a2
}

// tuneAs tunes the filter as o is tuned, keeping its own state.
func (f *svf) tuneAs(o *svf) { f.a1, f.a2, f.a3, f.k = o.a1, o.a2, o.a3, o.k }

// step filters x and returns its lowpass, bandpass and highpass.
func (f *svf) step(x float32) (lp, bp, hp float32) {
	v3 := x - f.ic2
	v1 := f.a1*f.ic1 + f.a2*v3
	v2 := f.ic2 + f.a2*f.ic1 + f.a3*v3
	f.ic1 = 2*v1 - f.ic1
	f.ic2 = 2*v2 - f.ic2
	return v2, v1, x - f.k*v1 - v2
}

func (f *svf) reset() { f.ic1, f.ic2 = 0, 0 }

// onePole is a gentle filter of 6 dB an octave.
type onePole struct {
	a, z float32
}

// set tunes the filter to cutoff hz.
func (f *onePole) set(hz float32) {
	f.a = 1 - float32(math.Exp(-2*math.Pi*float64(min(max(hz, 1), rate*0.49))/rate))
}

// lp filters x and returns its lowpass.
func (f *onePole) lp(x float32) float32 {
	f.z += f.a * (x - f.z)
	return f.z
}

// hp filters x and returns its highpass.
func (f *onePole) hp(x float32) float32 { return x - f.lp(x) }

// Env is an envelope's shape: Attack, Decay and Release in seconds, and
// Sustain the level held, from 0 to 1, as a synth's ADSR.
type Env struct {
	Attack, Decay, Sustain, Release float64
}

// The stages of an envelope.
const (
	envIdle = iota
	envAttack
	envDecay
	envSustain
	envRelease
)

// env runs an Env: a straight attack, and a decay and release that fall
// as a capacitor does.
type env struct {
	stage   int
	v       float32
	att     float32
	dec     float32
	sus     float32
	rel     float32
	shape   Env
	stepped bool
}

// set takes the shape e, keeping where the envelope is.
func (e *env) set(s Env) {
	if e.stepped && s == e.shape {
		return
	}
	e.shape, e.stepped = s, true
	e.att = 1 / float32(max(s.Attack, 0.0005)*rate)
	e.dec = fall(s.Decay)
	e.sus = float32(min(max(s.Sustain, 0), 1))
	e.rel = fall(s.Release)
}

// fall returns how much of the way to its target a falling stage of
// secs goes in a frame: it comes within -60 dB in secs.
func fall(secs float64) float32 {
	return float32(1 - math.Exp(-6.9/(max(secs, 0.0005)*rate)))
}

// gate starts the envelope from where it is, so a voice taken over
// mid-note does not click.
func (e *env) gate() { e.stage = envAttack }

// release lets go of the note.
func (e *env) release() {
	if e.stage != envIdle {
		e.stage = envRelease
	}
}

// step returns the envelope's next level.
func (e *env) step() float32 {
	switch e.stage {
	case envAttack:
		e.v += e.att
		if e.v >= 1 {
			e.v, e.stage = 1, envDecay
		}
	case envDecay:
		e.v += (e.sus - e.v) * e.dec
		if e.v-e.sus < 1e-4 {
			e.v, e.stage = e.sus, envSustain
		}
	case envSustain:
		e.v = e.sus
	case envRelease:
		e.v -= e.v * e.rel
		if e.v < 1e-4 {
			e.v, e.stage = 0, envIdle
		}
	}
	return e.v
}

// idle reports whether the envelope has finished.
func (e *env) idle() bool { return e.stage == envIdle }

// LFO is a slow oscillator that moves a sound: its pitch, its filter,
// its level, its pulse width or its place between the speakers.
type LFO struct {
	// To is what it moves: pitch, in semitones; cutoff, in octaves;
	// amp, from 0 to 1; width, of a pulse; or pan.
	To string
	// Wave is sine, tri, saw, square or random, a new value each cycle.
	Wave string
	// Hz is its speed, or Beats, where set, its cycle's length in beats,
	// in time with the song.
	Hz, Beats float64
	// Depth is how far it moves its target.
	Depth float64
	// Delay is how long, in seconds, a note plays before the LFO fades
	// in over as long again, as a violinist's vibrato does.
	Delay float64
}

// The waves of an LFO and the like.
const (
	waveSine = iota
	waveTri
	waveSaw
	waveSquare
	waveRandom
)

func lfoWave(name string) int {
	switch name {
	case "tri", "triangle":
		return waveTri
	case "saw":
		return waveSaw
	case "square":
		return waveSquare
	case "random", "rand":
		return waveRandom
	}
	return waveSine
}

// lfoAt returns wave w at phase p, from -1 to 1; r is the random value
// of the cycle.
func lfoAt(w int, p, r float32) float32 {
	switch w {
	case waveTri:
		return 1 - 4*abs32(p-0.5)
	case waveSaw:
		return 2*p - 1
	case waveSquare:
		if p < 0.5 {
			return 1
		}
		return -1
	case waveRandom:
		return r
	}
	return sin1(p)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// exp2 returns 2 to the x, quickly, for pitches and cutoffs moved by
// octaves at control rate.
func exp2(x float32) float32 { return float32(math.Exp2(float64(x))) }

// panGains returns the left and right gains of pan, from -1 to 1, at
// equal power.
func panGains(pan float32) (l, r float32) {
	a := (min(max(pan, -1), 1) + 1) * 0.125
	return sin1(a + 0.25), sin1(a)
}

// formants are the vowels' first three resonances, in hertz, and their
// levels, as a voice sings them.
var formants = map[byte][3][2]float32{
	'a': {{800, 1}, {1150, 0.5}, {2900, 0.25}},
	'e': {{400, 1}, {1600, 0.35}, {2700, 0.2}},
	'i': {{270, 1}, {2140, 0.3}, {2950, 0.2}},
	'o': {{450, 1}, {800, 0.45}, {2830, 0.15}},
	'u': {{325, 1}, {700, 0.3}, {2530, 0.1}},
}

// formant is a bank of three resonances that makes a sound sing a vowel,
// as SuperDirt's vowel does, gliding from one vowel to the next.
type formant struct {
	f     [3]svf
	hz    [3]float32
	gain  [3]float32
	to    [3][2]float32
	ready bool
}

// set aims the bank at vowel v.
func (f *formant) set(v byte) {
	fm, ok := formants[v]
	if !ok {
		fm = formants['a']
	}
	f.to = fm
	if !f.ready {
		for i := range f.hz {
			f.hz[i], f.gain[i] = fm[i][0], fm[i][1]
		}
		f.ready = true
	}
}

// tune moves the bank a control block's way toward its vowel.
func (f *formant) tune() {
	for i := range f.f {
		f.hz[i] += (f.to[i][0] - f.hz[i]) * 0.08
		f.gain[i] += (f.to[i][1] - f.gain[i]) * 0.08
		f.f[i].set(f.hz[i], 0.86)
	}
}

// step returns x singing the vowel. Each resonance's bandpass is
// scaled to peak at its level, so a vowel shapes the sound without
// making it louder.
func (f *formant) step(x float32) float32 {
	var y float32
	for i := range f.f {
		_, bp, _ := f.f[i].step(x)
		y += bp * f.f[i].k * f.gain[i]
	}
	return y * 1.8
}

// vocoderBands are the centres of a Roland VP-330's ten bands, in hertz.
var vocoderBands = [10]float32{150, 220, 350, 500, 760, 1100, 1600, 2200, 3600, 5200}

// vocoder is a vocoder's bank of bands, each a bandpass at a fixed
// centre, whose levels a vowel's resonances set, as a voice speaking
// into its microphone would: the sound comes out in steps of the bank,
// as a machine's voice.
type vocoder struct {
	f     [10]svf
	gain  [10]float32
	to    [10]float32
	ready bool
}

// set aims the bank's levels at vowel v: each band as loud as the
// vowel's resonances near it, an octave's third or so either side.
func (vc *vocoder) set(v byte) {
	fm, ok := formants[v]
	if !ok {
		fm = formants['a']
	}
	var peak float32
	for b, hz := range vocoderBands {
		var g float64
		for _, f := range fm {
			d := math.Log2(float64(hz / f[0]))
			g += float64(f[1]) * math.Exp(-d*d/(2*0.3*0.3))
		}
		vc.to[b] = float32(g)
		peak = max(peak, float32(g))
	}
	for b := range vc.to {
		vc.to[b] = 0.04 + 0.96*vc.to[b]/peak
	}
	if !vc.ready {
		vc.gain = vc.to
		for b, hz := range vocoderBands {
			vc.f[b].set(hz, 0.82)
		}
		vc.ready = true
	}
}

// tune moves the levels a control block's way toward the vowel's, as
// a vocoder's followers follow a voice, in about 10 ms.
func (vc *vocoder) tune() {
	for b := range vc.gain {
		vc.gain[b] += (vc.to[b] - vc.gain[b]) * 0.06
	}
}

// step returns x spoken through the bank.
func (vc *vocoder) step(x float32) float32 {
	var y float32
	for b := range vc.f {
		_, bp, _ := vc.f[b].step(x)
		y += bp * vc.f[b].k * vc.gain[b]
	}
	// As loud as the formants make a saw, over the vowels.
	return y * 1.15
}
