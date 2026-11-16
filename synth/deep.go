package synth

import "math"

// A drum's deep settings, [Drum.Sweep], [Drum.SweepTime], [Drum.Click],
// [Drum.Noise] and [Drum.Spread], reach its sound as a hit's deep: a
// few multipliers, each exactly 1 where its setting is 0, so a drum
// that leaves them sounds bit for bit as it did before they were.

// DrumFields says which of a drum's deep settings it reads.
type DrumFields uint8

// The deep settings, as [DrumFields] names them.
const (
	FieldSweep DrumFields = 1 << iota
	FieldSweepTime
	FieldClick
	FieldNoise
	FieldSpread
)

// partMask names parts of a drum's sound.
type partMask uint8

const (
	partBody partMask = 1 << iota
	partNoise
	partClick
)

// deep is how a hit's deep settings change its sound: sk scales its
// pitch's dive, past its own, bm its body, nm its noise and cm its
// click; spread is the power its partials' distances from their middle
// are raised to, 1 for none. Each is 1 for the drum's own sound.
type deep struct {
	sk, bm, nm, cm, spread float32
	// note is a chip drum's last tonal note, which Sweep scales the
	// fall to.
	note float32
}

// deepKind is how a drum kind's sound takes the deep settings: the
// envelope its pitch's dive follows, pe, and how far the dive starts
// above the pitch it falls to, k plus kv times the velocity, as
// multiples of it; and the settings it reads.
type deepKind struct {
	pe     int
	k, kv  float32
	reads  DrumFields
	pitch  bool
	metal  bool
	center float32
}

// The settings each family of drums reads.
const (
	readsDive  = FieldSweep | FieldSweepTime
	readsModal = FieldSweep | FieldSweepTime | FieldClick | FieldNoise
	readsPlate = FieldClick | FieldNoise | FieldSpread
	readsChip  = FieldSweep | FieldNoise
)

