package main

// phone is an insert that plays a sound as a phone's small speaker
// does: nothing much under about 700 Hz, a lift where it rings, about
// 2.5 kHz, and little over 8 kHz.
type phone struct {
	hp1, hp2, lp, ring biquad
}

func newPhone() *phone {
	return &phone{hp1: highPass(700, 0.5412), hp2: highPass(700, 1.3066), lp: lowPass(8000, 0.707), ring: peaking(2500, 1.2, 4)}
}

// Process implements audio.Insert: the speaker is one, so both sides
// play the same.
func (p *phone) Process(frames []float32) {
	for i := 0; i+1 < len(frames); i += 2 {
		x := float64(frames[i]+frames[i+1]) / 2
		x = p.ring.step(p.lp.step(p.hp2.step(p.hp1.step(x))))
		frames[i], frames[i+1] = float32(x), float32(x)
	}
}
