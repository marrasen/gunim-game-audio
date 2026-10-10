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

// clear silences the reverb, its tail and all.
func (r *reverb) clear() {
	clear(r.pre)
	for k := range r.ap {
		clear(r.ap[k].buf)
	}
	for k := range r.line {
		clear(r.line[k])
	}
	r.damp = [8]float32{}
	r.in.z, r.out[0].z, r.out[1].z = 0, 0, 0
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

// chorus thickens a sound with copies of it, each a few milliseconds
// late, by a slowly moving amount. Its kinds are:
//
//	chorusSoft      two copies, one each side, 9 ms late, moving at 0.45 Hz
//	chorusJuno1     a Juno-60's chorus I: a bucket brigade's 1.7 to 5.4 ms,
//	                swept by a triangle at 0.5 Hz, the right side swept
//	                against the left
//	chorusJuno2     its chorus II, the same sweep at 0.86 Hz
//	chorusJuno12    both buttons down: a fast, shallow 9.75 Hz vibrato
//	chorusEnsemble  a string machine's ensemble: three copies, each swept
//	                by a slow and a fast sine a third of a cycle apart
//
// A bucket brigade's copies are darker than the sound, so the Juno's and
// the ensemble's are.
type chorus struct {
	buf   [2][]float32
	at    int
	phase [2]float32
	mix   float32
	kind  int
	// delays are each copy's delay as the last block ended, in frames.
	delays [3]float32
	dark   [2]onePole
}

// The kinds of chorus.
const (
	chorusSoft = iota
	chorusJuno1
	chorusJuno2
	chorusJuno12
	chorusEnsemble
)

var chorusKinds = map[string]int{
	"": chorusSoft, "soft": chorusSoft, "juno1": chorusJuno1, "juno2": chorusJuno2, "juno12": chorusJuno12,
	"ensemble": chorusEnsemble,
}

const chorusSize = 2048

func newChorus(kind int) *chorus {
	c := &chorus{buf: [2][]float32{make([]float32, chorusSize), make([]float32, chorusSize)}, kind: kind}
	for i := range c.dark {
		c.dark[i].set(7500)
	}
	c.delays = c.targets()
	return c
}

// targets returns where each copy's delay is bound now, in frames, and
// moves the sweeps on a control block.
func (c *chorus) targets() (d [3]float32) {
	const ms = rate / 1000
	step := func(i int, hz float32) {
		c.phase[i] += hz * control / rate
		if c.phase[i] >= 1 {
			c.phase[i]--
		}
	}
	tri := func(p float32) float32 { return 1 - 4*abs32(p-0.5) }
	switch c.kind {
	case chorusSoft:
		step(0, 0.45)
		d[0] = (9 + 3.5*sin1(c.phase[0])) * ms
		d[1] = (9 + 3.5*sin1(wrap(c.phase[0]+0.25))) * ms
	case chorusJuno1, chorusJuno2:
		hz := float32(0.513)
		if c.kind == chorusJuno2 {
			hz = 0.863
		}
		step(0, hz)
		x := tri(c.phase[0])
		d[0] = (3.5 + 1.85*x) * ms
		d[1] = (3.5 - 1.85*x) * ms
	case chorusJuno12:
		step(0, 9.75)
		x := sin1(c.phase[0])
		d[0] = (3.5 + 0.2*x) * ms
		d[1] = (3.5 - 0.2*x) * ms
	case chorusEnsemble:
		step(0, 0.6)
		step(1, 6)
		for k := range 3 {
			o := float32(k) / 3
			d[k] = (6 + 1.6*sin1(wrap(c.phase[0]+o)) + 0.25*sin1(wrap(c.phase[1]+o))) * ms
		}
	}
	return d
}

// read returns channel ch's sound d frames back from where it is written
// next, between frames.
func (c *chorus) read(ch int, d float32) float32 {
	pos := float32(c.at) - d
	if pos < 0 {
		pos += chorusSize
	}
	j := int(pos)
	f := pos - float32(j)
	a := c.buf[ch][j&(chorusSize-1)]
	b := c.buf[ch][(j+1)&(chorusSize-1)]
	return a + (b-a)*f
}

func (c *chorus) process(l, r []float32) {
	dry := 1 - c.mix*0.5
	for at := 0; at < len(l); at += control {
		end := min(at+control, len(l))
		// The delays glide from where they were to where they are bound,
		// frame by frame, so the sweep makes no steps.
		from, to := c.delays, c.targets()
		var dd [3]float32
		for k := range dd {
			dd[k] = (to[k] - from[k]) / float32(end-at)
		}
		d := from
		for i := at; i < end; i++ {
			c.buf[0][c.at] = l[i]
			c.buf[1][c.at] = r[i]
			var wl, wr float32
			switch c.kind {
			case chorusSoft:
				wl, wr = c.read(0, d[0]), c.read(1, d[1])
			case chorusEnsemble:
				// One sound, as a string machine's, into three lines.
				m := (l[i] + r[i]) * 0.5
				c.buf[0][c.at] = m
				t0, t1, t2 := c.read(0, d[0]), c.read(0, d[1]), c.read(0, d[2])
				wl = c.dark[0].lp(t0*0.7 + t1*0.45)
				wr = c.dark[1].lp(t2*0.7 + t1*0.45)
			default:
				wl = c.dark[0].lp(c.read(0, d[0]))
				wr = c.dark[1].lp(c.read(1, d[1]))
			}
			l[i] = l[i]*dry + wl*c.mix
			r[i] = r[i]*dry + wr*c.mix
			c.at = (c.at + 1) & (chorusSize - 1)
			for k := range d {
				d[k] += dd[k]
			}
		}
		c.delays = to
	}
}

// inserts are a track's effects, in the order they run: its highpass and
// lowpass, its shape, a drive that rounds the sound off; its crush, to
// fewer bits; its coarse, to a lower sample rate; and its chorus.
type inserts struct {
	hp, lp     [2]svf
	hpOn, lpOn bool
	shape      float32
	dist       distortion
	ring       float32
	ringDt     float32
	ringPh     float32
	smash      *smasher
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
	in.dist.set(t.Distort, t.DistortType)
	in.ring = float32(min(max(t.Ring, 0), 1))
	hz := t.RingHz
	if hz <= 0 {
		hz = 440
	}
	in.ringDt = float32(hz / rate)
	if t.Smash > 0 {
		if in.smash == nil {
			in.smash = &smasher{}
		}
		in.smash.mix = float32(min(t.Smash, 1))
	} else {
		in.smash = nil
	}
	in.crush = 0
	if t.Crush > 0 {
		in.crush = float32(math.Exp2(t.Crush - 1))
	}
	in.coarse = max(t.Coarse, 0)
	if t.Chorus > 0 {
		kind := chorusKinds[t.ChorusType]
		if in.chorus == nil || in.chorus.kind != kind {
			in.chorus = newChorus(kind)
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
	if in.dist.on {
		in.dist.process(l, r)
	}
	if in.ring > 0 {
		for i := range l {
			m := 1 - in.ring + in.ring*sin1(in.ringPh)
			l[i] *= m
			r[i] *= m
			in.ringPh += in.ringDt
			if in.ringPh >= 1 {
				in.ringPh--
			}
		}
	}
	if in.smash != nil {
		in.smash.process(l, r)
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

// tweaker turns the whole mix by a song's Tweak: its width, tilt and
// bass before the compressor, and its make-up, drive and lo-fi after.
type tweaker struct {
	// on says any knob is turned; space scales the rooms and echoes.
	on    bool
	space float32
	// side scales the sides, where width is turned.
	side  float32
	width bool
	// The tilt: split splits the sound at 800 Hz, the bottom taken by
	// lowG and the top by highG.
	tilt        bool
	split       [2]onePole
	lowG, highG float32
	// The bass's shelf: shelf finds the bass, lifted by bassG.
	bass  bool
	shelf [2]onePole
	bassG float32
	// punch is how hard the compressor squeezes, and makeup the level it
	// squeezes away, on average, in decibels, which punch makes up.
	punch  float32
	makeup float32
	// drive pushes the mix into a soft clip, and driveOut brings it
	// back.
	drive, driveOut float32
	// lo-fi: steps are the levels a sample may take, hold how many
	// frames each is held, and lp and hp its cuts.
	lofi   bool
	steps  float32
	hold   int
	held   int
	holdV  [2]float32
	lp, hp [2][2]svf
}

// set readies the tweaker for t.
func (tw *tweaker) set(t Tweak) {
	clamp := func(x, lo, hi float64) float64 { return min(max(x, lo), hi) }
	tone, bass, space := clamp(t.Tone, -1, 1), clamp(t.Bass, -1, 1), clamp(t.Space, -1, 1)
	punch, width, drive, lofi := clamp(t.Punch, 0, 1), clamp(t.Width, -1, 1), clamp(t.Drive, 0, 1), clamp(t.LoFi, 0, 1)
	tw.on = tone != 0 || bass != 0 || space != 0 || punch != 0 || width != 0 || drive != 0 || lofi != 0
	tw.space = 1
	if space < 0 {
		tw.space = float32(1 + space)
	} else {
		tw.space = float32(1 + 2*space)
	}
	tw.width, tw.side = width != 0, float32(1+width)
	tw.tilt = tone != 0
	tw.lowG, tw.highG = float32(dbGain(-6*tone)), float32(dbGain(6*tone))
	for i := range 2 {
		tw.split[i].set(800)
		tw.shelf[i].set(120)
		for k := range 2 {
			tw.lp[i][k].set(float32(16000*math.Exp2(-2.7*lofi)), 0.1)
			tw.hp[i][k].set(float32(20*math.Exp2(3.6*lofi)), 0.1)
		}
	}
	tw.bass, tw.bassG = bass != 0, float32(dbGain(9*bass)-1)
	tw.punch = float32(punch)
	tw.drive = float32(1 + 4*drive)
	tw.driveOut = 1 / float32(math.Sqrt(float64(tw.drive)))
	tw.lofi = lofi > 0
	tw.steps = float32(math.Exp2(15 - 11*lofi))
	tw.hold = 1 + int(math.Round(5*lofi))
}

// pre turns the mix's width, tilt and bass, before the compressor.
func (tw *tweaker) pre(l, r []float32) {
	if tw.width {
		for i := range l {
			m, s := (l[i]+r[i])*0.5, (l[i]-r[i])*0.5*tw.side
			l[i], r[i] = m+s, m-s
		}
	}
	if tw.tilt {
		for i := range l {
			lo := tw.split[0].lp(l[i])
			l[i] = lo*tw.lowG + (l[i]-lo)*tw.highG
			lo = tw.split[1].lp(r[i])
			r[i] = lo*tw.lowG + (r[i]-lo)*tw.highG
		}
	}
	if tw.bass {
		for i := range l {
			l[i] += tw.shelf[0].lp(l[i]) * tw.bassG
			r[i] += tw.shelf[1].lp(r[i]) * tw.bassG
		}
	}
}

// post makes up the level the compressor took, where punched, drives,
// and makes the mix lo-fi, after the compressor, which takes reduction
// decibels away now.
func (tw *tweaker) post(l, r []float32, reduction float32) {
	if tw.punch > 0 {
		// The make-up follows the reduction over a second or so, so it
		// holds the mix's level as the squeeze holds its peaks down, and
		// lifts it a little more.
		tw.makeup += (reduction - tw.makeup) * min(float32(len(l))/rate, 1)
		g := float32(dbGain(float64(tw.makeup + 2*tw.punch)))
		for i := range l {
			l[i] *= g
			r[i] *= g
		}
	}
	if tw.drive != 1 {
		for i := range l {
			l[i] = softClip(l[i]*tw.drive) * tw.driveOut
			r[i] = softClip(r[i]*tw.drive) * tw.driveOut
		}
	}
	if tw.lofi {
		for i := range l {
			if tw.held == 0 {
				tw.holdV[0] = float32(math.Round(float64(l[i]*tw.steps))) / tw.steps
				tw.holdV[1] = float32(math.Round(float64(r[i]*tw.steps))) / tw.steps
			}
			tw.held = (tw.held + 1) % tw.hold
			for ch, x := range tw.holdV {
				for k := range 2 {
					x, _, _ = tw.lp[ch][k].step(x)
					_, _, x = tw.hp[ch][k].step(x)
				}
				if ch == 0 {
					l[i] = x
				} else {
					r[i] = x
				}
			}
		}
	}
}

// The kinds of distortion.
const (
	distFuzz = iota
	distAmp
	distFold
)

var distortKinds = map[string]int{"": distFuzz, "fuzz": distFuzz, "amp": distAmp, "fold": distFold}

// distortion is a track's distortion: a fuzz's hard clip, a guitar
// amplifier's and its cabinet's, or a wavefolder's.
type distortion struct {
	on    bool
	kind  int
	gain  float32
	level float32
	// The amp's: tight takes the lows out before the valves, and dc
	// what their lean leaves; the cabinet cuts the lows and the highs
	// and sings at 1.6 kHz.
	tight, dc  [2]onePole
	cabLP      [2][2]svf
	cabHP, mid [2]svf
}

func (d *distortion) set(amount float64, kind string) {
	amount = min(max(amount, 0), 1)
	d.on = amount > 0
	if !d.on {
		return
	}
	k := distortKinds[kind]
	if k != d.kind || d.gain == 0 {
		*d = distortion{kind: k}
		for ch := range 2 {
			d.tight[ch].set(320)
			d.dc[ch].set(18)
			d.cabLP[ch][0].set(4500, 0.2)
			d.cabLP[ch][1].set(4500, 0.2)
			d.cabHP[ch].set(85, 0.1)
			d.mid[ch].set(1600, 0.5)
		}
	}
	d.on = true
	a := float32(amount)
	switch k {
	case distFuzz:
		d.gain, d.level = 1+80*a*a, 1-0.5*a
	case distAmp:
		d.gain, d.level = 1+50*a*a, 0.75-0.3*a
	case distFold:
		d.gain, d.level = 1+6*a, 0.8
	}
}

func (d *distortion) process(l, r []float32) {
	for ch, x := range [2][]float32{l, r} {
		switch d.kind {
		case distFuzz:
			for i, v := range x {
				v *= d.gain
				x[i] = min(max(v, -1), 1) * d.level
			}
		case distAmp:
			const lean = 0.25
			bias := softClip(lean)
			for i, v := range x {
				// The lows tightened, so the chug stays clear.
				v = (v - 0.7*d.tight[ch].lp(v)) * d.gain
				// Valves clip one way sooner than the other.
				v = softClip(v+lean) - bias
				v -= d.dc[ch].lp(v)
				_, _, v = d.cabHP[ch].step(v)
				v, _, _ = d.cabLP[ch][0].step(v)
				v, _, _ = d.cabLP[ch][1].step(v)
				_, m, _ := d.mid[ch].step(v)
				x[i] = (v + 0.6*m) * d.level
			}
		case distFold:
			for i, v := range x {
				// A sine of the sound folds it back on itself each
				// time it passes full scale.
				x[i] = sin1(wrap(v*d.gain*0.25)) * d.level
			}
		}
	}
}

// smasher mixes a smashed copy of a track under it: compressed as an
// 1176 is with every ratio's button in, its attack near instant and its
// release quick, so it pumps, and then driven.
type smasher struct {
	mix  float32
	env  float32
	gain float32
}

func (s *smasher) process(l, r []float32) {
	const (
		threshold = 0.05
		att       = 0.5
		rel       = 0.0004
	)
	if s.gain == 0 {
		s.gain = 1
	}
	for i := range l {
		x := max(abs32(l[i]), abs32(r[i]))
		if x > s.env {
			s.env += (x - s.env) * att
		} else {
			s.env += (x - s.env) * rel * 4
		}
		// Far over the threshold, at a ratio of 20 or more: the level
		// held near the threshold, and made up to near full scale.
		g := float32(1)
		if s.env > threshold {
			g = threshold / s.env
		}
		s.gain += (g - s.gain) * 0.02
		w := s.gain * 24
		wl, wr := softClip(l[i]*w), softClip(r[i]*w)
		l[i] += (wl*0.6 - l[i]*0.4) * s.mix
		r[i] += (wr*0.6 - r[i]*0.4) * s.mix
	}
}
