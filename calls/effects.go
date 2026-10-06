package calls

import (
	"math"
	"slices"
)

// The sounds of things: bells, sparkles, air moving, a fizzle, a
// whistle, and a ball.

func init() {
	register(&Model{
		Name:  "sparkle",
		About: "A sparkle: small bells struck one after another, high, on the notes of a pentatonic scale, climbing as they go",
		Params: []Param{
			{Name: "bells", Label: "Bells", Lo: 1, Hi: 30, Def: 8, Step: 1, About: "How many bells"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.05, Hi: 1, Def: 0.35, Vary: 0.05, About: "The time the bells are struck over"},
			{Name: "low", Label: "Low", Unit: "Hz", Lo: 500, Hi: 6000, Def: 1800, Log: true, Vary: 0.03, About: "The lowest bell's pitch"},
			{Name: "high", Label: "High", Unit: "Hz", Lo: 800, Hi: 9000, Def: 4200, Log: true, Vary: 0.03, About: "The highest bell's pitch"},
			{Name: "climb", Label: "Climb", Lo: -1, Hi: 1, Def: 0.7, Vary: 0.05, About: "How surely the bells climb, above 0, or fall, below"},
			{Name: "decay", Label: "Decay", Unit: "s", Lo: 0.02, Hi: 1, Def: 0.15, Log: true, Vary: 0.05, About: "How long each bell rings"},
			{Name: "fade", Label: "Fade", Lo: -1, Hi: 1, Def: 0.3, About: "Later bells softer, above 0, or louder, below"},
			{Name: "shimmer", Label: "Shimmer", Unit: "Hz", Lo: 0, Hi: 20, Def: 6, Vary: 0.05, About: "How fast each bell's two halves beat against each other"},
		},
		render: sparkle,
	})
	register(&Model{
		Name:  "chime",
		About: "A chime: up to four notes struck on a bell, as a celesta or a music box, the last held",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 4000, Def: 1047, Log: true, Vary: 0.01, About: "The first note's pitch"},
			{Name: "notes", Label: "Notes", Lo: 1, Hi: 4, Def: 2, Step: 1, About: "How many notes"},
			{Name: "n2", Label: "Note 2", Unit: "st", Lo: -12, Hi: 24, Def: 5, Step: 1, About: "The second note, in semitones from the first"},
			{Name: "n3", Label: "Note 3", Unit: "st", Lo: -12, Hi: 24, Def: 9, Step: 1, About: "The third note"},
			{Name: "n4", Label: "Note 4", Unit: "st", Lo: -12, Hi: 24, Def: 12, Step: 1, About: "The fourth note"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.03, Hi: 0.4, Def: 0.11, Vary: 0.04, About: "The time between notes"},
			{Name: "decay", Label: "Decay", Unit: "s", Lo: 0.05, Hi: 2, Def: 0.45, Log: true, Vary: 0.04, About: "How long each note rings"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.03, About: "How strong the bell's upper partials are"},
			{Name: "chord", Label: "Chord", Lo: 0, Hi: 1, Def: 0, About: "How much the last note's chord rings with it: all the notes struck again"},
			{Name: "tremolo", Label: "Tremolo", Lo: 0, Hi: 1, Def: 0.2, About: "How deeply the notes pulse, as a vibraphone's"},
		},
		render: chime,
	})
	register(&Model{
		Name:  "whoosh",
		About: "A whoosh: air rushing past, its pitch sweeping, loudest as it passes; feet may patter under it",
		Params: []Param{
			{Name: "from", Label: "From", Unit: "Hz", Lo: 200, Hi: 8000, Def: 700, Log: true, Vary: 0.05, About: "The air's pitch where it starts"},
			{Name: "to", Label: "To", Unit: "Hz", Lo: 200, Hi: 8000, Def: 3200, Log: true, Vary: 0.05, About: "The air's pitch where it ends"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.05, Hi: 1, Def: 0.35, Vary: 0.05, About: "How long it lasts"},
			{Name: "peak", Label: "Peak", Lo: 0.05, Hi: 0.95, Def: 0.6, Vary: 0.05, About: "Where it is loudest, a share of it"},
			{Name: "start", Label: "Start", Lo: 0, Hi: 1, Def: 0.25, About: "How loud it is at its start, a share of its peak"},
			{Name: "width", Label: "Width", Lo: 0.1, Hi: 2, Def: 0.7, Log: true, Vary: 0.04, About: "How wide a band of pitches the air holds, a share of its pitch"},
			{Name: "tone", Label: "Tone", Lo: 0, Hi: 1, Def: 0.15, About: "How much it whistles, as air through a gap"},
			{Name: "patter", Label: "Patter", Lo: 0, Hi: 12, Def: 0, Step: 1, About: "How many footsteps patter under it"},
			{Name: "patterhz", Label: "Step Hz", Unit: "Hz", Lo: 300, Hi: 3000, Def: 1100, Log: true, Vary: 0.05, About: "The footsteps' pitch"},
		},
		render: whoosh,
	})
	register(&Model{
		Name:  "puff",
		About: "A puff: a short burst of air, as of smoke or a small flame, falling away",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 6000, Def: 1600, Log: true, Vary: 0.05, About: "The air's pitch where it starts"},
			{Name: "drop", Label: "Drop", Unit: "st", Lo: -12, Hi: 24, Def: 8, Vary: 0.04, About: "How far its pitch falls"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.04, Hi: 0.8, Def: 0.22, Vary: 0.05, About: "How long it lasts"},
			{Name: "width", Label: "Width", Lo: 0.1, Hi: 2, Def: 1, Log: true, Vary: 0.04, About: "How wide a band of pitches it holds"},
			{Name: "attack", Label: "Attack", Lo: 0.01, Hi: 0.5, Def: 0.06, About: "How fast it comes in, a share of it"},
			{Name: "crackle", Label: "Crackle", Lo: 0, Hi: 1, Def: 0, Vary: 0.05, About: "How much it crackles, as fire"},
		},
		render: puff,
	})
	register(&Model{
		Name:  "fizzle",
		About: "A fizzle: crackles thinning out over a hiss that falls, as a spell gone wrong",
		Params: []Param{
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.1, Hi: 1, Def: 0.4, Vary: 0.05, About: "How long it lasts"},
			{Name: "density", Label: "Density", Unit: "/s", Lo: 5, Hi: 600, Def: 140, Log: true, Vary: 0.05, About: "How many crackles a second at its start"},
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 500, Hi: 8000, Def: 2800, Log: true, Vary: 0.04, About: "The crackles' pitch where it starts"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -12, Hi: 24, Def: 10, Vary: 0.04, About: "How far the pitch falls"},
			{Name: "hiss", Label: "Hiss", Lo: 0, Hi: 1, Def: 0.4, Vary: 0.05, About: "How loud the hiss under the crackles is"},
			{Name: "thin", Label: "Thin", Lo: 0, Hi: 3, Def: 1.2, Vary: 0.05, About: "How fast the crackles thin out"},
		},
		render: fizzle,
	})
	register(&Model{
		Name:  "whistle",
		About: "A referee's pea whistle: a high tone warbled by the pea rolling in it, with breath",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 1500, Hi: 5000, Def: 2900, Log: true, Vary: 0.02, About: "The whistle's pitch"},
			{Name: "toots", Label: "Toots", Lo: 1, Hi: 4, Def: 1, Step: 1, About: "How many toots"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.04, Hi: 0.8, Def: 0.2, Vary: 0.05, About: "Each toot's length"},
			{Name: "gap", Label: "Gap", Unit: "s", Lo: 0, Hi: 0.3, Def: 0.07, Vary: 0.05, About: "The silence between toots"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 4, Def: 1, Vary: 0.03, About: "The last toot's length, times the others'"},
			{Name: "trill", Label: "Trill", Unit: "Hz", Lo: 10, Hi: 90, Def: 42, Log: true, Vary: 0.05, About: "How fast the pea rolls"},
			{Name: "depth", Label: "Depth", Lo: 0, Hi: 1, Def: 0.6, Vary: 0.04, About: "How deeply the pea warbles the tone"},
			{Name: "breath", Label: "Breath", Lo: 0, Hi: 1, Def: 0.3, Vary: 0.05, About: "How much breath is heard round the tone"},
		},
		render: whistle,
	})
	register(&Model{
		Name:  "bonk",
		About: "A ball's bonk: a hollow, pitched knock that drops as it rings, bouncing if asked",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 300, Hi: 2000, Def: 760, Log: true, Vary: 0.03, About: "The knock's pitch"},
			{Name: "drop", Label: "Drop", Unit: "st", Lo: 0, Hi: 24, Def: 7, Vary: 0.04, About: "How far its pitch drops as it rings"},
			{Name: "decay", Label: "Decay", Unit: "s", Lo: 0.01, Hi: 0.3, Def: 0.07, Log: true, Vary: 0.05, About: "How long it rings"},
			{Name: "hollow", Label: "Hollow", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.04, About: "How much the ball's shell rings above it"},
			{Name: "click", Label: "Click", Lo: 0, Hi: 1, Def: 0.4, Vary: 0.05, About: "How sharp the strike is"},
			{Name: "bounces", Label: "Bounces", Lo: 1, Hi: 6, Def: 1, Step: 1, About: "How many times it bounces"},
			{Name: "bounce", Label: "Bounce", Unit: "s", Lo: 0.04, Hi: 0.5, Def: 0.16, Vary: 0.05, About: "The time to the first bounce; each after is sooner"},
		},
		render: bonk,
	})
}

