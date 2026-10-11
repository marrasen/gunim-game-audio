package synth

import (
	"fmt"
	"strings"
)

// GMStyles are the styles a [GM] plays its instruments in: gm, its own
// instruments, each made to sound as its name says; sid, a Commodore
// 64's SID chip, its pulses, saws and triangles through its filter;
// nes, the NES's pulses, its stepped triangle for the bass, and its
// noise; gb, the Game Boy's pulses and its wave channel for the bass;
// and adlib, the two-operator FM of an AdLib or Sound Blaster card, as
// a DOS game played its music.
var GMStyles = []string{"gm", "sid", "nes", "gb", "adlib"}

// gmRole is what an instrument plays, for a style that has fewer sounds
// than General MIDI has instruments: each program plays the style's
// sound for its role.
type gmRole int

const (
	roleKeys gmRole = iota
	roleBell
	roleOrgan
	rolePluck
	roleDist
	roleBass
	rolePad
	roleBrass
	roleReed
	rolePipe
	roleLead
	roleTom
	roleNoise
)

// roleOf returns program's role.
func roleOf(program int) gmRole {
	switch {
	case program < 8:
		return roleKeys
	case program < 16:
		return roleBell
	case program < 24:
		return roleOrgan
	case program == 29 || program == 30 || program == 84:
		return roleDist
	case program < 32:
		return rolePluck
	case program < 40 || program == 47:
		return roleBass
	case program == 45 || program == 46:
		return rolePluck
	case program < 56:
		return rolePad
	case program < 64:
		return roleBrass
	case program < 72:
		return roleReed
	case program < 80:
		return rolePipe
	case program < 88:
		return roleLead
	case program < 104:
		return rolePad
	case program < 109:
		return rolePluck
	case program < 112:
		return roleReed
	case program < 115:
		return roleBell
	case program < 119:
		return roleTom
	}
	return roleNoise
}

// GMStylePatch returns instrument program as style plays it. Each call
// makes it afresh, so the patch is the caller's own.
func GMStylePatch(style string, program int) (*Patch, error) {
	program &= 127
	var p *Patch
	switch style {
	case "", "gm":
		return GMPatch(program), nil
	case "sid":
		p = sidPatch(program)
	case "nes":
		p = nesPatch(program, false)
	case "gb":
		p = nesPatch(program, true)
	case "adlib":
		p = adlibPatch(program)
	default:
		return nil, fmt.Errorf("synth: no style %q; the styles are %s", style, strings.Join(GMStyles, ", "))
	}
	p.Poly = 16
	p.Gain *= dbGain(styleLevels[style][roleOf(program)])
	return p, nil
}

// styleLevels are how many decibels each role is turned up in each
// style, measured, as gmLevels are, so that it plays as loud as the same
// instruments do in gm.
var styleLevels = map[string][roleNoise + 1]float64{
	"sid":   {10, 8.5, 6.5, 11, 10, 6, 8.5, 8.5, 5.5, 3, 6, 9, 15},
	"nes":   {10, 13.5, 5, 13.5, 7, -0.5, 9, 6, 7, 2, 7, 8, 8},
	"gb":    {10, 13, 5, 13.5, 7, 0, 4.5, 6, 7, 2.5, 7, 11, 8},
	"adlib": {8.5, 7, 5.5, 9, 3.5, 2.5, 5.5, 6, 5, 4, 7.5, 6.5, 15},
}

// GMStyleKit returns the drum kit style plays, kit the drum channel's
// program, as [GMKit] has it: the SID's drums for sid, the NES's for nes
// and gb, and General MIDI's own for gm and adlib.
func GMStyleKit(style string, kit int) *Patch {
	p := GMKit(kit)
	var types map[string]string
	switch style {
	case "sid":
		types = map[string]string{
			"kick": "sidkick", "kick909": "sidkick", "boom": "sidkick", "snare": "sidsnare", "clap": "sidclap",
			"hat": "sidhat", "ohat": "sidohat", "crash": "sidohat", "ride": "sidhat", "shaker": "sidhat",
			"tom": "sidtom", "syntom": "sidtom", "rim": "sidhat", "snap": "sidclap", "cowbell": "sidzap", "metal": "sidzap",
		}
	case "nes", "gb":
		types = map[string]string{
			"kick": "neskick", "kick909": "neskick", "boom": "neskick", "snare": "nessnare", "clap": "nesclap",
			"hat": "neshat", "ohat": "nesohat", "crash": "nesohat", "ride": "neshat", "shaker": "neshat",
			"tom": "nestom", "syntom": "nestom", "rim": "nesmetal", "snap": "nesmetal", "cowbell": "nesmetal", "metal": "nesmetal",
		}
	default:
		return p
	}
	for k, d := range p.Kit {
		if t, ok := types[d.Type]; ok {
			d.Type = t
			// A chip's drum is its frames: tone and decay stretch them as
			// they do the kit's own, so only the tune is kept, and toms
			// tuned far up, as a bongo, are brought nearer.
			d.Decay, d.Tone = 1, 0
			d.Tune = min(d.Tune, 9)
			if t == "sidohat" || t == "nesohat" {
				d.Decay = 1.5
			}
			p.Kit[k] = d
		}
	}
	return p
}

