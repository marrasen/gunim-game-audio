package synth

import (
	"math"
)

// The most oscillators a patch plays, and copies of each.
const (
	maxOsc     = 4
	maxUnison  = 9
	pluckFrame = 2048
)

// A note is what a voice is asked to play.
type note struct {
	pitch float32
	vel   float32
	// gate is how many frames the note is held, before it is let go.
	gate int64
	// pan, cutoff and res are the note's own, where set: cutoff in
	// hertz, 0 for the patch's, res from 0 to 1, -1 for the patch's.
	pan    float32
	cutoff float32
	res    float32
	vowel  byte
	// chord are the semitones over pitch a chord played as one voice's
	// arpeggio steps through, nchord of them.
	chord  [8]int8
	nchord int
}

// voice plays a note of a synth or pluck patch.
type voice struct {
	p      *patch
	on     bool
	n      note
	age    uint64
	gate   int64
	pitch  float32
	glide  float32
	amp    env
	fenv   env
	phase  [maxOsc][maxUnison]float32
	mphase [maxOsc][maxUnison]float32
	fl, fr svf
	gl, gr svf
	lfoP   []float32
	lfoR   []float32
	frames int64
	form   formant
	voc    vocoder
	// noise2 are the oscillators' SID noise, and nesN their NES noise.
	noise2 [maxOsc][maxUnison]lfsr
	nesN   [maxOsc][maxUnison]nesNoise
	// chipAmp is the level a chip patch holds until its next frame.
	chipAmp float32
	// vowel is the vowel the formant is aimed at, 0 for none yet.
	vowel byte
	noise *rng
	// pan is where the note sits, LFO and all, this control block.
	pan float32
	// drift is how far the note is off its pitch, in semitones, as an
	// analogue oscillator drifts.
	drift float32
	// hz is the pitch heard, from pitchAt, and cut and res the filter's
	// tuning, kept to tune it again only once they move.
	hz, pitchAt, cut, res float32
	// The plucked string: its delay line, where it is read, its length,
	// and the body it sounds through.
	str    []float32
	strAt  int
	strLen float32
	strZ   float32
	strG   float32
	body   [2]svf
}

func newVoice(seed uint64) *voice {
	return &voice{noise: newRand(seed)}
}

// start plays n on patch p.
func (v *voice) start(p *patch, n note, age uint64) {
	retrig := !v.on || v.p != p || p.poly > 1 || v.amp.stage == envRelease || v.amp.idle()
	if v.p != p {
		v.on = false
	}
	v.p, v.n, v.age, v.gate = p, n, age, n.gate
	src := p.src
	v.amp.set(src.Amp)
	v.fenv.set(src.FilterEnv)
	if !v.on || p.poly > 1 || src.Glide <= 0 {
		v.pitch = n.pitch
	}
	v.glide = 1
	if src.Glide > 0 && p.poly == 1 {
		v.glide = float32(1 - math.Exp(-float64(control)/(src.Glide*rate)))
	}
	if retrig {
		v.amp.gate()
		v.fenv.gate()
		v.frames = 0
		if !v.on {
			v.cut, v.hz = 0, 0
			v.fl.reset()
			v.fr.reset()
			v.gl.reset()
			v.gr.reset()
			v.amp.v, v.fenv.v = 0, 0
			for i := range v.phase {
				for u := range v.phase[i] {
					v.phase[i][u] = float32(v.noise.float())
					v.mphase[i][u] = 0
				}
			}
		}
		if len(v.lfoP) != len(p.lfo) {
			v.lfoP = make([]float32, len(p.lfo))
			v.lfoR = make([]float32, len(p.lfo))
		}
		for i := range v.lfoP {
			v.lfoP[i] = 0
		}
	}
	vowel := n.vowel
	if vowel == 0 {
		vowel = p.vowel
	}
	if vowel != 0 {
		v.aim(vowel)
	}
	v.drift = 0
	if src.Drift > 0 {
		v.drift = float32(src.Drift/100) * v.noise.bipolar()
	}
	if p.kind == kindPluck {
		v.pluck()
	}
	v.on = true
}

