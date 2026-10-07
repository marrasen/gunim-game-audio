package calls

import "math"

// Space's sounds: a theremin's eerie voice, a saucer's hum, and a
// rocket's rumble.

func init() {
	register(&Model{
		Name:  "theremin",
		About: "A theremin's voice: a tone nearly pure, sliding from note to note, its vibrato wide and wavering, as a film's alien sings",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 150, Hi: 3000, Def: 880, Log: true, Vary: 0.03, About: "The first note's pitch"},
			{Name: "notes", Label: "Notes", Lo: 1, Hi: 4, Def: 2, Step: 1, About: "How many notes"},
			{Name: "n2", Label: "Note 2", Unit: "st", Lo: -24, Hi: 24, Def: 5, Step: 1, About: "The second note, in semitones from the first"},
			{Name: "n3", Label: "Note 3", Unit: "st", Lo: -24, Hi: 24, Def: 0, Step: 1, About: "The third note"},
			{Name: "n4", Label: "Note 4", Unit: "st", Lo: -24, Hi: 24, Def: 0, Step: 1, About: "The fourth note"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.05, Hi: 1, Def: 0.22, Vary: 0.04, About: "Each note's length"},
			{Name: "last", Label: "Last", Unit: "×", Lo: 0.5, Hi: 4, Def: 1.5, Vary: 0.03, About: "The last note's length, times the others'"},
			{Name: "glide", Label: "Glide", Unit: "s", Lo: 0.005, Hi: 0.3, Def: 0.06, Log: true, Vary: 0.05, About: "How long the pitch takes to slide to each note"},
			{Name: "dip", Label: "Dip", Lo: 0, Hi: 1, Def: 0.3, About: "How far the tone dips between notes, as a hand moved off the antenna"},
			{Name: "fall", Label: "Fall", Unit: "st", Lo: -12, Hi: 24, Def: 0, Vary: 0.03, About: "How far the last note falls at its end; below 0 it rises"},
			{Name: "vibrato", Label: "Vibrato", Unit: "st", Lo: 0, Hi: 3, Def: 0.7, Vary: 0.05, About: "How far the pitch wavers"},
			{Name: "vibhz", Label: "Vib Hz", Unit: "Hz", Lo: 1, Hi: 14, Def: 6.5, Log: true, Vary: 0.05, About: "How fast it wavers"},
			{Name: "attack", Label: "Attack", Unit: "s", Lo: 0.002, Hi: 0.3, Def: 0.03, Log: true, About: "How softly it comes in"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.25, Vary: 0.03, About: "How much of its second and third harmonics it has: 0 a pure sine"},
		},
		render: theremin,
	})
	register(&Model{
		Name:  "hum",
		About: "A flying saucer's hum: a buzz of detuned tones beating, pulsing as it turns, wobbling, with a shimmer over it",
		Params: []Param{
			{Name: "pitch", Label: "Pitch", Unit: "Hz", Lo: 50, Hi: 800, Def: 160, Log: true, Vary: 0.03, About: "The hum's pitch"},
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.3, Hi: 8, Def: 2.5, About: "How long it hums; for a loop, a little longer than the loop"},
			{Name: "detune", Label: "Detune", Unit: "ct", Lo: 0, Hi: 40, Def: 9, Vary: 0.05, About: "How far its three tones are apart, in cents: they beat"},
			{Name: "bright", Label: "Bright", Lo: 0, Hi: 1, Def: 0.55, Vary: 0.03, About: "How strong its upper harmonics are"},
			{Name: "pulse", Label: "Pulse", Unit: "Hz", Lo: 0.5, Hi: 20, Def: 4, Log: true, Vary: 0.04, About: "How fast it pulses, as the saucer turns"},
			{Name: "depth", Label: "Depth", Lo: 0, Hi: 1, Def: 0.35, Vary: 0.04, About: "How deep the pulse is"},
			{Name: "wobble", Label: "Wobble", Unit: "st", Lo: 0, Hi: 4, Def: 0.4, Vary: 0.05, About: "How far its pitch wobbles"},
			{Name: "wobhz", Label: "Wob Hz", Unit: "Hz", Lo: 0.1, Hi: 6, Def: 0.4, Log: true, Vary: 0.05, About: "How fast it wobbles"},
			{Name: "shimmer", Label: "Shimmer", Lo: 0, Hi: 1, Def: 0.3, Vary: 0.05, About: "How loud a high, trembling tone over it is"},
			{Name: "attack", Label: "Attack", Unit: "s", Lo: 0.005, Hi: 1, Def: 0.05, Log: true, About: "How it comes in"},
		},
		render: hum,
	})
	register(&Model{
		Name:  "rumble",
		About: "A rocket's rumble: a deep roar building, its jet's rush rising in pitch, crackling, then falling away",
		Params: []Param{
			{Name: "length", Label: "Length", Unit: "s", Lo: 0.3, Hi: 3, Def: 1.6, Vary: 0.04, About: "How long it rumbles"},
			{Name: "build", Label: "Build", Lo: 0.02, Hi: 0.9, Def: 0.35, About: "How long it builds to its loudest, a share of it"},
			{Name: "fade", Label: "Fade", Lo: 0.05, Hi: 0.9, Def: 0.35, About: "How long it falls away at its end, a share of it"},
			{Name: "low", Label: "Low", Unit: "Hz", Lo: 40, Hi: 400, Def: 110, Log: true, Vary: 0.04, About: "The rumble's pitch"},
			{Name: "deep", Label: "Deep", Lo: 0, Hi: 1.5, Def: 0.8, Vary: 0.04, About: "How loud the deep rumble is"},
			{Name: "jet", Label: "Jet", Lo: 0, Hi: 1.5, Def: 0.8, Vary: 0.04, About: "How loud the jet's rush is"},
			{Name: "jethz", Label: "Jet Hz", Unit: "Hz", Lo: 200, Hi: 4000, Def: 650, Log: true, Vary: 0.05, About: "The jet's pitch where it starts"},
			{Name: "rise", Label: "Rise", Unit: "st", Lo: -12, Hi: 36, Def: 14, Vary: 0.04, About: "How far the jet's pitch rises as it lifts off"},
			{Name: "crackle", Label: "Crackle", Lo: 0, Hi: 1, Def: 0.5, Vary: 0.05, About: "How much it crackles"},
		},
		render: rumble,
	})
}