// strike adds a struck bell at hz into out, at at seconds: partials at
// ratios of hz, each at its level, ringing over decay times its share.
// A partial over the top of hearing is left out.
func strike(out *[]float64, at, hz, decay, level float64, ratios, levels, decays []float64, beat float64) {
	n := int(4 * decay * rate)
	x := make([]float64, n)
	for k, ratio := range ratios {
		f := hz * ratio
		if f > 0.45*rate || f > 16000 {
			continue
		}
		tau := decay * decays[k]
		for i := range x {
			t := float64(i) / rate
			env := math.Exp(-t/tau) * smooth(t/0.0015)
			v := math.Sin(2 * math.Pi * f * t)
			if beat > 0 {
				// Two halves a breath apart, beating.
				v = 0.5*v + 0.5*math.Sin(2*math.Pi*(f+beat)*t)
			}
			x[i] += levels[k] * env * v
		}
	}
	add(out, x, at, level)
}

// pentatonic returns the notes of the major pentatonic from lo to hi,
// in hertz.
func pentatonic(lo, hi float64) []float64 {
	var out []float64
	for oct := -1; oct < 8; oct++ {
		for _, d := range []float64{0, 2, 4, 7, 9} {
			f := lo * semis(12*float64(oct)+d)
			if f >= lo*0.999 && f <= hi*1.001 {
				out = append(out, f)
			}
		}
	}
	if len(out) == 0 {
		out = []float64{lo}
	}
	return out
}

