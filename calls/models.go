package calls

import "math"

// The models' parameters most share.
var (
	sizeParam = Param{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.3, Vary: 0.03,
		About: "Scales the mouth's resonances: above 1 a smaller creature, higher and brighter"}
	brightParam = Param{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.03,
		About: "How strong the voice's upper harmonics are"}
)

// tilt returns the harmonics' slope for a brightness from 0 to 1.
func tilt(bright float64) float64 { return 2.2 - 1.6*bright }

func init() {
	register(&Model{
		Name:  "hoot",
		About: "An owl's hoot: a soft, breathy voice on an \"oo\", sliding up into each hoot and down out of it",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 250, Hi: 1500, Def: 620, Log: true, Vary: 0.04, About: "The first hoot's pitch"},
			{Name: "hoots", Label: "Hoots", Lo: 1, Hi: 5, Def: 2, Step: 1, About: "How many hoots"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.06, Hi: 0.6, Def: 0.2, Vary: 0.05, About: "Each hoot's length"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.4, Def: 0.08, Vary: 0.04, About: "The silence between hoots"},
			{Name: "rise", Label: "Rise", Unit: "st", Lo: -7, Hi: 12, Def: 0, Vary: 0.02, About: "How far each hoot is above the last, in semitones"},
			{Name: "scoop", Label: "Scoop", Unit: "st", Lo: 0, Hi: 7, Def: 2, Vary: 0.03, About: "How far each hoot slides up into its pitch"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -5, Hi: 7, Def: 1.5, Vary: 0.03, About: "How far each hoot falls at its end; below 0 it rises"},
			{Name: "question", Label: "Question", Unit: "st", Lo: 0, Hi: 9, Def: 0, About: "How far the last hoot rises at its end, as a question"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 2.5, Def: 1, Vary: 0.03, About: "The last hoot's length, times the others'"},
			{Name: "swell", Label: "Swell", Lo: -1, Hi: 1, Def: 0, About: "Later hoots louder above 0, softer below"},
			{Name: "open", Label: "Open", Lo: 0, Hi: 1, Def: 0.25, Vary: 0.03, About: "The vowel, from \"oo\" through \"oh\" to \"ah\""},
			sizeParam, brightParam,
			{Name: "ring", Label: "Ring", Lo: 0, Hi: 2, Def: 0.6, Vary: 0.04, About: "How strongly the voice rings high, about 2.5 kHz, as a singer's does to carry"},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.3, Vary: 0.05, About: "How breathy, and how strong each hoot's \"h\""},
			{Name: "attack", Label: "Attack", Lo: 0.02, Hi: 0.5, Def: 0.12, About: "How softly each hoot comes in, a share of it"},
			{Name: "vibrato", Label: "Vibrato", Unit: "st", Lo: 0, Hi: 1, Def: 0.15, About: "How far the pitch wavers"},
		},
		render: hoot,
	})
	register(&Model{
		Name:  "ribbit",
		About: "A frog's \"rib-bit\": a croak rolled by its throat into fast pulses, then a short, higher \"bit\" with a \"t\"",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 100, Hi: 700, Def: 260, Log: true, Vary: 0.04, About: "The croak's pitch"},
			{Name: "ribbits", Label: "Ribbits", Lo: 1, Hi: 4, Def: 1, Step: 1, About: "How many ribbits"},
			{Name: "rise", Label: "Rise", Unit: "st", Lo: -5, Hi: 12, Def: 0, Vary: 0.02, About: "How far each ribbit is above the last"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.05, Hi: 0.4, Def: 0.13, Vary: 0.05, About: "The \"rib\"'s length"},
			{Name: "bit", Label: "Bit", Unit: "×", Lo: 0.3, Hi: 1.5, Def: 0.6, Vary: 0.04, About: "The \"bit\"'s length, times the \"rib\"'s"},
			{Name: "up", Label: "Up", Unit: "st", Lo: -7, Hi: 12, Def: 3, Vary: 0.02, About: "How far the \"bit\" is above the \"rib\""},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.15, Def: 0.03, Vary: 0.04, About: "The silence between \"rib\" and \"bit\""},
			{Name: "space", Label: "Space", Unit: "s", Lo: 0, Hi: 0.4, Def: 0.08, Vary: 0.04, About: "The silence between ribbits"},
			{Name: "roll", Label: "Roll", Unit: "Hz", Lo: 15, Hi: 120, Def: 50, Log: true, Vary: 0.06, About: "How fast the throat pulses"},
			{Name: "rr", Label: "Rrr", Lo: 0, Hi: 1, Def: 0.45, Vary: 0.04, About: "How much of the \"rib\" is rolled"},
			{Name: "rough", Label: "Rough", Lo: 0, Hi: 1, Def: 0.85, Vary: 0.03, About: "How deep the roll's pulses go"},
			{Name: "throat", Label: "Throat", Lo: 0, Hi: 2, Def: 0.6, Vary: 0.05, About: "How strongly the throat sac rings"},
			{Name: "sac", Label: "Sac", Unit: "Hz", Lo: 400, Hi: 3000, Def: 1100, Log: true, Vary: 0.03, About: "The throat sac's pitch"},
			{Name: "click", Label: "Click", Lo: 0, Hi: 1, Def: 0.35, Vary: 0.05, About: "How loud the \"t\" that ends the \"bit\" is"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.4, Vary: 0.03, About: sizeParam.About},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.6, Vary: 0.03, About: brightParam.About},
		},
		render: ribbit,
	})
	register(&Model{
		Name:  "blub",
		About: "A gulp: a small voiced \"bl-ub\" falling, with a lip's pop before it and bubbles rising after",
		Params: []Param{
			{Name: "voice", Label: "Voice", Lo: 0, Hi: 1, Def: 0.7, About: "How loud the voiced \"blub\" is"},
			{Name: "vpitch", Label: "V pitch", Unit: "Hz", Lo: 120, Hi: 900, Def: 330, Log: true, Vary: 0.04, About: "The voice's pitch"},
			{Name: "vlen", Label: "V length", Unit: "s", Lo: 0.05, Hi: 0.4, Def: 0.14, Vary: 0.05, About: "The voice's length"},
			{Name: "vfall", Label: "V fall", Unit: "st", Lo: -7, Hi: 12, Def: 5, Vary: 0.03, About: "How far the voice falls"},
			{Name: "pop", Label: "Pop", Lo: 0, Hi: 1, Def: 0.4, Vary: 0.05, About: "How loud the lips' pop before it is"},
			{Name: "pitch", Label: "Bubble", Unit: "Hz", Lo: 300, Hi: 3000, Def: 1000, Log: true, Vary: 0.06, About: "The first bubble's pitch"},
			{Name: "bubbles", Label: "Bubbles", Lo: 0, Hi: 5, Def: 2, Step: 1, About: "How many bubbles"},
			{Name: "level", Label: "B level", Lo: 0, Hi: 1.5, Def: 0.7, About: "How loud the bubbles are"},
			{Name: "start", Label: "B start", Unit: "s", Lo: 0, Hi: 0.4, Def: 0.08, Vary: 0.04, About: "When the first bubble comes"},
			{Name: "spacing", Label: "B spacing", Unit: "s", Lo: 0.02, Hi: 0.25, Def: 0.07, Vary: 0.06, About: "The time between bubbles"},
			{Name: "climb", Label: "B climb", Lo: 0, Hi: 2, Def: 0.7, Vary: 0.05, About: "How far each bubble's pitch climbs, a share of it"},
			{Name: "decay", Label: "B decay", Unit: "s", Lo: 0.01, Hi: 0.15, Def: 0.03, Vary: 0.05, About: "How fast each bubble dies"},
			{Name: "drop", Label: "B step", Unit: "st", Lo: -12, Hi: 12, Def: -3, Vary: 0.03, About: "How far each bubble is above the last"},
			sizeParam, brightParam,
		},
		render: blub,
	})
}