// aim aims the voice's formants, or its vocoder's bands, at vowel.
func (v *voice) aim(vowel byte) {
	if v.p.vocoder {
		v.voc.set(vowel)
	} else {
		v.form.set(vowel)
	}
	v.vowel = vowel
}

// pluck plucks the string afresh: a burst of noise as long as the
// string, brighter or darker, which then rings round the delay line.
func (v *voice) pluck() {
	if v.str == nil {
		v.str = make([]float32, pluckFrame)
	}
	pl := v.p.src.Pluck
	hz := float32(noteHz(float64(v.n.pitch)))
	v.strLen = min(max(rate/hz, 2), pluckFrame-2)
	decay := pl.Decay
	if decay <= 0 {
		decay = 1
	}
	// Each trip round the string loses as much as it must to fall 60 dB
	// in decay seconds.
	v.strG = float32(math.Exp(-6.9 / (decay * float64(hz))))
	bright := float32(pl.Bright)
	if bright <= 0 {
		bright = 0.6
	}
	var lp float32
	n := int(v.strLen) + 1
	for i := range pluckFrame {
		if i < n {
			lp += (v.noise.bipolar() - lp) * (0.08 + 0.92*bright*bright)
			v.str[i] = lp * v.n.vel
		} else {
			v.str[i] = 0
		}
	}
	// The string is read a length behind where it is written, which is
	// where the burst starts.
	v.strAt, v.strZ = n&(pluckFrame-1), 0
	v.body[0].set(280, 0.6)
	v.body[1].set(1150, 0.55)
}

// release lets go of the note.
func (v *voice) release() {
	v.amp.release()
	v.fenv.release()
	v.gate = 0
}

// quiet reports whether the voice is free or letting go, so that a new
// note takes it before one still held.
func (v *voice) quiet() bool { return !v.on || v.amp.stage == envRelease }

// renderCtx is what a voice needs of the song as it plays.
type renderCtx struct {
	// beatHz is beats a second, for LFOs in time with the song.
	beatHz float64
	// buf is room to make sound in.
	l, r []float32
}

