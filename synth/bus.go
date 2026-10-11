package synth

import "math"

// BusChannels is how many channels a [Bus] mixes.
const BusChannels = 32

// A Bus is a mixing desk: it takes each channel's sound, runs it
// through the channel's strip, sends it to a reverb, a chorus and a
// delay, and runs the sum through the master's equaliser, compressor,
// gain and limiter, as a [GMMix] sets them, metering as it goes. A [GM]
// mixes through one, and so may any player of channels, such as a
// tracker's.
//
// Each block, call Begin, then Channel for each channel that sounds,
// then End. It is not safe for use by more than one goroutine at once.
type Bus struct {
	rev  *reverb
	cho  *chorus
	dly  *delay
	comp *compressor
	lim  *limiter
	// mix is how the channels are mixed, strips the channels' strips at
	// work, and master the master equaliser's filters, each side's.
	mix    GMMix
	strips [BusChannels]stripFX
	master [5][2]biquad
	// mute silences a channel, and open is how far its sound is let
	// through, gliding to 0 as it is muted and back to 1.
	mute [BusChannels]bool
	open [BusChannels]float32
	// delayLeft counts down the frames the delay's echoes still ring for
	// once nothing is sent to it.
	delayLeft int
	meters    GMMeters
	// Gain is the whole mix's level, before the master's own.
	Gain float32
	// n is the block's length, and chorusOn and delayOn say a channel
	// sent to them in it.
	n                 int
	chorusOn, delayOn bool
	// The buffers: the mix, and the sends to the reverb, the chorus and
	// the delay.
	ml, mr, rl, rr, cl, cr, dl, dr, el, er []float32
}

// NewBus returns a desk mixing as [DefaultGMMix] does, blocks up to
// maxBlock frames long.
func NewBus() *Bus {
	b := &Bus{rev: newReverb(), cho: newChorus(chorusJuno12), dly: newDelay(), comp: newCompressor(-12, 2),
		lim: newLimiter(-1), Gain: 1}
	b.cho.mix = 1
	for _, s := range []*[]float32{&b.ml, &b.mr, &b.rl, &b.rr, &b.cl, &b.cr, &b.dl, &b.dr, &b.el, &b.er} {
		*s = make([]float32, maxBlock)
	}
	for i := range b.open {
		b.open[i] = 1
	}
	b.SetMix(DefaultGMMix())
	return b
}

// SetMix sets how the channels are mixed; see [GMMix].
func (b *Bus) SetMix(m GMMix) {
	b.mix = m
	r := m.Reverb
	b.rev.set(min(max(r.Size, 0.3), 1.5), max(r.Decay, 0.1), min(max(r.Tone, 0), 1), 0.02)
	d := m.Delay
	b.dly.set(min(max(d.Time, 0.01), 1.9), 60, min(max(d.Feedback, 0), 0.9), min(max(d.Tone, 0), 1))
	b.comp.threshold = float32(m.Comp.Threshold)
	b.comp.ratio = float32(max(m.Comp.Ratio, 1))
	b.lim.ceiling = float32(dbGain(min(m.Limiter.Ceiling, 0)))
	for i := range b.strips {
		b.strips[i].tune(m.Channels[i])
	}
	for k, band := range m.EQ {
		for side := range 2 {
			b.master[k][side].set(bandKind(k), band.Freq, band.Gain, band.Q)
		}
	}
}

// Mix returns how the channels are mixed.
func (b *Bus) Mix() GMMix { return b.mix }

// SetMute mutes channel ch, or lets it sound again; it glides in and
// out over 20 ms, so it does not click.
func (b *Bus) SetMute(ch int, mute bool) {
	if ch >= 0 && ch < BusChannels {
		b.mute[ch] = mute
	}
}

// Muted reports whether channel ch is muted.
func (b *Bus) Muted(ch int) bool { return ch >= 0 && ch < BusChannels && b.mute[ch] }

// TakeMeters returns how loud the mix has been since they were last
// taken, and starts them again.
func (b *Bus) TakeMeters() GMMeters {
	m := b.meters
	b.meters = GMMeters{}
	return m
}

// Begin starts a block of n frames, n up to maxBlock.
func (b *Bus) Begin(n int) {
	b.n = min(n, maxBlock)
	for _, s := range [][]float32{b.ml, b.mr, b.rl, b.rr, b.cl, b.cr, b.el, b.er} {
		clear(s[:b.n])
	}
	b.chorusOn, b.delayOn = false, false
}