// sparkle makes a sparkle of small bells.
func sparkle(p params, r *rng) []float64 {
	var out []float64
	n := int(p["bells"])
	scale := pentatonic(p["low"], max(p["high"], p["low"]))
	times := make([]float64, n)
	for i := range times {
		times[i] = p["length"] * r.float()
	}
	times[0] = 0
	slices.Sort(times)
	notes := make([]float64, n)
	for i := range notes {
		notes[i] = scale[int(r.float()*float64(len(scale)))]
	}
	// A climb: the notes sorted to rise with the times, as surely as
	// asked; a fall the other way.
	climb := p["climb"]
	if math.Abs(climb) > 0 {
		sorted := slices.Clone(notes)
		slices.Sort(sorted)
		if climb < 0 {
			slices.Reverse(sorted)
		}
		for i := range notes {
			if r.float() < math.Abs(climb) {
				notes[i] = sorted[i]
			}
		}
	}
	// A glockenspiel's bar: its partials at 1, 2.76 and 5.40 its pitch.
	ratios := []float64{1, 2.76, 5.40}
	levels := []float64{1, 0.35, 0.15}
	decays := []float64{1, 0.45, 0.25}
	for i := range n {
		u := float64(i) / max(float64(n-1), 1)
		lvl := (1 - 0.6*p["fade"]*u) * (0.7 + 0.3*r.float())
		strike(&out, times[i], notes[i], p["decay"]*(0.8+0.4*r.float()), lvl, ratios, levels, decays, p["shimmer"])
	}
	return out
}

