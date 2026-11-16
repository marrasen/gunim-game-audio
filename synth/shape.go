package synth

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// DrumShape is how a drum sounds as it is struck, for a tool to draw:
// its pitch and how loud each part of it is, a point each millisecond,
// heard from the drum itself, so it is what the drum does.
type DrumShape struct {
	// Reads says which deep settings the drum reads.
	Reads DrumFields
	// Pitch is the pitch it sounds at each millisecond, in Hz, or nil
	// for a drum with no one pitch, as a snare's noise or a cymbal.
	Pitch []float32
	// Partials are a metal drum's partials, in Hz, low to high, as Tune
	// and Spread set them, or nil for a drum that has none.
	Partials []float32
	// Body, Noise and Click are how loud each part of its sound is each
	// millisecond, each heard alone: the peak of the part over that
	// millisecond. A drum without a part has it all 0.
	Body, Noise, Click []float32
	// Step is how many times a second a chip drum's sound steps, as its
	// player switches frames, or 0 for a drum that glides.
	Step float32
}

// shapeMax is the longest a shape follows a drum for, in seconds.
const shapeMax = 3

// ShapeOf returns the shape of drum d, named name in its kit, struck at
// velocity vel, from 0 to 1. A drum's type is d.Type, or the one its
// name says, as a kit's is. It follows the drum for as long as it
// sounds, up to three seconds, and renders it four times to do so, so
// it costs as much as playing the drum that long four times over: a
// tool should take it apart from the frame it draws.
func ShapeOf(d Drum, name string, vel float64) (DrumShape, error) {
	t := d.Type
	if t == "" {
		t = drumNames[name]
	}
	k, ok := drumTypes[t]
	if !ok {
		return DrumShape{}, fmt.Errorf("synth: drum %s of type %q is none of %s", name, t, strings.Join(sortedKeys(drumTypes), ", "))
	}
	if d.Tune == 0 {
		d.Tune = tomTune[name]
	}
	d.Type = t
	dr := drum{Drum: d, name: name, kind: k, gain: 1, prog: -1}
	if d.Gain != 0 {
		dr.gain = float32(d.Gain)
	}
	v := float32(min(max(vel, 0.01), 1))
	dk := deepKinds[k]
	sh := DrumShape{Reads: dk.reads}
	follow := func(mute partMask, pitch bool) (peaks, hz []float32) {
		dr.mute = mute
		hs := newHits(1)
		h := hs.play(dr, v, 0, 40, rate/2, 0)
		n := int(min(float32(h.end)/rate, shapeMax) * 1000)
		peaks = make([]float32, 0, n)
		if pitch {
			hz = make([]float32, 0, n)
		}
		const ms = rate / 1000
		l, r := make([]float32, ms), make([]float32, ms)
		for range n {
			clear(l)
			clear(r)
			// The first frame alone, so the pitch it sets is read as the
			// millisecond starts.
			hs.render(l[:1], r[:1], true)
			if pitch {
				hz = append(hz, h.hz)
			}
			hs.render(l[1:], r[1:], true)
			var p float32
			for i := range l {
				p = max(p, abs32(l[i]), abs32(r[i]))
			}
			peaks = append(peaks, p)
		}
		return peaks, hz
	}
	whole, hz := follow(0, dk.pitch)
	sh.Pitch = hz
	// A chip drum strikes with noise before its tone sounds: its pitch
	// is its tone's from the start.
	if i := slices.IndexFunc(sh.Pitch, func(f float32) bool { return f > 0 }); i > 0 {
		for j := range i {
			sh.Pitch[j] = sh.Pitch[i]
		}
	}
	switch p := wholePart(k); p {
	case 0:
		sh.Body, _ = follow(partNoise|partClick, false)
		sh.Noise, _ = follow(partBody|partClick, false)
		sh.Click, _ = follow(partBody|partNoise, false)
	default:
		// A drum whose sound is not split into parts is all one.
		sh.Body, sh.Noise, sh.Click = make([]float32, len(whole)), make([]float32, len(whole)), make([]float32, len(whole))
		if p == partBody {
			sh.Body = whole
		} else {
			sh.Noise = whole
		}
	}
	sh.Partials = partialsOf(&dr)
	if isChipDrum(k) {
		_, fps := chipDrum(k)
		dec := float32(d.Decay)
		if dec <= 0 {
			dec = 1
		}
		sh.Step = fps / max(dec, 0.1)
	}
	return sh, nil
}

// wholePart returns the part a drum kind's whole sound counts as, where
// its sound is not split into body, noise and click, or 0 where it is.
func wholePart(k int) partMask {
	switch k {
	case drClap, drShaker, drSnap, drRiser, drDown, drTR808Clap, drTR909Clap, drTR808Maracas:
		return partNoise
	case drTimpani, drCowbell, drTR808Conga, drTR808Rim, drTR808Clave, drTR808Cowbell, drTR808Hat, drTR808OHat,
		drTR808Cymbal, drTR909Rim:
		return partBody
	}
	return 0
}

// partialsOf returns the partials of metal drum dr, as its hit rings
// them, low to high, or nil for a drum that has none.
func partialsOf(dr *drum) []float32 {
	dp := deepOf(dr, 1)
	tune := exp2(float32(dr.Tune) / 12)
	var mf [6]float32
	var out []float32
	switch dr.kind {
	case drHat, drOHat, drCrash, drRide, drTambourine, drTR808Hat, drTR808OHat, drTR808Cymbal:
		mul := map[int]float32{drHat: 1.55, drOHat: 1.55, drCrash: 2.3, drRide: 2.3, drTambourine: 4.2}[dr.kind]
		if mul == 0 {
			mul = 1
		}
		spreadOf(mf[:], metal[:], dp.spread)
		for _, f := range mf {
			out = append(out, f*mul*tune)
		}
	case drCowbell, drTR808Cowbell:
		spreadOf(mf[:2], cowbellHz[:], dp.spread)
		out = []float32{mf[0] * tune, mf[1] * tune}
	case drMetal:
		spreadOf(mf[:], metalRatios[:], dp.spread)
		for _, r := range mf {
			out = append(out, 330*tune*r)
		}
	default:
		if !isCymbal(dr.kind) {
			return nil
		}
		for _, p := range cymPartials {
			hz := p.hz
			if dp.spread != 1 {
				hz = cymCentre * float32(math.Pow(float64(hz/cymCentre), float64(dp.spread)))
			}
			if hz*tune < 0.85*rate/2 {
				out = append(out, hz*tune)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// DrumReads returns which deep settings drum d, named name in its kit,
// reads: none for a drum whose type is unknown.
func DrumReads(d Drum, name string) DrumFields {
	t := d.Type
	if t == "" {
		t = drumNames[name]
	}
	k, ok := drumTypes[t]
	if !ok {
		return 0
	}
	return deepKinds[k].reads
}
