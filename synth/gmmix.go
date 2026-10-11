package synth

import "math"

// GMMix is how a [GM] mixes its channels: each channel's strip, the
// effects they send to, and the master's equaliser, compressor, gain
// and limiter, in that order. [DefaultGMMix] leaves the sound as the
// file has it.
type GMMix struct {
	Channels [16]GMStrip
	Reverb   GMReverb
	Chorus   GMChorus
	Delay    GMDelay
	// EQ are the master equaliser's five bands: a low shelf, three
	// bells and a high shelf.
	EQ      [5]GMBand
	Comp    GMComp
	Limiter GMLimiter
	// Gain is the master's level, in decibels, before the limiter.
	Gain float64
}

// GMStrip is a channel's strip of the mix.
type GMStrip struct {
	// Gain is the channel's level, in decibels, on top of what its
	// volume and expression set, and Pan moves it from where its pan
	// puts it, from -1 to 1.
	Gain, Pan float64
	// Reverb and Chorus scale what the channel sends to them, 1 as the
	// file has it; Delay is what it sends to the delay, from 0 to 1.
	Reverb, Chorus, Delay float64
	// Low, Mid and High turn its lows, at 120 Hz, its middle, at 1 kHz,
	// and its highs, at 6 kHz, up or down, in decibels.
	Low, Mid, High float64
	// Drive saturates the channel, from 0, clean, to 1.
	Drive float64
}

// GMReverb is the reverb the channels send to: its size, from 0.3 to
// 1.5, how many seconds it takes to die, how bright it is, from 0 to
// 1, and how loud it comes back, 1 as made.
type GMReverb struct{ Size, Decay, Tone, Return float64 }

// GMChorus is the chorus the channels send to: how loud it comes back.
type GMChorus struct{ Return float64 }

// GMDelay is the delay the channels send to: Time seconds between its
// echoes, each Feedback as loud as the one before, from 0 to 0.9, Tone
// how bright they are, from 0 to 1, and how loud they come back.
type GMDelay struct{ Time, Feedback, Tone, Return float64 }

// GMBand is a band of the master equaliser: where it is, in hertz, how
// far it turns that up or down, in decibels, and how narrow a bell is.
type GMBand struct{ Freq, Gain, Q float64 }

// GMComp is the master's compressor, which glues the mix: it turns the
// sound down by Ratio above Threshold decibels, and up by Makeup after.
type GMComp struct {
	On                       bool
	Threshold, Ratio, Makeup float64
}

// GMLimiter is the master's limiter, which keeps the sound under
// Ceiling decibels.
type GMLimiter struct {
	On      bool
	Ceiling float64
}

// DefaultGMMix returns the mix that leaves the sound as the file has
// it: each strip flat, the effects as a GM has them, and the limiter on
// at -1 dB.
func DefaultGMMix() GMMix {
	m := GMMix{
		Reverb:  GMReverb{Size: 0.9, Decay: 2.2, Tone: 0.5, Return: 1},
		Chorus:  GMChorus{Return: 1},
		Delay:   GMDelay{Time: 0.375, Feedback: 0.35, Tone: 0.5, Return: 1},
		EQ:      [5]GMBand{{Freq: 80, Q: 0.7}, {Freq: 250, Q: 1}, {Freq: 1000, Q: 1}, {Freq: 4000, Q: 1}, {Freq: 10000, Q: 0.7}},
		Comp:    GMComp{Threshold: -12, Ratio: 2},
		Limiter: GMLimiter{On: true, Ceiling: -1},
	}
	for i := range m.Channels {
		m.Channels[i] = GMStrip{Reverb: 1, Chorus: 1}
	}
	return m
}

// GMMeters are how loud the mix has been since they were last taken:
// each channel's peak, after its strip, and the master's, both from 0
// to 1 and over, and how far the compressor and the limiter turned it
// down, in decibels.
type GMMeters struct {
	Channels    [16]float32
	Left, Right float32
	Comp, Limit float32
}

// biquad is a filter of two poles and two zeros, as RBJ's cookbook
// gives its shelves and bells.
type biquad struct {
	b0, b1, b2, a1, a2 float32
	z1, z2             float32
}

// The kinds of biquad.
const (
	bqLowShelf = iota
	bqBell
	bqHighShelf
)

