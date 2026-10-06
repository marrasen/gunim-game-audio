package calls

import "math"

// The voices of animals and people: a kitten's mew, a fox's yip, a baby
// dragon's roar, a singer's syllables, and a crowd.

func init() {
	register(&Model{
		Name:  "mew",
		About: "A kitten's mew, \"m-i-a-u\": in from closed lips, its pitch arching up and down; a trill rolls it, as a happy cat's chirp",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 1600, Def: 820, Log: true, Vary: 0.04, About: "The mew's pitch where it starts"},
			{Name: "mews", Label: "Mews", Lo: 1, Hi: 4, Def: 1, Step: 1, About: "How many mews"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.08, Hi: 0.8, Def: 0.35, Vary: 0.05, About: "Each mew's length"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.3, Def: 0.05, Vary: 0.04, About: "The silence between mews"},
			{Name: "rise", Label: "Rise", Unit: "st", Lo: -7, Hi: 12, Def: 0, Vary: 0.02, About: "How far each mew is above the last"},
			{Name: "arch", Label: "Arch", Unit: "st", Lo: 0, Hi: 12, Def: 4, Vary: 0.03, About: "How far the pitch climbs early in the mew"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -5, Hi: 12, Def: 6, Vary: 0.03, About: "How far it falls by its end"},
			{Name: "question", Label: "Question", Unit: "st", Lo: 0, Hi: 9, Def: 0, About: "How far the last mew rises at its end, as a question"},
			{Name: "open", Label: "Open", Lo: 0, Hi: 1, Def: 0.8, Vary: 0.03, About: "How far the vowel opens, from \"i\" to \"a\""},
			{Name: "close", Label: "Close", Lo: 0, Hi: 1, Def: 0.7, Vary: 0.03, About: "How far the end closes to \"u\""},
			{Name: "trill", Label: "Trill", Lo: 0, Hi: 1, Def: 0, Vary: 0.03, About: "How deeply a roll trills it, as \"brrrp\""},
			{Name: "trillhz", Label: "Trill Hz", Unit: "Hz", Lo: 10, Hi: 60, Def: 26, Log: true, Vary: 0.05, About: "How fast the trill rolls"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.6, Vary: 0.03, About: sizeParam.About},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.55, Vary: 0.03, About: brightParam.About},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.2, Vary: 0.05, About: "How breathy"},
		},
		render: mew,
	})
	register(&Model{
		Name:  "yip",
		About: "A fox's yip: short and bright, its pitch leaping up and dropping, on an \"a\" from a \"y\"",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 2000, Def: 1000, Log: true, Vary: 0.04, About: "The yip's pitch"},
			{Name: "yips", Label: "Yips", Lo: 1, Hi: 6, Def: 1, Step: 1, About: "How many yips"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.04, Hi: 0.4, Def: 0.11, Vary: 0.05, About: "Each yip's length"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.3, Def: 0.07, Vary: 0.05, About: "The silence between yips"},
			{Name: "rise", Label: "Rise", Unit: "st", Lo: -7, Hi: 12, Def: 0, Vary: 0.02, About: "How far each yip is above the last"},
			{Name: "leap", Label: "Leap", Unit: "st", Lo: 0, Hi: 12, Def: 5, Vary: 0.03, About: "How far the pitch leaps up into the yip"},
			{Name: "drop", Label: "Drop", Unit: "st", Lo: -5, Hi: 12, Def: 6, Vary: 0.03, About: "How far it drops at its end"},
			{Name: "question", Label: "Question", Unit: "st", Lo: 0, Hi: 12, Def: 0, About: "How far the last yip rises at its end, as a question"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 3, Def: 1, Vary: 0.03, About: "The last yip's length, times the others'"},
			{Name: "open", Label: "Open", Lo: 0, Hi: 1, Def: 0.7, Vary: 0.03, About: "How far the vowel opens, from \"e\" to \"a\""},
			{Name: "rough", Label: "Rough", Lo: 0, Hi: 1, Def: 0.2, Vary: 0.04, About: "How rough and barky the voice is"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.5, Vary: 0.03, About: sizeParam.About},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.65, Vary: 0.03, About: brightParam.About},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.25, Vary: 0.05, About: "How breathy"},
		},
		render: yip,
	})
	register(&Model{
		Name:  "roar",
		About: "A small creature's roar, \"rawr\": a voice made rough by a growl that shakes it, ending, if asked, in a squeak",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 150, Hi: 900, Def: 380, Log: true, Vary: 0.04, About: "The roar's pitch"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.1, Hi: 0.9, Def: 0.32, Vary: 0.05, About: "The roar's length"},
			{Name: "arch", Label: "Arch", Unit: "st", Lo: -5, Hi: 12, Def: 3, Vary: 0.03, About: "How far the pitch swells in the middle"},
			{Name: "growl", Label: "Growl", Lo: 0, Hi: 1, Def: 0.6, Vary: 0.04, About: "How deeply the growl shakes it"},
			{Name: "growlhz", Label: "Growl Hz", Unit: "Hz", Lo: 15, Hi: 90, Def: 38, Log: true, Vary: 0.05, About: "How fast the growl shakes"},
			{Name: "wobble", Label: "Wobble", Lo: 0, Hi: 0.1, Def: 0.03, Vary: 0.05, About: "How unsteady its pitch is"},
			{Name: "open", Label: "Open", Lo: 0, Hi: 1, Def: 0.85, Vary: 0.03, About: "How wide the mouth opens, \"o\" to \"a\""},
			{Name: "squeak", Label: "Squeak", Lo: 0, Hi: 1, Def: 0, Vary: 0.03, About: "How long the squeak at its end is, a share of the roar; 0 for none"},
			{Name: "squeakup", Label: "Squeak up", Unit: "st", Lo: 0, Hi: 24, Def: 14, Vary: 0.02, About: "How far above the roar the squeak jumps"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.5, Vary: 0.03, About: sizeParam.About},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.75, Vary: 0.03, About: brightParam.About},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.45, Vary: 0.05, About: "How breathy and hissing"},
		},
		render: roar,
	})
	register(&Model{
		Name:  "sing",
		About: "A sung phrase: a syllable on each of up to four notes, scooped into, with vibrato and a singer's ring",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 100, Hi: 1200, Def: 523, Log: true, Vary: 0.02, About: "The first note's pitch"},
			{Name: "notes", Label: "Notes", Lo: 1, Hi: 4, Def: 3, Step: 1, About: "How many notes"},
			{Name: "n2", Label: "Note 2", Unit: "st", Lo: -12, Hi: 12, Def: 2, Step: 1, About: "The second note, in semitones from the first"},
			{Name: "n3", Label: "Note 3", Unit: "st", Lo: -12, Hi: 12, Def: 4, Step: 1, About: "The third note"},
			{Name: "n4", Label: "Note 4", Unit: "st", Lo: -12, Hi: 24, Def: 7, Step: 1, About: "The fourth note"},
			{Name: "syllable", Label: "Syllable", Def: 0, Choices: syllables, About: "The syllable each note is sung on"},
			{Name: "lastsyl", Label: "Last syl", Def: 0, Choices: syllables, About: "The syllable the last note is sung on"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.06, Hi: 0.5, Def: 0.13, Vary: 0.04, About: "Each note's length"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 4, Def: 1.6, Vary: 0.03, About: "The last note's length, times the others'"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.2, Def: 0.02, Vary: 0.03, About: "The silence between notes"},
			{Name: "scoop", Label: "Scoop", Unit: "st", Lo: 0, Hi: 5, Def: 1, Vary: 0.03, About: "How far each note slides up into its pitch"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -5, Hi: 12, Def: 0, Vary: 0.03, About: "How far the last note falls at its end"},
			{Name: "vibrato", Label: "Vibrato", Unit: "st", Lo: 0, Hi: 1.5, Def: 0.35, Vary: 0.04, About: "How far the long notes waver"},
			{Name: "ring", Label: "Ring", Lo: 0, Hi: 2, Def: 0.8, Vary: 0.04, About: "How strongly the voice rings high, about 3 kHz, as a singer's does"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.2, Vary: 0.02, About: "Scales the mouth's resonances: 1 a grown man, 1.2 a woman, higher a child"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.55, Vary: 0.03, About: brightParam.About},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.2, Vary: 0.05, About: "How breathy, as a pop singer's close voice"},
		},
		render: singPhrase,
	})
	register(&Model{
		Name:  "crowd",
		About: "A small crowd: many voices together, each its own pitch and timing, cheering \"yay\" or sighing \"ooh\", with claps",
		Params: []Param{
			{Name: "kind", Label: "Kind", Def: 0, Choices: []string{"yay", "ooh", "aah"}, About: "What the crowd calls"},
			{Name: "voices", Label: "Voices", Lo: 2, Hi: 24, Def: 12, Step: 1, About: "How many voices"},
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 150, Hi: 800, Def: 340, Log: true, Vary: 0.03, About: "The voices' middle pitch"},
			{Name: "spread", Label: "Spread", Unit: "st", Lo: 0, Hi: 12, Def: 6, About: "How far the voices' pitches spread"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.2, Hi: 1.2, Def: 0.55, Vary: 0.04, About: "Each voice's length"},
			{Name: "ragged", Label: "Ragged", Unit: "s", Lo: 0, Hi: 0.3, Def: 0.08, Vary: 0.04, About: "How far apart the voices start"},
			{Name: "bend", Label: "Bend", Unit: "st", Lo: -7, Hi: 7, Def: 0, Vary: 0.03, About: "How far the voices bend over the call; 0 for the kind's own"},
			{Name: "claps", Label: "Claps", Lo: 0, Hi: 40, Def: 0, Step: 1, About: "How many claps"},
			{Name: "clapat", Label: "Claps at", Unit: "s", Lo: 0, Hi: 0.6, Def: 0.05, About: "When the claps start"},
			{Name: "size", Label: "Size", Lo: 0.8, Hi: 2.2, Def: 1.3, Vary: 0.03, About: "Scales the voices' mouths: higher for children"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.03, About: brightParam.About},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.04, About: "How breathy"},
		},
		render: crowd,
	})
}