// render adds n frames of the voice's sound to outL and outR.
func (v *voice) render(outL, outR []float32, ctx *renderCtx) {
	n := len(outL)
	p := v.p
	src := p.src
	for at := 0; at < n && v.on; at += control {
		m := min(control, n-at)
		// Modulation, once a control block.
		var lPitch, lCut, lAmp, lWidth, lPan float32
		for i, l := range p.lfo {
			hz := l.Hz
			if l.Beats > 0 {
				hz = ctx.beatHz / l.Beats
			}
			v.lfoP[i] += float32(hz * control / rate)
			if v.lfoP[i] >= 1 {
				v.lfoP[i] -= float32(math.Floor(float64(v.lfoP[i])))
				v.lfoR[i] = v.noise.bipolar()
			}
			x := lfoAt(l.wave, v.lfoP[i], v.lfoR[i]) * float32(l.Depth)
			if l.Delay > 0 {
				t := float64(v.frames)/rate/l.Delay - 1
				x *= float32(min(max(t, 0), 1))
			}
			switch l.to {
			case toPitch:
				lPitch += x
			case toCutoff:
				lCut += x
			case toAmp:
				lAmp += x
			case toWidth:
				lWidth += x
			case toPan:
				lPan += x
			}
		}
		v.pitch += (v.n.pitch - v.pitch) * v.glide
		if b := p.src.Bend; b != nil && b.Time > 0 {
			lPitch += float32(b.Semis * max(0, 1-float64(v.frames)/(b.Time*rate)))
		}
		if pitch := v.pitch + lPitch + v.drift; pitch != v.pitchAt || v.hz == 0 {
			v.pitchAt, v.hz = pitch, float32(noteHz(float64(pitch)))
		}
		hz := v.hz
		if step := v.arpStep(); step != 0 {
			hz *= exp2(step / 12)
		}
		if p.filter != filterNone {
			f := src.Filter
			cut := float32(f.Cutoff)
			if v.n.cutoff > 0 {
				cut = v.n.cutoff
			}
			if cut <= 0 {
				cut = 20000
			}
			oct := v.fenv.v*float32(f.Env) + lCut + float32(f.Key)*(v.pitch-60)/12 + float32(f.Vel)*(v.n.vel-1)
			cut *= exp2(oct)
			res := float32(f.Res)
			if v.n.res >= 0 {
				res = v.n.res
			}
			// The channels share their tuning, worked out once, and
			// only as the cutoff moves, as it does not while a note is
			// held with nothing moving it.
			if d := cut - v.cut; d > v.cut*0.002 || d < -v.cut*0.002 || res != v.res {
				v.cut, v.res = cut, res
				v.fl.set(cut, res)
				v.fr.tuneAs(&v.fl)
				if p.filter == filterLP24 {
					v.gl.set(cut, res*0.5)
					v.gr.tuneAs(&v.gl)
				}
			}
		}
		// The vowel is the note's, or else the patch's as it is now, so
		// a patch's vowel turned off or changed is heard at once.
		vowel := v.n.vowel
		if vowel == 0 {
			vowel = p.vowel
		}
		sing := vowel != 0
		if sing {
			if vowel != v.vowel {
				v.aim(vowel)
			}
			if p.vocoder {
				v.voc.tune()
			} else {
				v.form.tune()
			}
		}
		v.pan = min(max(v.n.pan+lPan, -1), 1)
		pl, pr := panGains(v.pan)
		gain := v.n.vel * p.gain * (1 + lAmp)
		bl, br := ctx.l[:m], ctx.r[:m]
		clear(bl)
		clear(br)
		if p.kind == kindPluck {
			v.renderString(bl, hz)
			copy(br, bl)
		} else {
			v.renderOsc(bl, br, hz, lWidth)
		}
		if src.Noise > 0 {
			nl := float32(src.Noise)
			for i := range bl {
				s := v.noise.bipolar() * nl
				bl[i] += s
				br[i] += s
			}
		}
		if src.Drive > 0 {
			d := 1 + float32(src.Drive)*4
			norm := 1 / softClip(d)
			for i := range bl {
				bl[i] = softClip(bl[i]*d) * norm
				br[i] = softClip(br[i]*d) * norm
			}
		}
		v.filter(bl, br)
		if sing {
			for i := range bl {
				x := (bl[i] + br[i]) * 0.5
				var s float32
				if p.vocoder {
					s = v.voc.step(x)
				} else {
					s = v.form.step(x)
				}
				bl[i], br[i] = s, s
			}
		}
		for i := range m {
			if v.gate > 0 {
				v.gate--
				if v.gate == 0 {
					v.amp.release()
					v.fenv.release()
				}
			}
			a := v.amp.step()
			if c := p.src.Chip; c != nil {
				// A console sets its level once a frame, in steps.
				if (v.frames+int64(i))%p.chipFrame == 0 {
					v.chipAmp = float32(math.Round(float64(a)*float64(p.chipLevels))) / float32(p.chipLevels)
				}
				a = v.chipAmp
			}
			a *= gain
			v.fenv.step()
			outL[at+i] += bl[i] * a * pl
			outR[at+i] += br[i] * a * pr
		}
		v.frames += int64(m)
		if v.amp.idle() {
			v.on = false
		}
	}
}

