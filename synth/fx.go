package synth

import (
	"math"
)

// reverb is a feedback delay network: eight delay lines whose outputs
// mix back into their inputs through a Hadamard matrix, each line
// darkening as it goes round, after a pre-delay and four allpass
// diffusers that smear the attack. It sounds a room to a hall by its
// size and decay.
type reverb struct {
	pre    []float32
	preAt  int
	preLen int
	ap     [4]allpass
	line   [8][]float32
	at     [8]int
	lens   [8]int
	damp   [8]float32
	g      [8]float32
	dampA  float32
	in     onePole
	out    [2]onePole
	size   float64
	decay  float64
	tone   float64
	preSec float64
}

// lineMs are the lengths of the network's lines at size 1, in
// milliseconds, chosen to share no rhythm.
var lineMs = [8]float64{29.7, 37.1, 41.1, 43.7, 53.3, 59.9, 67.1, 73.3}

type allpass struct {
	buf []float32
	at  int
	g   float32
}

func (a *allpass) step(x float32) float32 {
	d := a.buf[a.at]
	y := d - a.g*x
	a.buf[a.at] = x + a.g*y
	a.at++
	if a.at == len(a.buf) {
		a.at = 0
	}
	return y
}

func newReverb() *reverb {
	r := &reverb{pre: make([]float32, rate/4)}
	for i, ms := range []float64{4.7, 3.6, 12.7, 9.3} {
		r.ap[i] = allpass{buf: make([]float32, int(ms*rate/1000)), g: 0.62}
	}
	for i := range r.line {
		r.line[i] = make([]float32, 8192)
	}
	r.in.set(160)
	r.set(0.8, 2.2, 0.5, 0.02)
	return r
}

// set shapes the reverb: size from 0.3 to 1.5, decay in seconds to fall
// 60 dB, tone from 0, dark, to 1, bright, and the pre-delay in seconds.
func (r *reverb) set(size, decay, tone, pre float64) {
	if size == r.size && decay == r.decay && tone == r.tone && pre == r.preSec {
		return
	}
	r.size, r.decay, r.tone, r.preSec = size, decay, tone, pre
	size = min(max(size, 0.3), 1.5)
	decay = max(decay, 0.1)
	for i, ms := range lineMs {
		r.lens[i] = min(int(ms*size*rate/1000), len(r.line[i])-1)
		r.g[i] = float32(math.Pow(10, -3*float64(r.lens[i])/(decay*rate)))
	}
	r.dampA = 1 - float32(math.Exp(-2*math.Pi*(1500+tone*9000)/rate))
	r.preLen = min(max(int(pre*rate), 1), len(r.pre)-1)
	r.out[0].set(9000)
	r.out[1].set(9000)
}

// process takes the sound sent to the reverb from l and r, and leaves
// the reverb's sound in them.
func (r *reverb) process(l, rr []float32) {
	var o [8]float32
	for i := range l {
		x := (l[i] + rr[i]) * 0.5
		// Pre-delay, then the lows cut and the attack diffused.
		r.pre[r.preAt] = x
		j := r.preAt - r.preLen
		if j < 0 {
			j += len(r.pre)
		}
		r.preAt++
		if r.preAt == len(r.pre) {
			r.preAt = 0
		}
		x = r.in.hp(r.pre[j])
		for k := range r.ap {
			x = r.ap[k].step(x)
		}
		for k := range o {
			at := r.at[k] - r.lens[k]
			if at < 0 {
				at += len(r.line[k])
			}
			s := r.line[k][at]
			r.damp[k] += r.dampA * (s - r.damp[k])
			o[k] = r.damp[k]
		}
		// The Hadamard matrix, as three rounds of butterflies.
		h := o
		for step := 1; step < 8; step *= 2 {
			for k := 0; k < 8; k += 2 * step {
				for m := k; m < k+step; m++ {
					a, b := h[m], h[m+step]
					h[m], h[m+step] = a+b, a-b
				}
			}
		}
		const norm = 0.35355339 // 1/√8
		for k := range h {
			sign := float32(1)
			if k&1 == 1 {
				sign = -1
			}
			v := (h[k]*norm + x*sign*0.6) * r.g[k]
			// A whisper of noise keeps the lines out of denormals as the
			// tail dies.
			r.line[k][r.at[k]] = v + 1e-18
			r.at[k]++
			if r.at[k] == len(r.line[k]) {
				r.at[k] = 0
			}
		}
		l[i] = r.out[0].lp(o[0] + o[2] + o[4] + o[6])
		rr[i] = r.out[1].lp(o[1] + o[3] + o[5] + o[7])
	}
}

