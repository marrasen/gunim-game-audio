package calls

import "math"

// The vowels' first three resonances, their formants, in hertz, as a
// grown man's voice has them; a syllable's size scales them up, as a
// smaller throat lifts them.
var vowels = map[byte][3]float64{
	'a': {800, 1200, 2600},
	'e': {500, 1850, 2600},
	'i': {300, 2250, 3000},
	'o': {480, 850, 2500},
	'u': {320, 780, 2300},
	// The consonants a voice holds a moment: an "l", an "n" and an
	// "m", their mouths nearly closed.
	'l': {360, 1300, 2700},
	'n': {280, 1700, 2600},
	'm': {280, 1000, 2300},
}

// A mark is a vowel at a point of a syllable, from 0 to 1.
type mark struct {
	u float64
	v byte
}

// glide returns a mouth moving through marks, easing from each to the
// next.
func glide(marks ...mark) func(u float64) [3]float64 {
	return func(u float64) [3]float64 {
		if u <= marks[0].u {
			return vowels[marks[0].v]
		}
		for i := 1; i < len(marks); i++ {
			if u <= marks[i].u {
				t := (u - marks[i-1].u) / max(marks[i].u-marks[i-1].u, 1e-9)
				return vowel(marks[i-1].v, marks[i].v, smooth(t))
			}
		}
		return vowels[marks[len(marks)-1].v]
	}
}

// vowel returns the formants t of the way from vowel a to vowel b,
// moving by ratios, as the ear hears them move.
func vowel(a, b byte, t float64) [3]float64 {
	va, vb := vowels[a], vowels[b]
	t = min(max(t, 0), 1)
	var f [3]float64
	for i := range f {
		f[i] = va[i] * math.Pow(vb[i]/va[i], t)
	}
	return f
}

// The formants' widths, in hertz, before a syllable's size, and their
// levels.
var (
	formantWidth = [3]float64{90, 110, 170}
	formantLevel = [3]float64{1, 0.8, 0.5}
)

// A resonance is a body's own pitch that a voice rings, as a frog's
// throat sac does.
type resonance struct{ hz, width, level float64 }

// A syllable is a sound of a voice, made as a throat makes it: a buzz
// at a pitch, of every harmonic of it, and breath, both shaped by the
// mouth's resonances into a vowel. Each of its contours is a function
// of how far through the syllable it is, from 0 to 1.
type syllable struct {
	at, dur float64
	// pitch is the buzz's pitch, in hertz; level its level; breath the
	// breath's, with no buzz needed under it.
	pitch, level, breath func(u float64) float64
	// mouth is the vowel's formants, before size.
	mouth func(u float64) [3]float64
	// roll, when set, turns the whole sound up and down by the time,
	// in seconds from the call's start, as a rolled r does.
	roll func(t float64) float64
	// size scales the formants: above 1 a smaller creature.
	size float64
	// tilt is how fast the harmonics fall, each k-th at k to the
	// minus tilt: 1 bright, 2 dull.
	tilt float64
	// body is a resonance more, where set.
	body *resonance
	// wander is how far the pitch wanders, as a fraction.
	wander float64
	// top is the highest harmonic's pitch, 14 kHz where unset: a
	// lower top makes a voice quicker to make, as in a crowd.
	top float64
}

// block is how many samples a syllable's contours hold for, the
// harmonics' levels gliding between.
const block = 32