// renderOsc makes the oscillators' sound at hz into l and r.
func (v *voice) renderOsc(l, r []float32, hz, lWidth float32) {
	// The first copy of each oscillator's phase as the block started,
	// and its step, for an oscillator synced or ringed to it.
	var prevPh, prevDt float32
	for oi := range v.p.osc {
		o := &v.p.osc[oi]
		base := hz * o.ratio / rate
		startPh, startDt := v.phase[oi][0], base*o.detunes[0]
		if o.Sync || o.Ring {
			v.renderLinked(l, r, oi, base, prevPh, prevDt, lWidth)
			prevPh, prevDt = startPh, startDt
			continue
		}
		prevPh, prevDt = startPh, startDt
		width := min(max(float32(o.Width)+lWidth, 0.05), 0.95)
		var index float32
		if o.wave == oscFM {
			t := float64(v.frames) / rate
			sus := o.Sustain
			d := o.Decay
			if d <= 0 {
				d = 1e9
			}
			index = float32(o.Index * (sus + (1-sus)*math.Exp(-t/d)))
		}
		for u := range o.unison {
			dt := base * o.detunes[u]
			if dt >= 0.5 {
				continue
			}
			gl, gr := o.gl[u], o.gr[u]
			ph := v.phase[oi][u]
			switch o.wave {
			case oscSaw:
				for i := range l {
					s := 2*ph - 1 - polyBLEP(ph, dt)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
				}
			case oscSquare:
				for i := range l {
					s := float32(-1)
					if ph < width {
						s = 1
					}
					s += polyBLEP(ph, dt)
					t := ph - width
					if t < 0 {
						t++
					}
					s -= polyBLEP(t, dt)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
				}
			case oscTri:
				for i := range l {
					s := 1 - 4*abs32(ph-0.5)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
				}
			case oscSine:
				for i := range l {
					s := sin1(ph)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
				}
			case oscFM:
				mp := v.mphase[oi][u]
				mdt := dt * float32(o.Ratio)
				for i := range l {
					s := sin1(wrap(ph + index*sin1(mp)))
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
					mp += mdt
					if mp >= 1 {
						mp -= float32(int(mp))
					}
				}
				v.mphase[oi][u] = mp
			case oscNESNoise:
				n := &v.nesN[oi][u]
				n.short = o.Wave == "nesmetal"
				for i := range l {
					s := n.at(ph)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
						n.step = -1
					}
				}
			case oscNoise:
				n := &v.noise2[oi][u]
				for i := range l {
					s := n.at(ph)
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
						n.step = -1
					}
				}
			case oscSawTri, oscPulseTri, oscPulseSaw, oscTable:
				t := o.table
				for i := range l {
					s := t[int(ph*tableSize)&(tableSize-1)]
					l[i] += s * gl
					r[i] += s * gr
					ph += dt
					if ph >= 1 {
						ph--
					}
				}
			}
			v.phase[oi][u] = ph
		}
	}
}

// renderString rings the plucked string into out: each sample is the
// average of the two before it a string's length back, a little lower,
// which softens the string as it rings, as a real one softens.
func (v *voice) renderString(out []float32, hz float32) {
	_ = hz
	body := float32(v.p.src.Pluck.Body)
	size := float32(pluckFrame)
	for i := range out {
		// Read the string a length back, between samples.
		pos := float32(v.strAt) - v.strLen
		if pos < 0 {
			pos += size
		}
		j := int(pos)
		f := pos - float32(j)
		a := v.str[j&(pluckFrame-1)]
		b := v.str[(j+1)&(pluckFrame-1)]
		s := a + (b-a)*f
		y := (s + v.strZ) * 0.5 * v.strG
		v.strZ = s
		v.str[v.strAt] = y
		v.strAt = (v.strAt + 1) & (pluckFrame - 1)
		if body > 0 {
			_, b0, _ := v.body[0].step(y)
			_, b1, _ := v.body[1].step(y)
			y = y*(1-body*0.5) + (b0*1.4+b1)*body
		}
		out[i] = y * 1.6
	}
}

// filter runs l and r through the patch's filter.
func (v *voice) filter(l, r []float32) {
	switch v.p.filter {
	case filterLP:
		for i := range l {
			l[i], _, _ = v.fl.step(l[i])
			r[i], _, _ = v.fr.step(r[i])
		}
	case filterLP24:
		for i := range l {
			a, _, _ := v.fl.step(l[i])
			b, _, _ := v.fr.step(r[i])
			l[i], _, _ = v.gl.step(a)
			r[i], _, _ = v.gr.step(b)
		}
	case filterHP:
		for i := range l {
			_, _, l[i] = v.fl.step(l[i])
			_, _, r[i] = v.fr.step(r[i])
		}
	case filterBP:
		for i := range l {
			_, l[i], _ = v.fl.step(l[i])
			_, r[i], _ = v.fr.step(r[i])
		}
	case filterSIDLP, filterSIDBP, filterSIDHP, filterSIDNotch:
		m := v.p.filter
		for i := range l {
			l[i] = sidStep(&v.fl, l[i], m)
			r[i] = sidStep(&v.fr, r[i], m)
		}
	}
}