// hoot makes an owl's hoots.
func hoot(p params, r *rng) []float64 {
	var out []float64
	n := int(p["hoots"])
	at := 0.0
	vib := newDrift(r, 3)
	ring := &resonance{hz: 2500 * p["size"] / 1.3, width: 1400, level: p["ring"]}
	for i := range n {
		dur := p["length"]
		last := i == n-1
		if last {
			dur *= p["last"]
		}
		base := p["pitch"] * semis(p["rise"]*float64(i))
		lvl := 1 + p["swell"]*(float64(i)/max(float64(n-1), 1)-0.5)
		vrate := 5.5 * (1 + 0.1*vib.step())
		atk := p["attack"]
		start := at
		sing(&out, syllable{
			at: at, dur: dur,
			pitch: func(u float64) float64 {
				st := -p["scoop"] * (1 - smooth(u/0.3))
				st -= p["fall"] * smooth((u-0.55)/0.45)
				if last {
					st += p["question"] * smooth((u-0.35)/0.65)
				}
				st += p["vibrato"] * smooth(u/0.4) * math.Sin(2*math.Pi*vrate*(start+u*dur))
				return base * semis(st)
			},
			level: func(u float64) float64 {
				// In quick but soft, held, and out with a long fade.
				return lvl * smooth(u/atk) * smooth((1-u)/0.45)
			},
			breath: func(u float64) float64 {
				h := 1 - smooth(u/(atk+0.08))
				return p["breath"] * lvl * (0.35 + 1.4*h) * smooth(u/0.03) * smooth((1-u)/0.3)
			},
			mouth: func(u float64) [3]float64 {
				// The vowel opens through the hoot, and closes at its end.
				o := p["open"] * (0.6 + 0.4*smooth(u/0.5)) * (1 - 0.5*smooth((u-0.7)/0.3))
				if o < 0.5 {
					return vowel('u', 'o', o*2)
				}
				return vowel('o', 'a', o*2-1)
			},
			size:   p["size"],
			tilt:   tilt(p["bright"]),
			body:   ring,
			wander: 0.004,
		}, r)
		at += dur + p["gap"]
	}
	return out
}