// sing adds s into out, which it grows to hold it.
func sing(out *[]float64, s syllable, r *rng) {
	n := int(s.dur * rate)
	start := int(s.at * rate)
	if n <= 0 {
		return
	}
	if need := start + n; need > len(*out) {
		*out = append(*out, make([]float64, need-len(*out))...)
	}
	buf := (*out)[start : start+n]
	size := s.size
	if size == 0 {
		size = 1
	}
	wander := newDrift(r, 14)
	var (
		phase      float64
		prev, next []float64
		breath     [3]biquad
	)
	// levels returns the harmonics' levels at u, of a buzz at hz: the
	// source's slope through the formants' peaks, scaled to a steady
	// loudness so a vowel's change shapes the sound without swelling it.
	levels := func(dst []float64, u, hz float64) []float64 {
		top := min(14000.0, 0.45*rate)
		if s.top > 0 {
			top = min(top, s.top)
		}
		k := min(max(int(top/hz), 1), len(phases))
		dst = dst[:0]
		f := s.mouth(u)
		var power float64
		for h := 1; h <= k; h++ {
			fh := float64(h) * hz
			env := 0.02
			for i := range f {
				env += formantLevel[i] * peak(fh, f[i]*size, formantWidth[i]*math.Sqrt(size))
			}
			if s.body != nil {
				env += s.body.level * peak(fh, s.body.hz, s.body.width)
			}
			a := env * math.Pow(float64(h), -s.tilt)
			dst = append(dst, a)
			power += a * a
		}
		g := 1 / math.Sqrt(max(power, 1e-12))
		for i := range dst {
			dst[i] *= g
		}
		return dst
	}
	hzAt := func(u float64) float64 { return s.pitch(u) * (1 + s.wander*wander.step()) }
	hz := hzAt(0)
	next = levels(next, 0, hz)
	for b := 0; b < n; b += block {
		e := min(b+block, n)
		u0, u1 := float64(b)/float64(n), float64(e)/float64(n)
		hz0, hz1 := hz, hzAt(u1)
		for range e - b - 1 {
			wander.step()
		}
		hz = hz1
		prev, next = next, prev
		next = levels(next, u1, hz1)
		lv0, lv1 := s.level(u0), s.level(u1)
		br0, br1 := s.breath(u0), s.breath(u1)
		f := s.mouth(u0)
		for i := range breath {
			breath[i].bandPass(f[i]*size, 1.6*formantWidth[i]*math.Sqrt(size))
		}
		k := min(len(prev), len(next))
		for j := b; j < e; j++ {
			t := float64(j-b) / float64(e-b)
			phase += 2 * math.Pi * (hz0 + (hz1-hz0)*t) / rate
			if phase > 2*math.Pi {
				phase -= 2 * math.Pi
			}
			// Each harmonic's sine and cosine by their recurrences,
			// from the first's, turned to its phase.
			c2 := 2 * math.Cos(phase)
			s1, s0 := math.Sin(phase), 0.0
			k1, k0 := math.Cos(phase), 1.0
			var v float64
			for h := range k {
				a := prev[h] + (next[h]-prev[h])*t
				v += a * (s1*phases[h][0] + k1*phases[h][1])
				s1, s0 = c2*s1-s0, s1
				k1, k0 = c2*k1-k0, k1
			}
			v *= lv0 + (lv1-lv0)*t
			if br := br0 + (br1-br0)*t; br > 0 {
				w := r.norm()
				var nz float64
				for i := range breath {
					nz += formantLevel[i] * breath[i].step(w) / math.Sqrt(math.Pi*1.6*formantWidth[i]*math.Sqrt(size)/rate)
				}
				v += br * nz * 0.45
			}
			if s.roll != nil {
				v *= s.roll(s.at + float64(j)/rate)
			}
			buf[j] += v
		}
	}
}

// phases are the harmonics' phases, as the cosine and sine of each:
// spread as Schroeder spread them, so the harmonics' peaks fall apart
// and the buzz is as loud for a lower peak, its pitch and vowel heard
// the same.
var phases = func() (ph [512][2]float64) {
	for k := range ph {
		th := math.Pi * float64(k*(k+1)) / 48
		ph[k] = [2]float64{math.Cos(th), math.Sin(th)}
	}
	return ph
}()

// peak is how a resonance at hz, width wide, passes f: 1 at hz.
func peak(f, hz, width float64) float64 {
	x := (f*f - hz*hz) / (f * width)
	return 1 / math.Sqrt(1+x*x)
}