// deepKinds are the drum kinds that read deep settings. A kind missing
// from it reads none.
var deepKinds = map[int]deepKind{
	drKick:         {pe: 0, k: 110.0 / 46, reads: readsDive | FieldClick, pitch: true},
	drSnare:        {reads: FieldNoise},
	drHat:          {reads: FieldNoise | FieldSpread, metal: true},
	drOHat:         {reads: FieldNoise | FieldSpread, metal: true},
	drRim:          {reads: FieldNoise},
	drTom:          {pe: 0, k: 0.55, reads: readsDive | FieldClick, pitch: true},
	drCrash:        {reads: FieldNoise | FieldSpread, metal: true},
	drRide:         {reads: FieldNoise | FieldSpread, metal: true},
	drBoom:         {pe: 0, k: 90.0 / 36, reads: readsDive | FieldNoise, pitch: true},
	drSynTom:       {pe: 0, k: 1, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drCowbell:      {reads: FieldSpread, metal: true},
	drKick909:      {pe: 0, k: 3, reads: readsDive | FieldClick, pitch: true},
	drMetal:        {reads: FieldClick | FieldSpread, metal: true},
	drTambourine:   {reads: FieldNoise | FieldSpread, metal: true},
	drTR808Kick:    {pe: 0, k: 0.4, reads: readsDive | FieldClick, pitch: true},
	drTR808Snare:   {pe: 3, k: 0.08, reads: readsDive | FieldNoise, pitch: true},
	drTR808Tom:     {pe: 0, k: 0.03 * 0.4, kv: 0.03 * 0.6, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drTR909Tom:     {pe: 0, k: 0.5 * 0.4, kv: 0.5 * 0.6, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drSDSTom:       {pe: 0, k: 0.5 * 0.4, kv: 0.5 * 0.6, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drTR808Conga:   {pe: 0, k: 0.08, reads: readsDive, pitch: true},
	drTR808Cowbell: {reads: FieldSpread, metal: true},
	drTR808Hat:     {reads: FieldSpread, metal: true},
	drTR808OHat:    {reads: FieldSpread, metal: true},
	drTR808Cymbal:  {reads: FieldSpread, metal: true},
	drTR909Kick:    {pe: 0, k: 3.2, reads: readsDive | FieldClick, pitch: true},
	drTR909Snare:   {pe: 0, k: 0.25, reads: readsDive | FieldNoise, pitch: true},
	drSDSKick:      {pe: 0, kv: 2.5, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drSDSSnare:     {pe: 0, kv: 0.8, reads: readsDive | FieldNoise | FieldClick, pitch: true},
	drModalKick:    {reads: readsModal, pitch: true},
	drModalTom:     {reads: readsModal, pitch: true},
	drModalSnare:   {reads: readsModal, pitch: true},
	drModalHat:     {reads: readsPlate, metal: true},
	drModalOHat:    {reads: readsPlate, metal: true},
	drModalCrash:   {reads: readsPlate, metal: true},
	drModalRide:    {reads: readsPlate, metal: true},
	drTR909Hat:     {reads: readsPlate, metal: true},
	drTR909OHat:    {reads: readsPlate, metal: true},
	drTR909Crash:   {reads: readsPlate, metal: true},
	drTR909Ride:    {reads: readsPlate, metal: true},
}

func init() {
	for k := drSIDKick; k <= drNESClap; k++ {
		frames, _ := chipDrum(k)
		dk := deepKind{reads: FieldNoise}
		if chipDive(frames) != 0 {
			dk.reads |= FieldSweep
			dk.pitch = true
		}
		deepKinds[k] = dk
	}
}

// deepOf returns how drum dr's deep settings change a hit at velocity
// vel, with the parts it mutes silenced.
func deepOf(dr *drum, vel float32) (dp deep) {
	d, k := &dr.Drum, dr.kind
	dp = deep{sk: 1, bm: 1, nm: 1, cm: 1, spread: 1}
	defer func() {
		if dr.mute&partBody != 0 {
			dp.bm = 0
		}
		if dr.mute&partNoise != 0 {
			dp.nm = 0
		}
		if dr.mute&partClick != 0 {
			dp.cm = 0
		}
	}()
	dk, ok := deepKinds[k]
	if !ok {
		return dp
	}
	if d.Sweep != 0 && dk.reads&FieldSweep != 0 && isChipDrum(k) {
		frames, _ := chipDrum(k)
		if dive := chipDive(frames); dive > 0 {
			dp.sk = max((dive+12*float32(clamp64(d.Sweep, -2, 2)))/dive, 0)
			dp.note = chipLast(frames)
		}
	} else if d.Sweep != 0 && dk.reads&FieldSweep != 0 {
		if kk := dk.k + dk.kv*vel; kk > 0 {
			dp.sk = max(float32((float64(1+kk)*math.Exp2(clamp64(d.Sweep, -2, 2))-1)/float64(kk)), 0)
		}
	}
	if d.Noise != 0 && dk.reads&FieldNoise != 0 {
		n := clamp64(d.Noise, -1, 1)
		dp.nm, dp.bm = float32(math.Exp2(2*n)), float32(math.Exp2(-n))
	}
	if d.Click != 0 && dk.reads&FieldClick != 0 {
		dp.cm = clickMul(d.Click)
	}
	if d.Spread != 0 && dk.reads&FieldSpread != 0 {
		dp.spread = float32(math.Exp2(clamp64(d.Spread, -1, 1)))
	}
	return dp
}

// clickMul is how much louder Click c makes a click: none at -1, its
// own at 0 and four times at 1.
func clickMul(c float64) float32 {
	c = clamp64(c, -1, 1)
	if c < 0 {
		return float32(1 + c)
	}
	return float32(1 + 3*c)
}

// sweepTime is how many times longer d's dive takes.
func sweepTime(d *Drum) float32 {
	if d.SweepTime <= 0 {
		return 1
	}
	return float32(clamp64(d.SweepTime, 0.25, 4))
}

func clamp64(x, lo, hi float64) float64 { return min(max(x, lo), hi) }

// spreadOf writes into out the frequencies fs spread by power p round
// their middle, the mean of their logs: each as far from it as it was,
// raised to p, in pitch. With p 1 it copies them as they are.
func spreadOf(out []float32, fs []float32, p float32) {
	if p == 1 {
		copy(out, fs)
		return
	}
	var lm float64
	for _, f := range fs {
		lm += math.Log(float64(f))
	}
	lm /= float64(len(fs))
	for i, f := range fs {
		out[i] = float32(math.Exp(lm + (math.Log(float64(f))-lm)*float64(p)))
	}
}

// chipDive returns how far a chip drum's tone falls, in semitones,
// from its first tonal frame to its last, or 0 for a drum with no tone
// or none that falls.
func chipDive(frames []sidFrame) float32 {
	first, last := -1, -1
	for i, f := range frames {
		if f.wave != sfNoise && f.wave != sfNESNoise && f.wave != sfNESMetal {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 || first == last {
		return 0
	}
	return frames[first].note - frames[last].note
}

// isTonal reports whether a chip drum's frame f plays a tone, not noise.
func isTonal(f sidFrame) bool {
	return f.wave != sfNoise && f.wave != sfNESNoise && f.wave != sfNESMetal
}

// chipLast returns a chip drum's last tonal note.
func chipLast(frames []sidFrame) float32 {
	for i := len(frames) - 1; i >= 0; i-- {
		if isTonal(frames[i]) {
			return frames[i].note
		}
	}
	return 0
}
