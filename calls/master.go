package calls

import (
	"fmt"
	"math"
	"slices"

	"github.com/marrasen/gunim/audio"
)

// Stats are how a take measures, against what the game asks of a call.
type Stats struct {
	// Length is how long it lasts, in seconds.
	Length float64
	// Peak is its true peak, in dBTP.
	Peak float64
	// Loudness is how loud it is at its loudest, in LUFS over 400 ms.
	Loudness float64
	// Lows is the share of its energy below 300 Hz, and Presence the
	// share from 1 to 4 kHz, where a phone's speaker carries it.
	Lows, Presence float64
	// Phone is how much quieter it is on a phone's speaker than in
	// full, in decibels: what a phone, playing nothing low, takes away.
	Phone float64
	// Limited is the most the limiter turned it down, in decibels.
	Limited float64
	// Onset is how long it takes to come within 20 dB of its peak, in
	// seconds.
	Onset float64
}

// What the game asks of a call.
const (
	maxOnset = 0.02
	// maxPhone is the most a phone's speaker may take from a call: a
	// call quieter than that on a phone is lost under the game.
	maxPhone = 6
)

// Lengths returns the shortest and longest a call of kind may be, in
// seconds.
func Lengths(kind string) (lo, hi float64) {
	switch kind {
	case Cheer:
		return 0.3, 1.2
	case Hurt:
		return 0.2, 0.6
	case Taunt, Roar, Laugh, Worried, Whimper:
		return 0.4, 1.3
	case Defeat:
		return 0.6, 1.8
	}
	return 0.3, 0.7
}

// Problems says where a take of a call of kind falls short of what the
// game asks, with m the master it was finished by.
func (s Stats) Problems(kind string, m Master) []string {
	var out []string
	lo, hi := Lengths(kind)
	if s.Length < lo || s.Length > hi {
		out = append(out, fmt.Sprintf("lasts %.2f s, not %.1f–%.1f s", s.Length, lo, hi))
	}
	if s.Peak > m.Ceiling+0.05 {
		out = append(out, fmt.Sprintf("peaks at %.1f dBTP, over %.1f", s.Peak, m.Ceiling))
	}
	if s.Onset > maxOnset {
		out = append(out, fmt.Sprintf("takes %.0f ms to sound", s.Onset*1000))
	}
	if s.Phone > maxPhone {
		out = append(out, fmt.Sprintf("is %.1f dB quieter on a phone", s.Phone))
	}
	if math.Abs(s.Loudness-m.Loudness) > 1 {
		out = append(out, fmt.Sprintf("is %.1f LUFS, not %.0f", s.Loudness, m.Loudness))
	}
	return out
}

// master finishes a call: lifted where a phone carries it, in its room,
// ended by its cut where set, cut below the master's pitch,
// trimmed to start at once and end with its tail, brought to the
// master's loudness and held under its ceiling.
func (l *Library) master(x []float64, call *Call) ([]float32, Stats) {
	roomAmt, cut := call.Room, call.Cut
	m := l.Master
	if len(x) == 0 {
		return nil, Stats{}
	}
	// Room for the tail.
	x = append(x, make([]float64, int(0.3*rate))...)
	if call.Presence != 0 {
		eq := peaking(2200, 0.6, call.Presence)
		for i, v := range x {
			x[i] = eq.step(v)
		}
	}
	if roomAmt > 0 {
		rm := newRoom(0.35)
		pre := highPass(400, 0.7)
		for i, v := range x {
			x[i] = v + roomAmt*0.6*rm.step(pre.step(v))
		}
	}
	if m.Slope == 12 {
		h := highPass(m.HighPass, 0.7071)
		for i, v := range x {
			x[i] = h.step(v)
		}
	} else {
		// A fourth-order Butterworth, as two sections.
		h1, h2 := highPass(m.HighPass, 0.5412), highPass(m.HighPass, 1.3066)
		for i, v := range x {
			x[i] = h2.step(h1.step(v))
		}
	}
	if cut > 0 {
		// Fade over the 80 ms before the cut, from where the sound
		// starts, so the call ends by then.
		start := 0
		for start < len(x) && x[start] == 0 {
			start++
		}
		end := min(start+int(cut*rate), len(x))
		fade := int(0.08 * rate)
		for i := max(end-fade, 0); i < end; i++ {
			x[i] *= smooth(float64(end-i) / float64(fade))
		}
		x = x[:end]
	}
	x = trim(x)
	if len(x) == 0 {
		return nil, Stats{}
	}
	// The limiter takes some of the loudness with each peak it holds
	// down, so bring it up and hold it again, a few times, from the
	// sound as it was.
	src := slices.Clone(x)
	var limited, loud float64
	g := dB(m.Loudness - loudness(x))
	for range 4 {
		for i, v := range src {
			x[i] = v * g
		}
		limited = limit(x, dB(m.Ceiling-0.3))
		// Untouched by the limiter, the take is as loud as asked.
		loud = m.Loudness
		if limited > 0 {
			loud = loudness(x)
		}
		miss := m.Loudness - loud
		if math.Abs(miss) < 0.1 {
			break
		}
		g *= dB(miss)
	}
	out := make([]float32, len(x))
	for i, v := range x {
		out[i] = float32(v)
	}
	// The limiter holds the samples under the ceiling; between them a
	// peak may pass it still, so turn the take down by what passes.
	peak := truePeak(out)
	if over := peak - m.Ceiling; over > 0 {
		g := float32(dB(-over - 0.02))
		for i := range out {
			out[i] *= g
		}
		peak -= over + 0.02
		loud -= over + 0.02
	}
	st := measure(out, loud)
	st.Peak, st.Limited = peak, limited
	return out, st
}