// theremin makes a theremin's phrase: one tone, gliding to each note.
func theremin(p params, r *rng) []float64 {
	n := int(p["notes"])
	notes := []float64{0, p["n2"], p["n3"], p["n4"]}
	starts := make([]float64, n+1)
	for i := range n {
		dur := p["length"]
		if i == n-1 {
			dur *= p["last"]
		}
		starts[i+1] = starts[i] + dur
	}
	total := starts[n]
	m := int(total * rate)
	out := make([]float64, m)
	vib := newDrift(r, 2)
	glide := 1 - math.Exp(-1/(p["glide"]*rate))
	st := notes[0]
	var ph, vph float64
	b := p["bright"]
	for i := range out {
		t := float64(i) / rate
		k := 0
		for k < n-1 && t >= starts[k+1] {
			k++
		}
		target := notes[k]
		u := (t - starts[k]) / (starts[k+1] - starts[k])
		if k == n-1 {
			target -= p["fall"] * smooth((u-0.45)/0.55)
		}
		st += (target - st) * glide
		// The vibrato wavers in its rate and grows as a note holds.
		vph += 2 * math.Pi * p["vibhz"] * (1 + 0.08*vib.step()) / rate
		v := p["vibrato"] * (0.5 + 0.5*smooth(u/0.5)) * math.Sin(vph)
		ph += 2 * math.Pi * p["pitch"] * semis(st+v) / rate
		amp := smooth(t/p["attack"]) * smooth((total-t)/(0.3*(starts[n]-starts[n-1])))
		if k > 0 {
			// The dip as the hand moves to the next note.
			amp *= 1 - p["dip"]*(1-smooth((t-starts[k])/0.04))
		}
		out[i] = amp * (math.Sin(ph) + 0.5*b*math.Sin(2*ph) + 0.3*b*math.Sin(3*ph))
	}
	return out
}

// hum makes a flying saucer's hum.
func hum(p params, r *rng) []float64 {
	dur := p["length"]
	n := int(dur * rate)
	out := make([]float64, n)
	tilt := 2.4 - 1.8*p["bright"]
	k := max(int(3000/p["pitch"]), 1)
	amps := make([]float64, k)
	var sum float64
	for h := range amps {
		amps[h] = math.Pow(float64(h+1), -tilt)
		sum += amps[h]
	}
	var ph [3]float64
	var wob, pul, sh float64
	cents := p["detune"]
	shimmer := p["shimmer"]
	for i := range out {
		t := float64(i) / rate
		wob += 2 * math.Pi * p["wobhz"] / rate
		pul += 2 * math.Pi * p["pulse"] / rate
		f := p["pitch"] * semis(p["wobble"]*math.Sin(wob))
		var v float64
		for j := range ph {
			ph[j] += 2 * math.Pi * f * math.Exp2(float64(j-1)*cents/1200) / rate
			// Each harmonic by the recurrence of sines, from the first's.
			c2 := 2 * math.Cos(ph[j])
			s1, s0 := math.Sin(ph[j]), 0.0
			for h := range amps {
				v += amps[h] * s1
				s1, s0 = c2*s1-s0, s1
			}
		}
		v /= 3 * sum
		v *= 1 - p["depth"]*0.5*(1+math.Sin(pul))
		if shimmer > 0 {
			sh += 2 * math.Pi * f * 12 / rate
			v += shimmer * 0.25 * math.Sin(sh) * (0.6 + 0.4*math.Sin(3*pul+1))
		}
		out[i] = v * smooth(t/p["attack"])
	}
	return out
}

// rumble makes a rocket's rumble as it lifts off.
func rumble(p params, r *rng) []float64 {
	dur := p["length"]
	build, fade := p["build"], p["fade"]
	level := func(u float64) float64 {
		// It starts at once, a third as loud, and builds.
		return (0.35 + 0.65*smooth(u/build)) * smooth((1-u)/fade) * smooth(u/0.01)
	}
	out := make([]float64, int(dur*rate))
	if d := p["deep"]; d > 0 {
		add(&out, air(r, dur, 1.2, func(float64) float64 { return p["low"] }, level), 0, d)
	}
	jet := func(u float64) float64 { return p["jethz"] * semis(p["rise"]*smooth(u)) }
	if j := p["jet"]; j > 0 {
		add(&out, air(r, dur, 1.4, jet, level), 0, j)
	}
	if c := p["crackle"]; c > 0 {
		add(&out, crackles(r, dur, 70, 0.5, func(u float64) float64 { return 2.5 * jet(u) }, 0.8*c), 0, 1)
	}
	return out
}