// syllables are the syllables a singer sings.
var syllables = []string{"la", "oh", "yeah", "hey", "ooh", "na", "ah"}

// sungSyllable returns the mouth and the level a syllable takes over a
// note dur seconds long, and how much breath starts it: each
// consonant takes about 50 ms, however long the note.
func sungSyllable(name string, dur float64) (mouth func(u float64) [3]float64, level func(u float64) float64, h func(u float64) float64) {
	c := min(0.05/dur, 0.35)
	none := func(float64) float64 { return 0 }
	flat := func(float64) float64 { return 1 }
	switch name {
	case "la":
		return glide(mark{0, 'l'}, mark{c, 'l'}, mark{2.2 * c, 'a'}),
			func(u float64) float64 { return 0.45 + 0.55*smooth((u-0.6*c)/(1.2*c)) }, none
	case "na":
		return glide(mark{0, 'n'}, mark{c, 'n'}, mark{2 * c, 'a'}),
			func(u float64) float64 { return 0.3 + 0.7*smooth((u-0.7*c)/c) }, none
	case "oh":
		return glide(mark{0, 'o'}, mark{0.6, 'o'}, mark{1, 'u'}), flat, none
	case "ooh":
		return glide(mark{0, 'u'}), flat, none
	case "ah":
		return glide(mark{0, 'a'}), flat, none
	case "yeah":
		return glide(mark{0, 'i'}, mark{1.2 * c, 'i'}, mark{2.5 * c, 'e'}, mark{4.5 * c, 'a'}), flat, none
	case "hey":
		return glide(mark{0, 'e'}, mark{0.6, 'e'}, mark{1, 'i'}),
			func(u float64) float64 { return smooth((u - 0.8*c) / c) },
			func(u float64) float64 { return 1 - smooth((u-c)/c) }
	}
	return glide(mark{0, 'a'}), flat, none
}