// widthOf varies a pulse's width by program, so instruments of a role
// differ.
func widthOf(program int, widths ...float64) float64 { return widths[program%len(widths)] }

// sidPatch is program as a Commodore 64 plays it.
func sidPatch(program int) *Patch {
	vib := vibrato(6, 0.18, 0.2)
	pwm := LFO{To: "width", Wave: "tri", Hz: 0.6, Depth: 0.2}
	switch roleOf(program) {
	case roleKeys:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: widthOf(program, 0.3, 0.25, 0.4)}},
			Filter: Filter{Type: "sidlp", Cutoff: 3000, Res: 0.2},
			Amp:    adsr(0.002, 0.6, 0.25, 0.15),
			LFO:    []LFO{pwm},
			Gain:   0.6,
		}
	case roleBell:
		return &Patch{
			Osc:  []Osc{{Wave: "tri"}, {Wave: "tri", Ring: true, Semi: 19, Level: 0.6}},
			Amp:  adsr(0.001, 0.5, 0, 0.2),
			Gain: 0.7,
		}
	case roleOrgan:
		return &Patch{
			Osc:  []Osc{{Wave: "pulsetri"}, {Wave: "tri", Octave: 1, Level: 0.4}},
			Amp:  adsr(0.002, 0.1, 1, 0.05),
			Gain: 0.6,
		}
	case rolePluck:
		return &Patch{
			Osc:       []Osc{{Wave: "pulse", Width: widthOf(program, 0.25, 0.15, 0.35)}},
			Filter:    Filter{Type: "sidlp", Cutoff: 1200, Res: 0.35, Env: 2},
			FilterEnv: adsr(0.001, 0.15, 0.1, 0.1),
			Amp:       adsr(0.002, 0.4, 0.15, 0.1),
			Gain:      0.7,
		}
	case roleDist:
		return &Patch{
			Osc:    []Osc{{Wave: "pulsesaw"}, {Wave: "saw", Detune: 10, Level: 0.6}},
			Filter: Filter{Type: "sidlp", Cutoff: 2500, Res: 0.5},
			Amp:    adsr(0.002, 0.5, 0.7, 0.1),
			LFO:    []LFO{vib},
			Gain:   0.45,
		}
	case roleBass:
		return &Patch{
			Osc:       []Osc{{Wave: "saw"}, {Wave: "pulse", Width: 0.5, Octave: -1, Level: 0.4}},
			Filter:    Filter{Type: "sidlp", Cutoff: 500, Res: 0.45, Env: 2.2},
			FilterEnv: adsr(0.001, 0.12, 0.1, 0.05),
			Amp:       adsr(0.002, 0.15, 0.55, 0.05),
			Gain:      0.8,
		}
	case rolePad:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.4}, {Wave: "pulse", Width: 0.4, Detune: 8, Level: 0.6}},
			Filter: Filter{Type: "sidlp", Cutoff: 1400, Res: 0.25},
			Amp:    adsr(0.25, 0.8, 0.8, 0.6),
			LFO:    []LFO{{To: "width", Wave: "tri", Hz: 0.4, Depth: 0.3}},
			Gain:   0.4,
		}
	case roleBrass:
		return &Patch{
			Osc:       []Osc{{Wave: "saw"}},
			Filter:    Filter{Type: "sidlp", Cutoff: 600, Res: 0.3, Env: 2.5},
			FilterEnv: adsr(0.05, 0.3, 0.5, 0.15),
			Amp:       adsr(0.02, 0.2, 0.8, 0.12),
			LFO:       []LFO{vib},
			Gain:      0.6,
		}
	case roleReed:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: widthOf(program, 0.2, 0.3, 0.15)}},
			Filter: Filter{Type: "sidlp", Cutoff: 2000, Res: 0.4},
			Amp:    adsr(0.01, 0.2, 0.8, 0.1),
			LFO:    []LFO{vib},
			Gain:   0.6,
		}
	case rolePipe:
		return &Patch{
			Osc:  []Osc{{Wave: "tri"}},
			Amp:  adsr(0.02, 0.2, 0.85, 0.1),
			LFO:  []LFO{vib},
			Gain: 0.9,
		}
	case roleLead:
		return &Patch{
			Osc:    []Osc{{Wave: "pulse", Width: 0.45}, {Wave: "tri", Octave: 1, Level: 0.3}},
			Filter: Filter{Type: "sidlp", Cutoff: 3500, Res: 0.3},
			Amp:    adsr(0.004, 0.3, 0.75, 0.15),
			LFO:    []LFO{vib, {To: "width", Wave: "tri", Hz: 0.5, Depth: 0.25}},
			Gain:   0.55,
		}
	case roleTom:
		return &Patch{
			Osc:  []Osc{{Wave: "tri"}, {Wave: "noise", Level: 0.2}},
			Amp:  adsr(0.001, 0.3, 0, 0.1),
			Bend: &Bend{Semis: 12, Time: 0.12},
			Gain: 0.8,
		}
	}
	return &Patch{
		Osc:    []Osc{{Wave: "noise", Octave: 2}},
		Filter: Filter{Type: "sidbp", Cutoff: 1500, Res: 0.6},
		Amp:    adsr(0.05, 0.5, 0.6, 0.4),
		Gain:   0.4,
	}
}