// trim cuts x's silence before it and after it, leaving it starting at
// once, fading in over a fraction of a millisecond, and ending with a
// short fade.
func trim(x []float64) []float64 {
	var top float64
	for _, v := range x {
		top = max(top, math.Abs(v))
	}
	if top == 0 {
		return nil
	}
	start, end := 0, len(x)
	for start < len(x) && math.Abs(x[start]) < top*dB(-40) {
		start++
	}
	for end > start && math.Abs(x[end-1]) < top*dB(-60) {
		end--
	}
	end = min(end+int(0.005*rate), len(x))
	x = x[start:end]
	in, out := 15, min(int(0.008*rate), len(x))
	for i := range min(in, len(x)) {
		x[i] *= float64(i) / float64(in)
	}
	for i := range out {
		x[len(x)-1-i] *= float64(i) / float64(out)
	}
	return x
}

// loudness returns how loud x is at its loudest over 400 ms, in LUFS, as
// BS.1770 weighs it, the sound taken as heard in silence.
func loudness(x []float64) float64 {
	// The K-weighting's two filters at 48 kHz, as BS.1770 gives them.
	shelf := biquad{b0: 1.53512485958697, b1: -2.69169618940638, b2: 1.19839281085285, a1: -1.69065929318241, a2: 0.73248077421585}
	hp := biquad{b0: 1, b1: -2, b2: 1, a1: -1.99004745483398, a2: 0.99007225036621}
	win, hop := int(0.4*rate), int(0.01*rate)
	n := len(x) + win
	pw := make([]float64, n+1)
	for i := range n {
		var v float64
		if i < len(x) {
			v = x[i]
		}
		v = hp.step(shelf.step(v))
		pw[i+1] = pw[i] + v*v
	}
	var top float64
	for s := 0; s+win <= n; s += hop {
		top = max(top, (pw[s+win]-pw[s])/float64(win))
	}
	return -0.691 + 10*math.Log10(max(top, 1e-12))
}

// limit holds x under ceil, turning it down ahead of each peak and back
// up over 60 ms, and returns the most it turned it down, in decibels.
func limit(x []float64, ceil float64) float64 {
	look := int(0.0015 * rate)
	need := make([]float64, len(x))
	for i, v := range x {
		need[i] = min(1, ceil/max(math.Abs(v), 1e-12))
	}
	// Each sample takes the least gain of the lookahead after it, then
	// the mean of the lookahead before it, which never rises over what
	// any peak in reach needs.
	held := make([]float64, len(x))
	// A running least, from the end: q holds the samples ahead that
	// may yet be the least, their needs rising.
	q := make([]int, 0, look+1)
	for i := len(x) - 1; i >= 0; i-- {
		for len(q) > 0 && need[q[len(q)-1]] >= need[i] {
			q = q[:len(q)-1]
		}
		q = append(q, i)
		if q[0] > i+look {
			q = q[1:]
		}
		held[i] = need[q[0]]
	}
	rel := 1 - math.Exp(-1/(0.06*rate))
	var sum float64
	gain, most := 1.0, 1.0
	for i := range x {
		sum += held[i]
		if i > look {
			sum -= held[i-look-1]
		}
		avg := sum / float64(min(i+1, look+1))
		gain = min(avg, gain+(1-gain)*rel)
		x[i] *= gain
		most = min(most, gain)
	}
	return max(0, -toDB(most))
}