// cry is a short call of an animal's voice: its pitch from a start, and
// how it moves over the call; its vowels; and how rough it is.
type cry struct {
	at, dur float64
	pitch   float64
	// contour is the pitch over the call, in semitones from pitch.
	contour func(u float64) float64
	mouth   func(u float64) [3]float64
	// attack and release are the shares of the call it fades in and
	// out over.
	attack, release float64
	// breath is how breathy it is throughout, and onsetBreath how much
	// more breath starts it.
	breath, onsetBreath float64
	// rough is how deeply a growl or trill shakes it, roughHz how fast.
	rough, roughHz       float64
	size, bright, wander float64
}

// voice sings c into out.
func (c cry) voice(out *[]float64, r *rng) {
	at, dur := c.at, c.dur
	var roll func(t float64) float64
	if c.rough > 0 {
		hz := c.roughHz * (1 + 0.05*r.norm())
		roll = func(t float64) float64 {
			ph := math.Mod(math.Max(t-at, 0)*hz, 1)
			return 1 - c.rough + c.rough*1.4*math.Exp(-4*ph)
		}
	}
	sing(out, syllable{
		at: at, dur: dur,
		pitch: func(u float64) float64 { return c.pitch * semis(c.contour(u)) },
		level: func(u float64) float64 { return smooth(u/c.attack) * smooth((1-u)/c.release) },
		breath: func(u float64) float64 {
			return (c.breath + c.onsetBreath*(1-smooth(u/0.25))) * smooth(u/0.02) * smooth((1-u)/c.release)
		},
		mouth: c.mouth,
		roll:  roll,
		size:  c.size, tilt: tilt(c.bright), wander: c.wander,
	}, r)
}

