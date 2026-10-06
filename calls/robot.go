package calls

import "math"

// A friendly robot's sounds: beeps, a servo's whirr, and a glitch.

func init() {
	waveParam := Param{Name: "wave", Label: "Wave", Lo: 0, Hi: 1, Def: 0.35, Vary: 0.02, About: "The tone's shape: 0 a pure sine, 1 a buzzing square"}
	topParam := Param{Name: "top", Label: "Top", Lo: 0, Hi: 15, Def: 0, Step: 1, About: "The highest odd harmonic the tone keeps, as 5 keeps the 3rd and the 5th: rounder than the whole square; 0 keeps every one up to 12 kHz"}
	register(&Model{
		Name:  "beeps",
		About: "A robot's beeps: tones in a run, each a step from the last, sliding if asked",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 4000, Def: 1400, Log: true, Vary: 0.03, About: "The first beep's pitch"},
			{Name: "beeps", Label: "Beeps", Lo: 1, Hi: 12, Def: 2, Step: 1, About: "How many beeps"},
			{Name: "step", Label: "Step", Unit: "st", Lo: -12, Hi: 12, Def: -5, Vary: 0.02, About: "How far each beep is above the last"},
			{Name: "zigzag", Label: "Zigzag", Lo: 0, Hi: 1, Def: 0, Step: 1, About: "1 turns every other step the other way"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.02, Hi: 0.4, Def: 0.09, Vary: 0.04, About: "Each beep's length"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.2, Def: 0.035, Vary: 0.04, About: "The silence between beeps"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 4, Def: 1.3, Vary: 0.03, About: "The last beep's length, times the others'"},
			{Name: "slide", Label: "Slide", Unit: "st", Lo: -12, Hi: 12, Def: 0, Vary: 0.03, About: "How far each beep slides through its length"},
			{Name: "warble", Label: "Warble", Unit: "st", Lo: 0, Hi: 3, Def: 0, About: "How far a fast warble shakes each beep"},
			waveParam,
			topParam,
		},
		render: beeps,
	})
	register(&Model{
		Name:  "whirr",
		About: "A servo's whirr: a little motor's buzz sliding up or down, its gears ticking",
		Params: []Param{
			{Name: "from", Label: "From", Unit: "Hz", Lo: 100, Hi: 3000, Def: 500, Log: true, Vary: 0.04, About: "The motor's pitch where it starts"},
			{Name: "to", Label: "To", Unit: "Hz", Lo: 100, Hi: 3000, Def: 1300, Log: true, Vary: 0.04, About: "Its pitch where it ends"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.05, Hi: 1, Def: 0.3, Vary: 0.05, About: "How long it lasts"},
			{Name: "grit", Label: "Grit", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.04, About: "How much the gears tick"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.6, Vary: 0.03, About: "How buzzing the motor is"},
			{Name: "swell", Label: "Swell", Lo: 0, Hi: 1, Def: 0.5, About: "Where it is loudest, from its start to its end"},
		},
		render: whirr,
	})
	register(&Model{
		Name:  "glitch",
		About: "A cute glitch, \"bwoo\": a tone that stutters, then falls away wobbling, its samples crushed",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 200, Hi: 4000, Def: 1300, Log: true, Vary: 0.03, About: "The tone's pitch where it starts"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -12, Hi: 36, Def: 14, Vary: 0.03, About: "How far it falls"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.1, Hi: 1, Def: 0.38, Vary: 0.04, About: "How long it lasts"},
			{Name: "stutters", Label: "Stutters", Lo: 0, Hi: 8, Def: 3, Step: 1, About: "How many times its start repeats"},
			{Name: "stutter", Label: "Stutter", Unit: "s", Lo: 0.01, Hi: 0.1, Def: 0.03, Vary: 0.04, About: "How long each repeat is"},
			{Name: "wobble", Label: "Wobble", Unit: "st", Lo: 0, Hi: 4, Def: 1, Vary: 0.04, About: "How far it wobbles as it falls"},
			{Name: "crush", Label: "Crush", Lo: 0, Hi: 1, Def: 0.35, Vary: 0.03, About: "How coarse its samples are, as an old chip's"},
			waveParam,
			topParam,
		},
		render: glitch,
	})
}