// arpStep returns the semitones the arpeggio is on now, over the note:
// the chord's notes in turn where the note is a chord played as one
// voice, else the patch's steps.
func (v *voice) arpStep() float32 {
	p := v.p
	if p.arpHz <= 0 {
		return 0
	}
	i := int(float64(v.frames) / rate * p.arpHz)
	if v.n.nchord > 0 {
		return float32(v.n.chord[i%v.n.nchord])
	}
	if len(p.arpSteps) > 0 {
		return p.arpSteps[i%len(p.arpSteps)]
	}
	return 0
}

// renderLinked makes oscillator oi's sound where it is synced or ringed
// to the one before it, sample by sample: the one before it is followed
// from prevPh, stepping prevDt, so each cycle it starts restarts this
// one, or each half of its cycle turns this one over.
func (v *voice) renderLinked(l, r []float32, oi int, base, prevPh, prevDt, lWidth float32) {
	o := &v.p.osc[oi]
	width := min(max(float32(o.Width)+lWidth, 0.05), 0.95)
	for u := range o.unison {
		dt := base * o.detunes[u]
		if dt >= 0.5 {
			continue
		}
		gl, gr := o.gl[u], o.gr[u]
		ph, pp := v.phase[oi][u], prevPh
		for i := range l {
			s := v.waveAt(o, oi, u, ph, dt, width)
			if o.Ring && pp >= 0.5 {
				s = -s
			}
			l[i] += s * gl
			r[i] += s * gr
			ph += dt
			if ph >= 1 {
				ph--
				v.noise2[oi][u].step = -1
			}
			pp += prevDt
			if pp >= 1 {
				pp--
				if o.Sync {
					ph = pp * dt / max(prevDt, 1e-9)
				}
			}
		}
		v.phase[oi][u] = ph
	}
}

// waveAt returns oscillator o's wave at phase ph, a sample at a time,
// for an oscillator that follows another.
func (v *voice) waveAt(o *osc, oi, u int, ph, dt, width float32) float32 {
	switch o.wave {
	case oscSaw:
		return 2*ph - 1 - polyBLEP(ph, dt)
	case oscSquare:
		s := float32(-1)
		if ph < width {
			s = 1
		}
		t := ph - width
		if t < 0 {
			t++
		}
		return s + polyBLEP(ph, dt) - polyBLEP(t, dt)
	case oscTri:
		return 1 - 4*abs32(ph-0.5)
	case oscNoise:
		return v.noise2[oi][u].at(ph)
	case oscNESNoise:
		n := &v.nesN[oi][u]
		n.short = o.Wave == "nesmetal"
		return n.at(ph)
	case oscSawTri, oscPulseTri, oscPulseSaw, oscTable:
		return o.table[int(ph*tableSize)&(tableSize-1)]
	case oscFM:
		mp := v.mphase[oi][u]
		v.mphase[oi][u] = wrap(mp + dt*float32(o.Ratio))
		return sin1(wrap(ph + float32(o.Index)*sin1(mp)))
	}
	return sin1(ph)
}

// voices are a track's voices, which its notes take in turn.
type voices struct {
	vs  []*voice
	age uint64
}

func newVoices(n int, seed uint64) *voices {
	vv := &voices{}
	for i := range n {
		vv.vs = append(vv.vs, newVoice(seed+uint64(i)*7919))
	}
	return vv
}

// play gives n to a voice: a mono patch's one voice, or one at rest, or
// one letting go, or else the oldest.
func (vv *voices) play(p *patch, n note) {
	vv.age++
	if p.poly == 1 {
		vv.vs[0].start(p, n, vv.age)
		return
	}
	var best *voice
	for _, v := range vv.vs[:min(p.poly, len(vv.vs))] {
		switch {
		case !v.on:
			v.start(p, n, vv.age)
			return
		case best == nil,
			v.quiet() && !best.quiet(),
			v.quiet() == best.quiet() && v.age < best.age:
			best = v
		}
	}
	best.start(p, n, vv.age)
}

// releaseAll lets go of every note.
func (vv *voices) releaseAll() {
	for _, v := range vv.vs {
		if v.on {
			v.release()
		}
	}
}

// render adds the voices' sound to l and r.
func (vv *voices) render(l, r []float32, ctx *renderCtx) bool {
	sounding := false
	for _, v := range vv.vs {
		if v.on {
			v.render(l, r, ctx)
			sounding = true
		}
	}
	return sounding
}