// nesPatch is program as a NES plays it, or, gb, a Game Boy: two pulses
// of four widths, a triangle, or the Game Boy's wave, and noise, their
// levels stepped 60 times a second.
func nesPatch(program int, gb bool) *Patch {
	chip := &Chip{Levels: 16, Hz: 60}
	if gb {
		chip.Hz = 64
	}
	vib := LFO{To: "pitch", Wave: "tri", Hz: 6, Depth: 0.15, Delay: 0.22}
	pulse := func(width float64, amp Env, lfo ...LFO) *Patch {
		return &Patch{Osc: []Osc{{Wave: "nespulse", Width: width}}, Amp: amp, LFO: lfo, Chip: chip, Gain: 0.5}
	}
	wave := func(amp Env) *Patch {
		if !gb {
			return &Patch{Osc: []Osc{{Wave: "nestri"}}, Amp: amp, Gain: 0.85}
		}
		return &Patch{Osc: []Osc{{Wave: "gbwave"}}, Amp: amp, Chip: &Chip{Levels: 4, Hz: 64}, Gain: 0.75}
	}
	switch roleOf(program) {
	case roleKeys:
		return pulse(widthOf(program, 0.25, 0.5, 0.125), adsr(0.001, 0.5, 0.3, 0.08))
	case roleBell:
		return pulse(0.125, adsr(0.001, 0.25, 0, 0.05))
	case roleOrgan:
		return pulse(0.5, adsr(0.001, 0.1, 0.9, 0.03))
	case rolePluck:
		p := pulse(widthOf(program, 0.125, 0.25), adsr(0.001, 0.3, 0.2, 0.05))
		p.Bend = &Bend{Semis: -1, Time: 0.03}
		return p
	case roleDist:
		return pulse(0.125, adsr(0.001, 0.4, 0.7, 0.05), vib)
	case roleBass:
		return wave(adsr(0.001, 0.1, 1, 0.015))
	case rolePad:
		if gb {
			return wave(adsr(0.15, 0.5, 0.8, 0.3))
		}
		p := pulse(0.5, adsr(0.15, 0.5, 0.6, 0.3))
		p.Gain = 0.35
		return p
	case roleBrass:
		return pulse(0.5, adsr(0.02, 0.3, 0.7, 0.06), vib)
	case roleReed:
		return pulse(0.25, adsr(0.01, 0.3, 0.65, 0.05), vib)
	case rolePipe:
		p := wave(adsr(0.01, 0.1, 0.95, 0.03))
		p.LFO = []LFO{vib}
		return p
	case roleLead:
		p := pulse(widthOf(program, 0.5, 0.25), adsr(0.001, 0.3, 0.55, 0.05), vib)
		p.Bend = &Bend{Semis: -1, Time: 0.03}
		return p
	case roleTom:
		p := wave(adsr(0.001, 0.25, 0, 0.03))
		p.Bend = &Bend{Semis: 12, Time: 0.1}
		return p
	}
	return &Patch{Osc: []Osc{{Wave: "nesnoise", Octave: 3}}, Amp: adsr(0.01, 0.4, 0.5, 0.2), Chip: chip, Gain: 0.35}
}