// chime makes a chime of struck notes.
func chime(p params, r *rng) []float64 {
	var out []float64
	n := int(p["notes"])
	notes := []float64{0, p["n2"], p["n3"], p["n4"]}
	b := p["bright"]
	// A celesta's or a music box's tine: its partials near the
	// harmonics, the upper ones dying sooner.
	ratios := []float64{1, 2, 3.01, 4.16, 5.43}
	levels := []float64{1, 0.25 * b, 0.35 * b, 0.2 * b, 0.12 * b}
	decays := []float64{1, 0.6, 0.4, 0.25, 0.15}
	for i := range n {
		at := float64(i) * p["length"]
		lvl := 1.0
		if i == n-1 {
			lvl = 1.15
		}
		strike(&out, at, p["pitch"]*semis(notes[i]), p["decay"], lvl, ratios, levels, decays, 0)
		if i == n-1 && p["chord"] > 0 {
			for j := range n - 1 {
				strike(&out, at+0.005*float64(j+1), p["pitch"]*semis(notes[j]), p["decay"], 0.5*p["chord"], ratios, levels, decays, 0)
			}
		}
	}
	if tr := p["tremolo"]; tr > 0 {
		hz := 6 * (1 + 0.05*r.norm())
		for i := range out {
			out[i] *= 1 - tr*0.5*(1-math.Cos(2*math.Pi*hz*float64(i)/rate))
		}
	}
	return out
}

// air returns noise dur seconds long, its band about pitch(u) wide by
// width of it, at level(u), u from 0 to 1 through it: two band passes
// in turn, gliding, each block of samples tuned anew.
func air(r *rng, dur, width float64, pitch, level func(u float64) float64) []float64 {
	n := int(dur * rate)
	out := make([]float64, n)
	var a, b biquad
	for s := 0; s < n; s += block {
		u := float64(s) / float64(n)
		hz := pitch(u)
		bw := hz * width
		a.bandPass(hz, bw)
		b.bandPass(hz, bw)
		// Two band passes narrow the noise to about 0.7 of the band.
		g := 1 / math.Sqrt(math.Pi*bw*0.7/rate)
		for i := s; i < min(s+block, n); i++ {
			out[i] = b.step(a.step(r.norm())) * g * level(float64(i)/float64(n)) * 0.5
		}
	}
	return out
}

// whoosh makes air rushing past.
func whoosh(p params, r *rng) []float64 {
	dur, peak := p["length"], p["peak"]
	from, to := p["from"], p["to"]
	pitch := func(u float64) float64 { return from * math.Pow(to/from, u) }
	level := func(u float64) float64 {
		if u < peak {
			st := p["start"]
			return smooth(u/0.02) * (st + (1-st)*math.Pow(smooth(u/peak), 1.5))
		}
		return math.Pow(1-smooth((u-peak)/(1-peak)), 1.3)
	}
	out := air(r, dur, p["width"], pitch, level)
	if tone := p["tone"]; tone > 0 {
		var ph float64
		for i := range out {
			u := float64(i) / float64(len(out))
			ph += 2 * math.Pi * pitch(u) / rate
			out[i] += tone * 0.5 * level(u) * math.Sin(ph)
		}
	}
	if steps := int(p["patter"]); steps > 0 {
		for i := range steps {
			t := dur * (0.1 + 0.8*float64(i)/float64(steps)) * (1 + 0.04*r.norm())
			lvl := 0.35 + 0.65*level(t/dur)
			add(&out, knock(r, p["patterhz"]*(1+0.08*r.norm()), 0.012), t, lvl*0.6)
		}
	}
	return out
}

// puff makes a short burst of air.
func puff(p params, r *rng) []float64 {
	dur := p["length"]
	pitch := func(u float64) float64 { return p["pitch"] * semis(-p["drop"]*smooth(u)) }
	level := func(u float64) float64 { return smooth(u/p["attack"]) * math.Pow(1-u, 1.6) }
	out := air(r, dur, p["width"], pitch, level)
	if c := p["crackle"]; c > 0 {
		add(&out, crackles(r, dur, 90, 1.5, pitch, 0.9*c), 0, 1)
	}
	return out
}