// ribbit makes a frog's ribbits.
func ribbit(p params, r *rng) []float64 {
	var out []float64
	n := int(p["ribbits"])
	at := 0.0
	body := &resonance{hz: p["sac"], width: p["sac"] / 6, level: p["throat"]}
	tl := tilt(p["bright"])
	for i := range n {
		base := p["pitch"] * semis(p["rise"]*float64(i))
		rib := p["length"]
		roll := p["roll"] * (1 + 0.04*r.norm())
		// The "rib": rolled for its first part, its pulses each a sharp
		// knock dying away, then voiced on an "i", closed by the lips.
		ribAt := at
		sing(&out, syllable{
			at: at, dur: rib,
			pitch: func(u float64) float64 { return base * semis(1.5*smooth(u/0.5)-1*smooth((u-0.6)/0.4)) },
			level: func(u float64) float64 { return smooth(u/0.04) * (1 - 0.8*smooth((u-0.8)/0.2)) },
			breath: func(u float64) float64 {
				return 0.15 * smooth(u/0.02) * smooth((1-u)/0.2)
			},
			mouth: func(u float64) [3]float64 {
				f := vowel('o', 'i', smooth((u-p["rr"]*0.6)/0.4))
				// The lips close the "b": every formant falls.
				return scale(f, 1-0.35*smooth((u-0.82)/0.18))
			},
			roll: func(t float64) float64 {
				u := (t - ribAt) / rib
				depth := p["rough"] * (1 - smooth((u-p["rr"])/0.15))
				ph := math.Mod((t-ribAt)*roll, 1)
				pulse := math.Exp(-ph * 4)
				return 1 - depth + depth*pulse*1.4
			},
			size: p["size"], tilt: tl, body: body, wander: 0.01,
		}, r)
		at += rib + p["gap"]
		// The "bit": in from the "b", higher, short, ending in a "t".
		bit := rib * p["bit"]
		hi := base * semis(p["up"])
		sing(&out, syllable{
			at: at, dur: bit,
			pitch: func(u float64) float64 { return hi * semis(-0.7+0.7*smooth(u/0.25)-0.8*smooth((u-0.7)/0.3)) },
			level: func(u float64) float64 { return smooth(u/0.06) * smooth((1-u)/0.12) },
			breath: func(u float64) float64 {
				return 0.12 * smooth(u/0.06) * smooth((1-u)/0.12)
			},
			mouth: func(u float64) [3]float64 {
				return scale(vowel('e', 'i', smooth(u/0.4)), 0.7+0.3*smooth(u/0.2))
			},
			size: p["size"], tilt: tl, body: body, wander: 0.01,
		}, r)
		at += bit
		// The "t": a short burst of high breath at the tongue's release.
		if c := p["click"]; c > 0 {
			t := burst(r, 3500*p["size"]/1.4, 0.4, 0.012)
			add(&out, t, at+0.01, c*0.9)
		}
		at += p["space"] + 0.03
	}
	return out
}