// mew makes a kitten's mews.
func mew(p params, r *rng) []float64 {
	var out []float64
	n := int(p["mews"])
	at := 0.0
	for i := range n {
		dur := p["length"]
		last := i == n-1
		base := p["pitch"] * semis(p["rise"]*float64(i))
		o := p["open"]
		mouth := func(u float64) [3]float64 {
			// "m", then "i" opening toward "a", and closing to "u".
			var f [3]float64
			switch {
			case u < 0.12:
				f = vowel('m', 'i', smooth(u/0.12))
			case u < 0.55:
				f = vowel('i', 'a', smooth((u-0.12)/0.43)*o)
			default:
				v := vowel('i', 'a', o)
				w := vowels['u']
				t := smooth((u-0.55)/0.45) * p["close"]
				for k := range f {
					f[k] = v[k] * math.Pow(w[k]/v[k], t)
				}
			}
			return f
		}
		q := 0.0
		if last {
			q = p["question"]
		}
		c := cry{
			at: at, dur: dur, pitch: base,
			contour: func(u float64) float64 {
				st := p["arch"]*smooth(u/0.3) - (p["arch"]+p["fall"])*smooth((u-0.35)/0.65)*(1-min(q, 1))
				return st + q*smooth((u-0.45)/0.55)
			},
			mouth: mouth, attack: 0.06, release: 0.3,
			breath: p["breath"] * 0.4, size: p["size"], bright: p["bright"], wander: 0.006,
			rough: p["trill"], roughHz: p["trillhz"],
		}
		c.voice(&out, r)
		at += dur + p["gap"]
	}
	return out
}