// truePeak returns x's true peak, in dBTP, as BS.1770 finds it: the
// loudest x gets between its samples too, found by playing it at four
// times its rate through a windowed sinc.
func truePeak(x []float32) float64 {
	const taps = 12
	var top float64
	for i := range x {
		top = max(top, math.Abs(float64(x[i])))
		for ph := 1; ph < 4; ph++ {
			var v float64
			for k := range taps {
				j := i - taps/2 + 1 + k
				if j >= 0 && j < len(x) {
					v += float64(x[j]) * sincTable[ph][k]
				}
			}
			top = max(top, math.Abs(v))
		}
	}
	return toDB(top)
}

// sincTable holds the interpolating filter's phases: phase p reads the
// sound p quarters of a sample after each sample.
var sincTable = func() (f [4][12]float64) {
	for p := range f {
		for k := range f[p] {
			t := float64(k-5) - float64(p)/4
			s := 1.0
			if t != 0 {
				s = math.Sin(math.Pi*t) / (math.Pi * t)
			}
			// A Blackman-Harris window over the twelve taps.
			u := 2 * math.Pi * (t + 6) / 12
			w := 0.35875 - 0.48829*math.Cos(u) + 0.14128*math.Cos(2*u) - 0.01168*math.Cos(3*u)
			f[p][k] = s * w
		}
	}
	return f
}()

// measure returns how a finished take measures, all but its peak, it
// being loud LUFS.
func measure(x []float32, loud float64) Stats {
	st := Stats{Length: float64(len(x)) / rate}
	xs := make([]float64, len(x))
	var top float64
	for i, v := range x {
		xs[i] = float64(v)
		top = max(top, math.Abs(xs[i]))
	}
	st.Loudness = loud
	for i, v := range xs {
		if math.Abs(v) >= top*dB(-20) {
			st.Onset = float64(i) / rate
			break
		}
	}
	st.Lows, st.Presence = bands(xs)
	st.Phone = phoneDrop(xs)
	return st
}

// bands returns the share of x's energy below 300 Hz, and from 1 to 4
// kHz.
func bands(x []float64) (lows, presence float64) {
	n := 1
	for n < len(x) {
		n *= 2
	}
	buf := make([]complex128, n)
	for i, v := range x {
		buf[i] = complex(v, 0)
	}
	audio.NewFFT(n).Transform(buf)
	var all float64
	for k := 1; k < n/2; k++ {
		hz := float64(k) * rate / float64(n)
		p := real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
		all += p
		switch {
		case hz < 300:
			lows += p
		case hz >= 1000 && hz <= 4000:
			presence += p
		}
	}
	if all == 0 {
		return 0, 0
	}
	return lows / all, presence / all
}

// Spectrum returns the level of x in bins bands from 100 Hz to 16 kHz,
// spaced by ratios, in decibels under its loudest band.
func Spectrum(x []float32, bins int) []float32 {
	n := 1
	for n < len(x) {
		n *= 2
	}
	buf := make([]complex128, n)
	for i, v := range x {
		// A Hann window over the take.
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(max(len(x)-1, 1)))
		buf[i] = complex(float64(v)*w, 0)
	}
	audio.NewFFT(n).Transform(buf)
	out := make([]float64, bins)
	lo, hi := math.Log(100.0), math.Log(16000.0)
	for k := 1; k < n/2; k++ {
		hz := float64(k) * rate / float64(n)
		b := int((math.Log(hz) - lo) / (hi - lo) * float64(bins))
		if b < 0 || b >= bins {
			continue
		}
		out[b] += real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
	}
	var top float64
	for b := range out {
		// Each band's energy over the bins it holds, which grow with
		// the pitch.
		lo0 := math.Exp(lo + (hi-lo)*float64(b)/float64(bins))
		lo1 := math.Exp(lo + (hi-lo)*float64(b+1)/float64(bins))
		out[b] /= max((lo1-lo0)*float64(n)/rate, 1)
		top = max(top, out[b])
	}
	res := make([]float32, bins)
	for b, v := range out {
		res[b] = float32(10 * math.Log10(max(v/max(top, 1e-30), 1e-9)))
	}
	return res
}