// adlibPatch is program as an AdLib card's OPL2 chip plays it: a carrier
// and its modulator, the modulator's depth falling from its strike to
// where it holds.
func adlibPatch(program int) *Patch {
	op := func(ratio, index, strike, hold float64) Osc {
		return Osc{Wave: "fm", Ratio: ratio, Index: index, Decay: strike, Sustain: hold}
	}
	vib := vibrato(6.1, 0.1, 0.25)
	switch roleOf(program) {
	case roleKeys:
		return &Patch{Osc: []Osc{op(1, 1.8, 0.4, 0.2)}, Amp: adsr(0.002, 2, 0, 0.2), Gain: 0.6}
	case roleBell:
		return &Patch{Osc: []Osc{op(widthOf(program, 3.5, 5, 7, 4), 2.5, 0.15, 0.1)}, Amp: adsr(0.001, 1, 0, 0.3), Gain: 0.6}
	case roleOrgan:
		return &Patch{Osc: []Osc{op(widthOf(program, 1, 2), 1, 1, 1)}, Amp: adsr(0.003, 0.1, 1, 0.05), Gain: 0.45}
	case rolePluck:
		return &Patch{Osc: []Osc{op(widthOf(program, 1, 3, 2), 2.2, 0.15, 0.1)}, Amp: adsr(0.001, 0.8, 0, 0.1), Gain: 0.6}
	case roleDist:
		return &Patch{Osc: []Osc{op(1, 4, 1, 1)}, Amp: adsr(0.002, 0.6, 0.7, 0.1), Drive: 0.4, Gain: 0.4}
	case roleBass:
		return &Patch{Osc: []Osc{op(1, 2.5, 0.15, 0.4)}, Amp: adsr(0.002, 0.6, 0.35, 0.08), Gain: 0.8}
	case rolePad:
		return &Patch{
			Osc:  []Osc{op(1, 1.2, 0.8, 0.7), {Wave: "fm", Ratio: 1, Index: 1, Detune: 7, Level: 0.6}},
			Amp:  adsr(0.2, 1, 0.85, 0.6),
			LFO:  []LFO{vib},
			Gain: 0.4,
		}
	case roleBrass:
		return &Patch{Osc: []Osc{op(1, 2.8, 0.3, 0.75)}, Amp: adsr(0.04, 0.2, 0.85, 0.1), LFO: []LFO{vib}, Gain: 0.45}
	case roleReed:
		return &Patch{Osc: []Osc{op(widthOf(program, 1, 2, 3), 1.8, 0.2, 0.8)}, Amp: adsr(0.02, 0.2, 0.85, 0.08), LFO: []LFO{vib}, Gain: 0.5}
	case rolePipe:
		return &Patch{Osc: []Osc{op(1, 0.4, 0.1, 0.2)}, Noise: 0.02, Amp: adsr(0.03, 0.1, 0.9, 0.08), LFO: []LFO{vib}, Gain: 0.7}
	case roleLead:
		return &Patch{Osc: []Osc{op(widthOf(program, 1, 2), 3, 0.3, 0.9)}, Amp: adsr(0.003, 0.3, 0.8, 0.1), LFO: []LFO{vib}, Gain: 0.4}
	case roleTom:
		return &Patch{Osc: []Osc{op(1.4, 3, 0.05, 0)}, Amp: adsr(0.001, 0.35, 0, 0.1), Bend: &Bend{Semis: 7, Time: 0.1}, Gain: 0.7}
	}
	return &Patch{Noise: 1, Filter: Filter{Type: "bp", Cutoff: 1500, Res: 0.3}, Amp: adsr(0.05, 0.5, 0.6, 0.4), Gain: 1.6}
}