// Channel adds channel ch's sound, l and r, to the block: through its
// strip, which changes l and r, at gain, panned to pan, from -1 to 1,
// and sending rev to the reverb and cho to the chorus, as the channel
// itself asks, before its strip's own sends.
func (b *Bus) Channel(ch int, l, r []float32, gain, pan, rev, cho float32) {
	n := b.n
	l, r = l[:n], r[:n]
	from := b.open[ch]
	to := float32(1)
	if b.mute[ch] {
		to = 0
	}
	step := float32(n) / (0.02 * rate)
	b.open[ch] = min(max(to, from-step), from+step)
	if from == 0 && b.open[ch] == 0 {
		return
	}
	st := &b.mix.Channels[ch]
	b.strips[ch].process(l, r, st.Drive)
	gain *= float32(dbGain(st.Gain))
	pl, pr := panGains(min(max(pan+float32(st.Pan), -1), 1))
	pl *= gain
	pr *= gain
	rv := rev * float32(st.Reverb)
	cv := cho * float32(st.Chorus)
	dv := float32(st.Delay)
	b.chorusOn = b.chorusOn || cv > 0
	b.delayOn = b.delayOn || dv > 0
	ml, mr, rl, rr, cl, cr, el, er := b.ml[:n], b.mr[:n], b.rl[:n], b.rr[:n], b.cl[:n], b.cr[:n], b.el[:n], b.er[:n]
	open := b.open[ch]
	var peak float32
	for j := range l {
		o := from + (open-from)*float32(j)/float32(n)
		x, y := l[j]*pl*o, r[j]*pr*o
		peak = max(peak, abs32(x), abs32(y))
		ml[j] += x
		mr[j] += y
		rl[j] += x * rv
		rr[j] += y * rv
		cl[j] += x * cv
		cr[j] += y * cv
		el[j] += x * dv
		er[j] += y * dv
	}
	b.meters.Channels[ch] = max(b.meters.Channels[ch], peak*b.Gain)
}

// End runs the block's sum through the effects and the master, and
// writes it to dst, interleaved stereo.
func (b *Bus) End(dst []float32) {
	n := b.n
	mx := &b.mix
	ml, mr, rl, rr, cl, cr, el, er := b.ml[:n], b.mr[:n], b.rl[:n], b.rr[:n], b.cl[:n], b.cr[:n], b.el[:n], b.er[:n]
	if b.chorusOn {
		// The chorus gives back half its input and its copies; a send
		// wants only the copies.
		dl, dr := b.dl[:n], b.dr[:n]
		copy(dl, cl)
		copy(dr, cr)
		b.cho.process(cl, cr)
		back := float32(mx.Chorus.Return)
		for j := range cl {
			wl, wr := (cl[j]-dl[j]*0.5)*back, (cr[j]-dr[j]*0.5)*back
			ml[j] += wl
			mr[j] += wr
			rl[j] += wl * 0.3
			rr[j] += wr * 0.3
		}
	}
	if b.delayOn {
		b.delayLeft = 12 * rate
	}
	if b.delayLeft > 0 {
		b.delayLeft -= n
		b.dly.process(el, er)
		back := float32(mx.Delay.Return)
		for j := range el {
			ml[j] += el[j] * back
			mr[j] += er[j] * back
			// The echoes sound in the room too.
			rl[j] += el[j] * back * 0.25
			rr[j] += er[j] * back * 0.25
		}
	}
	b.rev.process(rl, rr)
	back := float32(mx.Reverb.Return)
	for j := range ml {
		ml[j] = (ml[j] + rl[j]*back) * b.Gain
		mr[j] = (mr[j] + rr[j]*back) * b.Gain
	}
	// The master: its equaliser, its compressor, its gain, and its
	// limiter.
	for k := range b.master {
		if mx.EQ[k].Gain == 0 {
			continue
		}
		fl, fr := &b.master[k][0], &b.master[k][1]
		for j := range ml {
			ml[j] = fl.step(ml[j])
			mr[j] = fr.step(mr[j])
		}
	}
	if mx.Comp.On {
		b.comp.process(ml, mr)
		b.meters.Comp = max(b.meters.Comp, b.comp.reduction)
	}
	if mg := float32(dbGain(mx.Gain + mx.Comp.makeup())); mg != 1 {
		for j := range ml {
			ml[j] *= mg
			mr[j] *= mg
		}
	}
	if mx.Limiter.On {
		b.lim.process(ml, mr)
		b.meters.Limit = max(b.meters.Limit, -20*float32(math.Log10(float64(max(b.lim.gain, 1e-6)))))
	}
	for j := range ml {
		b.meters.Left = max(b.meters.Left, abs32(ml[j]))
		b.meters.Right = max(b.meters.Right, abs32(mr[j]))
		dst[2*j] = ml[j]
		dst[2*j+1] = mr[j]
	}
}