// delay is a stereo delay whose echoes cross from side to side, in time
// with the song, each echo darker and thinner than the one before.
type delay struct {
	l, r     []float32
	at       int
	frames   int
	feedback float32
	tone     onePole
	low      onePole
}

func newDelay() *delay {
	d := &delay{l: make([]float32, 2*rate), r: make([]float32, 2*rate)}
	d.tone.set(3500)
	d.low.set(250)
	return d
}

// set times the echoes beats apart at bpm, each feedback as loud as the
// one before.
func (d *delay) set(beats, bpm, feedback, tone float64) {
	d.frames = min(max(int(beats*60/bpm*rate), 1), len(d.l)-1)
	d.feedback = float32(min(max(feedback, 0), 0.95))
	d.tone.set(float32(1200 + tone*8000))
}

func (d *delay) process(l, r []float32) {
	for i := range l {
		j := d.at - d.frames
		if j < 0 {
			j += len(d.l)
		}
		dl, dr := d.l[j], d.r[j]
		in := (l[i] + r[i]) * 0.5
		fb := d.low.hp(d.tone.lp(dr)) * d.feedback
		d.l[d.at] = in + fb + 1e-18
		d.r[d.at] = dl
		d.at++
		if d.at == len(d.l) {
			d.at = 0
		}
		l[i], r[i] = dl, dr
	}
}

// chorus thickens a sound with two copies of it, each a few
// milliseconds late, by a slowly moving amount, one each side.
type chorus struct {
	buf   [2][]float32
	at    int
	phase float32
	mix   float32
}

func newChorus() *chorus {
	return &chorus{buf: [2][]float32{make([]float32, 2048), make([]float32, 2048)}}
}

func (c *chorus) process(l, r []float32) {
	const size = 2048
	var delays [2]float32
	for i := range l {
		c.buf[0][c.at] = l[i]
		c.buf[1][c.at] = r[i]
		if i%control == 0 {
			// The delays move slowly, so once a control block will do.
			c.phase += 0.45 * control / rate
			if c.phase >= 1 {
				c.phase--
			}
			delays[0] = (0.009 + 0.0035*sin1(c.phase)) * rate
			delays[1] = (0.009 + 0.0035*sin1(wrap(c.phase+0.25))) * rate
		}
		for ch := range 2 {
			d := delays[ch]
			pos := float32(c.at) - d
			if pos < 0 {
				pos += size
			}
			j := int(pos)
			f := pos - float32(j)
			a := c.buf[ch][j&(size-1)]
			b := c.buf[ch][(j+1)&(size-1)]
			wet := a + (b-a)*f
			if ch == 0 {
				l[i] = l[i]*(1-c.mix*0.5) + wet*c.mix
			} else {
				r[i] = r[i]*(1-c.mix*0.5) + wet*c.mix
			}
		}
		c.at = (c.at + 1) & (size - 1)
	}
}

// inserts are a track's effects, in the order they run: its highpass and
// lowpass, its shape, a drive that rounds the sound off; its crush, to
// fewer bits; its coarse, to a lower sample rate; and its chorus.
type inserts struct {
	hp, lp     [2]svf
	hpOn, lpOn bool
	shape      float32
	crush      float32
	coarse     int
	hold       [2]float32
	held       int
	chorus     *chorus
}

func (in *inserts) set(t *Track) {
	in.hpOn = t.HPF > 0
	if in.hpOn {
		in.hp[0].set(float32(t.HPF), 0.1)
		in.hp[1].set(float32(t.HPF), 0.1)
	}
	in.lpOn = t.LPF > 0
	if in.lpOn {
		in.lp[0].set(float32(t.LPF), 0.1)
		in.lp[1].set(float32(t.LPF), 0.1)
	}
	in.shape = float32(min(max(t.Shape, 0), 0.99))
	in.crush = 0
	if t.Crush > 0 {
		in.crush = float32(math.Exp2(t.Crush - 1))
	}
	in.coarse = max(t.Coarse, 0)
	if t.Chorus > 0 {
		if in.chorus == nil {
			in.chorus = newChorus()
		}
		in.chorus.mix = float32(min(t.Chorus, 1))
	} else {
		in.chorus = nil
	}
}