// yip makes a fox's yips.
func yip(p params, r *rng) []float64 {
	var out []float64
	n := int(p["yips"])
	at := 0.0
	for i := range n {
		dur := p["length"]
		last := i == n-1
		q := 0.0
		if last {
			dur *= p["last"]
			q = p["question"]
		}
		base := p["pitch"] * semis(p["rise"]*float64(i)) * (1 + 0.015*r.norm())
		o := p["open"]
		c := cry{
			at: at, dur: dur, pitch: base,
			contour: func(u float64) float64 {
				st := -p["leap"]*(1-smooth(u/0.25)) - p["drop"]*smooth((u-0.5)/0.5)*(1-min(q/3, 1))
				return st + q*smooth((u-0.4)/0.6)
			},
			mouth: func(u float64) [3]float64 {
				// "y", then the vowel: "e" opening to "a".
				f := vowel('e', 'a', o)
				i := vowels['i']
				t := smooth(u / 0.2)
				for k := range f {
					f[k] = i[k] * math.Pow(f[k]/i[k], t)
				}
				return f
			},
			attack: 0.05, release: 0.3,
			breath: p["breath"] * 0.4, size: p["size"], bright: p["bright"], wander: 0.01,
			rough: p["rough"] * 0.6, roughHz: 70,
		}
		c.voice(&out, r)
		at += dur + p["gap"]
	}
	return out
}

// roar makes a small creature's roar, ending in a squeak where asked.
func roar(p params, r *rng) []float64 {
	var out []float64
	dur := p["length"]
	sq := p["squeak"]
	main := dur * (1 - 0.5*sq)
	o := p["open"]
	c := cry{
		at: 0, dur: main, pitch: p["pitch"],
		contour: func(u float64) float64 {
			return -2*(1-smooth(u/0.25)) + p["arch"]*math.Sin(math.Pi*min(u, 1)) - 2*smooth((u-0.7)/0.3)
		},
		mouth: glide(mark{0, 'o'}, mark{0.3, 'a'}, mark{0.75, 'a'}, mark{1, 'o'}),
		// "r-a-w-r": the growl strongest at the start.
		attack: 0.06, release: 0.25,
		breath: p["breath"] * 0.5, onsetBreath: p["breath"] * 0.5,
		rough: p["growl"], roughHz: p["growlhz"], size: p["size"], bright: p["bright"], wander: p["wobble"],
	}
	if o < 1 {
		base := c.mouth
		c.mouth = func(u float64) [3]float64 { return scale(base(u), 0.75+0.25*o) }
	}
	c.voice(&out, r)
	if sq > 0 {
		// The squeak: the voice cracks up high, short and clean.
		sdur := dur * 0.5 * sq
		hi := p["pitch"] * semis(p["squeakup"])
		s := cry{
			at: main - 0.02, dur: sdur + 0.02, pitch: hi,
			contour: func(u float64) float64 { return -1*(1-smooth(u/0.3)) + 1.5*smooth((u-0.5)/0.5) },
			mouth:   glide(mark{0, 'e'}, mark{0.4, 'i'}),
			attack:  0.15, release: 0.4, breath: 0.1, size: p["size"], bright: p["bright"], wander: 0.01,
		}
		s.voice(&out, r)
	}
	return out
}

// singPhrase makes a sung phrase.
func singPhrase(p params, r *rng) []float64 {
	var out []float64
	n := int(p["notes"])
	notes := []float64{0, p["n2"], p["n3"], p["n4"]}
	at := 0.0
	ring := &resonance{hz: 3000 * p["size"] / 1.2, width: 1200, level: p["ring"]}
	for i := range n {
		dur := p["length"]
		last := i == n-1
		syl := syllables[int(p["syllable"])]
		if last {
			dur *= p["last"]
			syl = syllables[int(p["lastsyl"])]
		}
		hz := p["pitch"] * semis(notes[i])
		mouth, cons, h := sungSyllable(syl, dur)
		fall := 0.0
		if last {
			fall = p["fall"]
		}
		start := at
		vrate := 5.6 * (1 + 0.05*r.norm())
		sing(&out, syllable{
			at: at, dur: dur,
			pitch: func(u float64) float64 {
				t := u * dur
				st := -p["scoop"] * (1 - smooth(t/0.06))
				st -= fall * smooth((u-0.5)/0.5)
				// The vibrato comes in once a note has held a moment.
				st += p["vibrato"] * smooth((t-0.12)/0.15) * math.Sin(2*math.Pi*vrate*(start+t))
				return hz * semis(st)
			},
			level: func(u float64) float64 {
				rel := 0.15
				if last {
					rel = 0.35
				}
				return cons(u) * smooth(u*dur/0.012) * smooth((1-u)/rel)
			},
			breath: func(u float64) float64 {
				return (p["breath"]*0.35 + 0.9*h(u)) * smooth(u*dur/0.01) * smooth((1-u)/0.3)
			},
			mouth: mouth,
			size:  p["size"], tilt: tilt(p["bright"]), body: ring, wander: 0.005,
		}, r)
		at += dur + p["gap"]
	}
	return out
}