// set tunes the filter: a shelf or a bell at hz, gain decibels, q wide.
func (f *biquad) set(kind int, hz, gain, q float64) {
	a := math.Pow(10, gain/40)
	w := 2 * math.Pi * min(max(hz, 10), rate*0.45) / rate
	cw, sw := math.Cos(w), math.Sin(w)
	q = max(q, 0.1)
	alpha := sw / (2 * q)
	var b0, b1, b2, a0, a1, a2 float64
	switch kind {
	case bqBell:
		b0, b1, b2 = 1+alpha*a, -2*cw, 1-alpha*a
		a0, a1, a2 = 1+alpha/a, -2*cw, 1-alpha/a
	case bqLowShelf:
		s := 2 * math.Sqrt(a) * alpha
		b0, b1, b2 = a*((a+1)-(a-1)*cw+s), 2*a*((a-1)-(a+1)*cw), a*((a+1)-(a-1)*cw-s)
		a0, a1, a2 = (a+1)+(a-1)*cw+s, -2*((a-1)+(a+1)*cw), (a+1)+(a-1)*cw-s
	default:
		s := 2 * math.Sqrt(a) * alpha
		b0, b1, b2 = a*((a+1)+(a-1)*cw+s), -2*a*((a-1)+(a+1)*cw), a*((a+1)+(a-1)*cw-s)
		a0, a1, a2 = (a+1)-(a-1)*cw+s, 2*((a-1)-(a+1)*cw), (a+1)-(a-1)*cw-s
	}
	f.b0, f.b1, f.b2 = float32(b0/a0), float32(b1/a0), float32(b2/a0)
	f.a1, f.a2 = float32(a1/a0), float32(a2/a0)
}

func (f *biquad) step(x float32) float32 {
	y := f.b0*x + f.z1
	f.z1 = f.b1*x - f.a1*y + f.z2
	f.z2 = f.b2*x - f.a2*y
	return y
}

// GMBandResponse returns how many decibels band i of the master
// equaliser, set as b, turns sound at hz up or down, for a tool to draw
// the equaliser's curve.
func GMBandResponse(i int, b GMBand, hz float64) float64 {
	var f biquad
	f.set(bandKind(i), b.Freq, b.Gain, b.Q)
	w := 2 * math.Pi * hz / rate
	// |H(e^jw)| of the filter's coefficients.
	c1, s1 := math.Cos(w), math.Sin(w)
	c2, s2 := math.Cos(2*w), math.Sin(2*w)
	nr := float64(f.b0) + float64(f.b1)*c1 + float64(f.b2)*c2
	ni := -float64(f.b1)*s1 - float64(f.b2)*s2
	dr := 1 + float64(f.a1)*c1 + float64(f.a2)*c2
	di := -float64(f.a1)*s1 - float64(f.a2)*s2
	return 10 * math.Log10((nr*nr+ni*ni)/(dr*dr+di*di))
}

// bandKind returns the kind of filter band i of the master equaliser
// is: a shelf at either end, and bells between.
func bandKind(i int) int {
	switch i {
	case 0:
		return bqLowShelf
	case 4:
		return bqHighShelf
	}
	return bqBell
}

// stripFX is a channel's strip at work: its equaliser's filters, for
// each side, and the settings they were tuned for.
type stripFX struct {
	eq    [3][2]biquad
	tuned [3]float64
	flat  bool
}

// tune tunes the strip's equaliser to s, where it changed.
func (x *stripFX) tune(s GMStrip) {
	gains := [3]float64{s.Low, s.Mid, s.High}
	x.flat = gains == [3]float64{}
	for b, g := range gains {
		if g == x.tuned[b] {
			continue
		}
		x.tuned[b] = g
		for side := range 2 {
			switch b {
			case 0:
				x.eq[b][side].set(bqLowShelf, 120, g, 0.7)
			case 1:
				x.eq[b][side].set(bqBell, 1000, g, 0.8)
			default:
				x.eq[b][side].set(bqHighShelf, 6000, g, 0.7)
			}
		}
	}
}

// process runs l and r through the strip's equaliser and drive.
func (x *stripFX) process(l, r []float32, drive float64) {
	if !x.flat {
		for b := range x.eq {
			if x.tuned[b] == 0 {
				continue
			}
			fl, fr := &x.eq[b][0], &x.eq[b][1]
			for i := range l {
				l[i] = fl.step(l[i])
				r[i] = fr.step(r[i])
			}
		}
	}
	if drive > 0.001 {
		d := 1 + float32(drive)*6
		norm := 1 / softClip(d)
		for i := range l {
			l[i] = softClip(l[i]*d) * norm
			r[i] = softClip(r[i]*d) * norm
		}
	}
}