// tone returns a tone sliding through hz(u) over dur seconds, at
// level(u): a sine, and odd harmonics as wave asks, toward a square,
// up to harmonic top where it is over 0.
func tone(dur, wave, top float64, hz, level func(u float64) float64) []float64 {
	n := int(dur * rate)
	out := make([]float64, n)
	var ph float64
	for i := range out {
		u := float64(i) / float64(n)
		f := hz(u)
		ph += 2 * math.Pi * f / rate
		if ph > 2*math.Pi {
			ph -= 2 * math.Pi
		}
		v := math.Sin(ph)
		if wave > 0 {
			for k := 3; float64(k)*f < 12000 && (top <= 0 || float64(k) <= top); k += 2 {
				v += wave * math.Sin(float64(k)*ph) / float64(k)
			}
		}
		out[i] = v * level(u)
	}
	return out
}

// beeps makes a robot's beeps.
func beeps(p params, r *rng) []float64 {
	var out []float64
	n := int(p["beeps"])
	at := 0.0
	st := 0.0
	for i := range n {
		dur := p["length"]
		if i == n-1 {
			dur *= p["last"]
		}
		base := p["pitch"] * semis(st)
		war := p["warble"]
		x := tone(dur, p["wave"], p["top"],
			func(u float64) float64 {
				return base * semis(p["slide"]*u+war*math.Sin(2*math.Pi*28*u*dur))
			},
			func(u float64) float64 { return smooth(u*dur/0.003) * smooth((1-u)*dur/0.008) })
		add(&out, x, at, 0.7)
		step := p["step"]
		if p["zigzag"] > 0.5 && i%2 == 1 {
			step = -step
		}
		st += step
		at += dur + p["gap"]
	}
	return out
}

// whirr makes a servo's whirr.
func whirr(p params, r *rng) []float64 {
	dur := p["length"]
	n := int(dur * rate)
	out := make([]float64, n)
	from, to := p["from"], p["to"]
	b := p["bright"]
	var ph, gear float64
	sw := min(max(p["swell"], 0.05), 0.95)
	for i := range out {
		u := float64(i) / float64(n)
		f := from * math.Pow(to/from, smooth(u))
		ph += 2 * math.Pi * f / rate
		if ph > 2*math.Pi {
			ph -= 2 * math.Pi
		}
		// A saw's harmonics, as many as the brightness asks.
		var v float64
		for k := 1; float64(k)*f < 10000; k++ {
			v += math.Pow(float64(k), -1-1.5*(1-b)) * math.Sin(float64(k)*ph)
		}
		// The gears tick a quarter as fast as the motor turns.
		gear += f / 4 / rate
		tick := math.Exp(-math.Mod(gear, 1) * 12)
		v *= 1 - p["grit"]*0.6 + p["grit"]*0.6*tick
		env := smooth(u/sw) * smooth((1-u)/(1-sw)) * smooth(u*dur/0.01) * smooth((1-u)*dur/0.02)
		out[i] = v * env * 0.6
	}
	if g := p["grit"]; g > 0 {
		hiss := air(r, dur, 0.8, func(u float64) float64 { return 2.5 * from * math.Pow(to/from, u) },
			func(u float64) float64 { return 0.25 * g * smooth(u/0.1) * smooth((1-u)/0.2) })
		add(&out, hiss, 0, 1)
	}
	return out
}

// glitch makes a glitch: a tone that stutters at its start, then falls
// away wobbling, crushed.
func glitch(p params, r *rng) []float64 {
	dur := p["length"]
	wob := p["wobble"]
	x := tone(dur, p["wave"], p["top"],
		func(u float64) float64 {
			return p["pitch"] * semis(-p["fall"]*math.Pow(u, 0.7)+wob*math.Sin(2*math.Pi*14*u*dur)*smooth(u/0.3))
		},
		func(u float64) float64 { return smooth(u*dur/0.003) * (1 - 0.5*u) * smooth((1-u)/0.15) })
	// The stutter: its first moment, again and again.
	s := int(p["stutter"] * rate)
	var out []float64
	for range int(p["stutters"]) {
		piece := make([]float64, min(s, len(x)))
		copy(piece, x)
		for i := range piece {
			piece[i] *= smooth(float64(len(piece)-i) / (0.004 * rate))
		}
		out = append(out, piece...)
	}
	out = append(out, x...)
	// The crush: each sample held a while, and rounded coarsely.
	if c := p["crush"]; c > 0 {
		hold := 1 + int(c*10)
		steps := math.Pow(2, 12-9*c)
		var held float64
		for i := range out {
			if i%hold == 0 {
				held = math.Round(out[i]*steps) / steps
			}
			out[i] = held
		}
	}
	return out
}
