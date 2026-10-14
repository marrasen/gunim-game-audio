package soundfont

import "math"

// A bank's modulators move its generators by what a note is played
// with. Those whose sources are fixed as the note starts, its velocity
// and its key, are worked out as its regions are: a velocity that opens
// a piano's filter, a key that shortens its release. Those of a MIDI
// controller, the bend wheel or the pressure, which move while the note
// plays, are left out, as a player has its own ways with them.

// mod is a modulator, as a zone lists it: its source, the generator it
// moves, by how much at most, the source that scales that amount, and
// the transform of what it gives.
type mod struct {
	src, dest, amtSrc, trans uint16
	amount                   int16
}

// same reports whether m and o are one modulator, of which a zone's
// takes the place of one before it, as the specification has it: the
// same sources, generator and transform.
func (m mod) same(o mod) bool {
	return m.src == o.src && m.dest == o.dest && m.amtSrc == o.amtSrc && m.trans == o.trans
}

// Sources, as a modulator's source names them.
const (
	srcNone     = 0
	srcVelocity = 2
	srcKey      = 3
	// srcCC marks a MIDI controller's number, in the source's low seven
	// bits.
	srcCC = 0x80
)

// defaultMods are the modulators every instrument has unless its zones
// put one of their own in a modulator's place: the velocity turning
// the note down, by 96 dB on a concave curve, a level the square of the
// velocity; and closing its filter, by two octaves at most. The
// defaults of the controllers, which this package leaves out, are not
// listed.
var defaultMods = [...]mod{
	{src: 0x0502, dest: gAttenuation, amount: 960},
	{src: 0x0102, dest: gFilterFc, amount: -2400},
}

// modulate adds to sum, by generator, what the modulators of an
// instrument's zone iz over its global zone ig, and of a preset's zone
// pz over its global zone pg, give for a note of key at velocity vel.
// It allocates nothing, so it runs on an audio thread.
func modulate(sum *[genCount]float64, key, vel int, ig, iz, pg, pz *zone) {
	// The instrument's: its defaults, its global zone's in their place,
	// and its zone's in theirs.
	for i := range defaultMods {
		if !shadowed(defaultMods[i], ig.mods, iz.mods) {
			apply(sum, defaultMods[i], key, vel)
		}
	}
	for _, m := range ig.mods {
		if !shadowed(m, iz.mods, nil) {
			apply(sum, m, key, vel)
		}
	}
	for _, m := range iz.mods {
		apply(sum, m, key, vel)
	}
	// The preset's add to them.
	for _, m := range pg.mods {
		if !shadowed(m, pz.mods, nil) {
			apply(sum, m, key, vel)
		}
	}
	for _, m := range pz.mods {
		apply(sum, m, key, vel)
	}
}

// shadowed reports whether a or b lists a modulator that takes m's
// place.
func shadowed(m mod, a, b []mod) bool {
	for _, o := range a {
		if m.same(o) {
			return true
		}
	}
	for _, o := range b {
		if m.same(o) {
			return true
		}
	}
	return false
}

// apply adds what m gives for a note of key at velocity vel to sum,
// unless a source of it moves as the note plays.
func apply(sum *[genCount]float64, m mod, key, vel int) {
	if int(m.dest) >= genCount {
		return
	}
	x, ok := source(m.src, key, vel)
	if !ok {
		return
	}
	y, ok := source(m.amtSrc, key, vel)
	if !ok {
		return
	}
	v := float64(m.amount) * x * y
	if m.trans == 2 {
		v = math.Abs(v)
	}
	sum[m.dest] += v
}

// source returns what source src gives for a note of key at velocity
// vel, from -1 to 1, and whether this package knows it as the note
// starts: no controller, which gives 1, the velocity, or the key.
func source(src uint16, key, vel int) (float64, bool) {
	var x float64
	switch {
	case src == srcNone:
		return 1, true
	case src&srcCC != 0:
		return 0, false
	case src&0x7f == srcVelocity:
		x = float64(vel) / 127
	case src&0x7f == srcKey:
		x = float64(key) / 127
	default:
		return 0, false
	}
	if src&0x100 != 0 {
		// Negative: from 1 down to 0.
		x = 1 - x
	}
	curve := int(src >> 10)
	if src&0x200 == 0 {
		return shape(curve, x), true
	}
	// Bipolar: from -1 to 1, the curve laid out from the middle each way.
	t := 2*x - 1
	switch curve {
	case 0:
		return t, true
	case 3:
		if t >= 0 {
			return 1, true
		}
		return -1, true
	}
	if t < 0 {
		return -shape(curve, -t), true
	}
	return shape(curve, t), true
}

// shape lays x, from 0 to 1, on curve: 0 linear, 1 concave, 2 convex
// and 3 a switch, at a half.
func shape(curve int, x float64) float64 {
	x = min(max(x, 0), 1)
	switch curve {
	case 1:
		return concave(x)
	case 2:
		return 1 - concave(1-x)
	case 3:
		if x >= 0.5 {
			return 1
		}
		return 0
	}
	return x
}

// concave is the specification's concave curve: slow to rise, then
// steep, so a velocity's level falls by decibels.
func concave(x float64) float64 {
	if x >= 1 {
		return 1
	}
	return min(max(-40.0/96*math.Log10(1-x), 0), 1)
}

// genCount is how many generators the specification numbers.
const genCount = 61