// blub makes a small gulp: a lip's pop, a falling voiced "blub", and
// bubbles rising after it.
func blub(p params, r *rng) []float64 {
	var out []float64
	if pop := p["pop"]; pop > 0 {
		add(&out, knock(r, 900*p["size"]/1.3, 0.006), 0, pop)
	}
	if v := p["voice"]; v > 0 {
		vl, vp := p["vlen"], p["vpitch"]
		sing(&out, syllable{
			at: 0.004, dur: vl,
			pitch: func(u float64) float64 { return vp * semis(1-p["vfall"]*smooth(u)) },
			level: func(u float64) float64 { return v * smooth(u/0.08) * smooth((1-u)/0.35) },
			breath: func(u float64) float64 {
				return 0.1 * v * smooth(u/0.05) * smooth((1-u)/0.3)
			},
			mouth: func(u float64) [3]float64 {
				// "b-l-u-b": from closed lips, through an "l"'s high
				// second formant, to "u", the lips closing again.
				f := vowel('e', 'u', smooth((u-0.1)/0.4))
				return scale(f, (0.7+0.3*smooth(u/0.12))*(1-0.3*smooth((u-0.8)/0.2)))
			},
			size: p["size"], tilt: tilt(p["bright"]), wander: 0.008,
		}, r)
	}
	at := p["start"]
	for i := range int(p["bubbles"]) {
		hz := p["pitch"] * semis(p["drop"]*float64(i)) * (1 + 0.03*r.norm())
		add(&out, bubble(hz, p["climb"], p["decay"]), at, p["level"]*(1-0.15*float64(i)))
		at += p["spacing"] * (1 + 0.1*r.norm())
	}
	return out
}

// scale returns the formants f times k.
func scale(f [3]float64, k float64) [3]float64 {
	for i := range f {
		f[i] *= k
	}
	return f
}

// add adds x at at seconds into out, times g, growing out to hold it.
func add(out *[]float64, x []float64, at, g float64) {
	s := int(max(at, 0) * rate)
	if need := s + len(x); need > len(*out) {
		*out = append(*out, make([]float64, need-len(*out))...)
	}
	for i, v := range x {
		(*out)[s+i] += v * g
	}
}

// bubble returns a bubble's sound, as Minnaert worked it out: a pure
// tone at the bubble's pitch, its pitch climbing by climb as it rises
// and dies away over decay seconds.
func bubble(hz, climb, decay float64) []float64 {
	n := int(6 * decay * rate)
	out := make([]float64, n)
	var ph float64
	for i := range out {
		t := float64(i) / rate
		f := hz * (1 + climb*min(t/(3*decay), 1))
		ph += 2 * math.Pi * f / rate
		env := math.Exp(-t/decay) * smooth(t/0.0008)
		out[i] = math.Sin(ph) * env
	}
	return out
}

// knock returns a short knock: a tone at hz dying over decay seconds,
// with a little noise in its strike.
func knock(r *rng, hz, decay float64) []float64 {
	n := int(6 * decay * rate)
	out := make([]float64, n)
	bp := biquad{}
	bp.bandPass(hz*1.5, hz)
	for i := range out {
		t := float64(i) / rate
		env := math.Exp(-t / decay)
		out[i] = env * (math.Sin(2*math.Pi*hz*t) + 0.5*bp.step(r.noise())*math.Exp(-t/0.002))
	}
	return out
}

// burst returns a short burst of noise about hz, as a "t" or an "s",
// width a share of hz wide, over dur seconds.
func burst(r *rng, hz, width, dur float64) []float64 {
	n := int(dur * rate)
	out := make([]float64, n)
	bp := biquad{}
	bp.bandPass(hz, hz*width)
	g := 1 / math.Sqrt(math.Pi*hz*width/rate)
	for i := range out {
		u := float64(i) / float64(n)
		out[i] = bp.step(r.norm()) * g * 0.4 * smooth(u/0.1) * (1 - u) * (1 - u)
	}
	return out
}