// crackles returns crackles over dur seconds: tiny bursts at random
// times, density a second at the start, thinning at thin, each about
// pitch(u), at level.
func crackles(r *rng, dur, density, thin float64, pitch func(u float64) float64, level float64) []float64 {
	out := make([]float64, int(dur*rate))
	t := 0.0
	for {
		u := t / dur
		d := density * math.Pow(max(1-u, 0.02), thin)
		t += -math.Log(max(r.float(), 1e-9)) / d
		if t >= dur {
			break
		}
		hz := pitch(t/dur) * (0.7 + 0.6*r.float())
		k := knock(r, hz, 0.0015+0.002*r.float())
		amp := level * (0.3 + 0.7*r.float()) * (1 - 0.5*t/dur)
		s := int(t * rate)
		for i, v := range k {
			if s+i < len(out) {
				out[s+i] += v * amp
			}
		}
	}
	return out
}

// fizzle makes a fizzle.
func fizzle(p params, r *rng) []float64 {
	dur := p["length"]
	pitch := func(u float64) float64 { return p["pitch"] * semis(-p["fall"]*u) }
	out := crackles(r, dur, p["density"], p["thin"], pitch, 1)
	if h := p["hiss"]; h > 0 {
		hiss := air(r, dur, 0.9, func(u float64) float64 { return 1.3 * pitch(u) },
			func(u float64) float64 { return h * smooth(u/0.05) * math.Pow(1-u, 2) })
		add(&out, hiss, 0, 1)
	}
	return out
}

// whistle makes a referee's whistle's toots.
func whistle(p params, r *rng) []float64 {
	var out []float64
	n := int(p["toots"])
	at := 0.0
	for i := range n {
		dur := p["length"]
		if i == n-1 {
			dur *= p["last"]
		}
		m := int(dur * rate)
		x := make([]float64, m)
		pea := newDrift(r, 8)
		var ph, peaPh float64
		hz := p["pitch"] * (1 + 0.01*r.norm())
		depth := p["depth"]
		for j := range x {
			u := float64(j) / float64(m)
			// The pea rolls round at about its rate, unsteadily, and
			// each time it passes the hole it chokes the tone.
			peaPh += 2 * math.Pi * p["trill"] * (1 + 0.15*pea.step()) / rate
			w := math.Sin(peaPh)
			f := hz * semis(-0.6*(1-smooth(u/0.08))) * (1 + 0.025*depth*w)
			ph += 2 * math.Pi * f / rate
			am := 1 - depth*0.45*(1+w)
			env := smooth(u/0.04) * smooth((1-u)/0.08)
			x[j] = env * am * (math.Sin(ph) + 0.12*math.Sin(2*ph))
		}
		if b := p["breath"]; b > 0 {
			br := air(r, dur, 0.35, func(float64) float64 { return hz },
				func(u float64) float64 { return b * smooth(u/0.03) * smooth((1-u)/0.1) })
			for j := range x {
				x[j] += br[j]
			}
		}
		add(&out, x, at, 0.8)
		at += dur + p["gap"]
	}
	return out
}

// bonk makes a ball's knock, and its bounces.
func bonk(p params, r *rng) []float64 {
	var out []float64
	at, gap, lvl := 0.0, p["bounce"], 1.0
	for range int(p["bounces"]) {
		n := int(5 * p["decay"] * rate)
		x := make([]float64, n)
		var ph, ph2 float64
		hz := p["pitch"] * (1 + 0.02*r.norm())
		click := p["click"]
		var bp biquad
		bp.bandPass(3500, 3000)
		for i := range x {
			t := float64(i) / rate
			f := hz * semis(-p["drop"]*(1-math.Exp(-t/(0.6*p["decay"]))))
			ph += 2 * math.Pi * f / rate
			ph2 += 2 * math.Pi * f * 2.32 / rate
			env := math.Exp(-t/p["decay"]) * smooth(t/0.001)
			v := math.Sin(ph) + p["hollow"]*0.6*math.Sin(ph2)*math.Exp(-t/(0.4*p["decay"]))
			v += click * 2.5 * bp.step(r.norm()) * math.Exp(-t/0.0015)
			x[i] = env * v
		}
		add(&out, x, at, lvl)
		at += gap
		gap *= 0.7
		lvl *= 0.6
	}
	return out
}