func (in *inserts) process(l, r []float32) {
	if in.hpOn {
		for i := range l {
			_, _, l[i] = in.hp[0].step(l[i])
			_, _, r[i] = in.hp[1].step(r[i])
		}
	}
	if in.lpOn {
		for i := range l {
			l[i], _, _ = in.lp[0].step(l[i])
			r[i], _, _ = in.lp[1].step(r[i])
		}
	}
	if in.shape > 0 {
		// SuperDirt's shape: more drive the nearer to 1.
		k := 2 * in.shape / (1 - in.shape)
		for i := range l {
			l[i] = (1 + k) * l[i] / (1 + k*abs32(l[i]))
			r[i] = (1 + k) * r[i] / (1 + k*abs32(r[i]))
		}
	}
	if in.crush > 0 {
		for i := range l {
			l[i] = float32(math.Round(float64(l[i]*in.crush))) / in.crush
			r[i] = float32(math.Round(float64(r[i]*in.crush))) / in.crush
		}
	}
	if in.coarse > 1 {
		for i := range l {
			if in.held == 0 {
				in.hold[0], in.hold[1] = l[i], r[i]
			}
			in.held = (in.held + 1) % in.coarse
			l[i], r[i] = in.hold[0], in.hold[1]
		}
	}
	if in.chorus != nil {
		in.chorus.process(l, r)
	}
}

// compressor is a bus compressor that glues a mix together: it turns
// the sound down by ratio above its threshold, both channels as one,
// reading the level once a control block.
type compressor struct {
	threshold float32
	ratio     float32
	att, rel  float32
	env       float32
	gain      float32
	// reduction is how far it turns the sound down now, in decibels.
	reduction float32
}

func newCompressor(thresholdDB, ratio float64) *compressor {
	return &compressor{
		threshold: float32(thresholdDB), ratio: float32(ratio), gain: 1,
		att: 1 - float32(math.Exp(-float64(control)/(0.008*rate))),
		rel: 1 - float32(math.Exp(-float64(control)/(0.18*rate))),
	}
}

func (c *compressor) process(l, r []float32) {
	for at := 0; at < len(l); at += control {
		end := min(at+control, len(l))
		var peak float32
		for i := at; i < end; i++ {
			peak = max(peak, abs32(l[i]), abs32(r[i]))
		}
		if peak > c.env {
			c.env += (peak - c.env) * c.att
		} else {
			c.env += (peak - c.env) * c.rel
		}
		// Silence would take it into the denormal numbers.
		c.env = max(c.env, 1e-9)
		db := 20 * float32(math.Log10(float64(c.env)+1e-9))
		var red float32
		if over := db - c.threshold; over > 0 {
			red = over * (1 - 1/c.ratio)
		}
		c.reduction = red
		target := float32(math.Pow(10, float64(-red/20)))
		g0 := c.gain
		step := (target - g0) / float32(end-at)
		for i := at; i < end; i++ {
			g0 += step
			l[i] *= g0
			r[i] *= g0
		}
		c.gain = target
	}
}

// limiter keeps the mix under its ceiling: it looks a millisecond ahead,
// so it turns the sound down before a peak arrives rather than as it
// does, and lets it back up over a fifth of a second.
type limiter struct {
	ceiling float32
	buf     [2][]float32
	at      int
	env     float32
	gain    float32
	rel     float32
	att     float32
}

const lookahead = 48

func newLimiter(ceilingDB float64) *limiter {
	return &limiter{
		ceiling: float32(dbGain(ceilingDB)),
		buf:     [2][]float32{make([]float32, lookahead), make([]float32, lookahead)},
		gain:    1,
		rel:     float32(math.Exp(-1 / (0.2 * rate))),
		att:     1 - float32(math.Exp(-1/(0.0008*rate))),
	}
}

func (lm *limiter) process(l, r []float32) {
	for i := range l {
		x := max(abs32(l[i]), abs32(r[i]))
		lm.env = max(x, lm.env*lm.rel, 1e-9)
		target := float32(1)
		if lm.env > lm.ceiling {
			target = lm.ceiling / lm.env
		}
		if target < lm.gain {
			lm.gain += (target - lm.gain) * lm.att
		} else {
			lm.gain = target + (lm.gain-target)*lm.rel
		}
		dl, dr := lm.buf[0][lm.at], lm.buf[1][lm.at]
		lm.buf[0][lm.at], lm.buf[1][lm.at] = l[i], r[i]
		lm.at++
		if lm.at == lookahead {
			lm.at = 0
		}
		l[i] = softClip(dl*lm.gain/lm.ceiling) * lm.ceiling
		r[i] = softClip(dr*lm.gain/lm.ceiling) * lm.ceiling
	}
}
