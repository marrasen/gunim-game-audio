package calls

// A Phone plays a sound as a phone's small speaker does: nothing much
// under about 700 Hz, a lift where it rings, about 2.5 kHz, and little
// over 8 kHz. It is an audio.Insert, for hearing a call as a phone
// plays it, and the stats measure each call through it.
type Phone struct {
	hp1, hp2, lp, ring biquad
}

// NewPhone returns a phone's speaker.
func NewPhone() *Phone {
	return &Phone{hp1: highPass(700, 0.5412), hp2: highPass(700, 1.3066), lp: lowPass(8000, 0.707), ring: peaking(2500, 1.2, 4)}
}

// step returns x as the speaker plays it.
func (p *Phone) step(x float64) float64 {
	return p.ring.step(p.lp.step(p.hp2.step(p.hp1.step(x))))
}

// Process implements audio.Insert: the speaker is one, so both sides
// play the same.
func (p *Phone) Process(frames []float32) {
	for i := 0; i+1 < len(frames); i += 2 {
		x := p.step(float64(frames[i]+frames[i+1]) / 2)
		frames[i], frames[i+1] = float32(x), float32(x)
	}
}

// phoneDrop returns how much quieter x is on a phone's speaker than in
// full, in decibels, its loudness measured as Loudness is.
func phoneDrop(x []float64) float64 {
	p := NewPhone()
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = p.step(v)
	}
	return loudness(x) - loudness(y)
}