// crowd makes a small crowd's call: many voices, and claps.
func crowd(p params, r *rng) []float64 {
	var out []float64
	kind := int(p["kind"])
	n := int(p["voices"])
	for range n {
		at := p["ragged"] * r.float()
		dur := p["length"] * (0.8 + 0.4*r.float())
		hz := p["pitch"] * semis(p["spread"]*(r.float()-0.5))
		var mouth func(u float64) [3]float64
		var contour func(u float64) float64
		bend := p["bend"]
		switch kind {
		case 1: // "ooh": a sympathetic sigh, falling.
			mouth = glide(mark{0, 'o'}, mark{0.3, 'u'})
			if bend == 0 {
				bend = -4
			}
			contour = func(u float64) float64 { return 1.5*smooth(u/0.2) + bend*smooth((u-0.25)/0.75) }
		case 2: // "aah"
			mouth = glide(mark{0, 'a'})
			if bend == 0 {
				bend = -2
			}
			contour = func(u float64) float64 { return bend * smooth(u) }
		default: // "yay": rising, then falling away.
			mouth = glide(mark{0, 'i'}, mark{0.12, 'e'}, mark{0.35, 'a'}, mark{1, 'e'})
			if bend == 0 {
				bend = 4
			}
			contour = func(u float64) float64 { return bend*smooth(u/0.35) - 1.5*bend*smooth((u-0.5)/0.5) }
		}
		size := p["size"] * (0.9 + 0.2*r.float())
		sing(&out, syllable{
			at: at, dur: dur,
			pitch: func(u float64) float64 { return hz * semis(contour(u)) },
			level: func(u float64) float64 { return smooth(u/0.15) * smooth((1-u)/0.4) },
			breath: func(u float64) float64 {
				return p["breath"] * smooth(u/0.15) * smooth((1-u)/0.4)
			},
			mouth: mouth, size: size, tilt: tilt(p["bright"]), wander: 0.012, top: 6000,
		}, r)
	}
	// Each voice adds to the crowd's power, not its level: bring it to
	// about one voice's.
	g := 1 / math.Sqrt(float64(n))
	for i := range out {
		out[i] *= g
	}
	for range int(p["claps"]) {
		t := p["clapat"] + 0.5*p["length"]*r.float()
		add(&out, clap(r), t, 0.6+0.4*r.float())
	}
	return out
}

// clap returns a hand clap: a few quick bursts of noise as the hands
// meet, ringing about 1.5 kHz.
func clap(r *rng) []float64 {
	n := int(0.06 * rate)
	out := make([]float64, n)
	bp := biquad{}
	hz := 1200 + 800*r.float()
	bp.bandPass(hz, hz*0.8)
	g := 1 / math.Sqrt(math.Pi*hz*0.8/rate)
	for i := range out {
		t := float64(i) / rate
		// The flesh meets in steps a few milliseconds apart.
		env := math.Exp(-t/0.012) + 0.6*math.Exp(-math.Abs(t-0.004)/0.002) + 0.4*math.Exp(-math.Abs(t-0.009)/0.002)
		out[i] = bp.step(r.norm()) * g * env * 0.4
	}
	return out
}
