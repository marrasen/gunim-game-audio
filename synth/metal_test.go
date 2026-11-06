package synth

import (
	"math"
	"testing"
)

// strikeCymbal strikes a cymbal of kind k hard, with noise of its own seed, as the same each time.
func strikeCymbal(k int) (*plate, *hit) { return strikeCymbalAs(k, 1, 1) }

// strikeCymbalAs strikes it as hard as vel says, ringing for decay times its own length.
func strikeCymbalAs(k int, vel, decay float32) (*plate, *hit) {
	h := &hit{vel: vel, noise: newRand(7)}
	m := &h.cym
	m.start(h, k, 1, 0, decay)
	return m, h
}

func TestACymbalLetsGoOfWhatFallsPastHearing(t *testing.T) {
	for _, k := range []int{drTR909Hat, drTR909OHat, drTR909Crash, drTR909Ride, drModalHat, drModalOHat, drModalCrash, drModalRide} {
		m, h := strikeCymbal(k)
		first := m.np
		if first == 0 {
			t.Fatalf("kind %d starts with no partials", k)
		}
		for i := range int64(60 * rate) {
			if i%control == 0 && i > 0 {
				m.renorm()
			}
			m.step(i, h.noise)
			if m.quiet() {
				break
			}
		}
		if m.np >= first {
			t.Errorf("kind %d still runs %d of its %d partials as it falls silent", k, m.np, first)
		}
	}
}

// keepAll keeps every partial of m turning at an even level, as renorm does, letting go of none.
func keepAll(m *plate) {
	for i := range m.np {
		r := m.re[i]*m.re[i] + m.im[i]*m.im[i]
		k := (3 - r) / 2
		m.re[i] *= k
		m.im[i] *= k
	}
}

func TestLettingGoOfACymbalsPartsCannotBeHeard(t *testing.T) {
	for _, k := range []int{drTR909Hat, drTR909OHat, drTR909Crash, drTR909Ride, drModalCrash, drModalRide} {
		a, ha := strikeCymbal(k)
		b, hb := strikeCymbal(k)
		var peak, worst float64
		for i := range int64(rate * 4) {
			if i%control == 0 && i > 0 {
				a.renorm()
				keepAll(b)
			}
			x, y := float64(a.step(i, ha.noise)), float64(b.step(i, hb.noise))
			peak = max(peak, math.Abs(y))
			worst = max(worst, math.Abs(x-y))
		}
		if db := 20 * math.Log10(worst/peak); db > -40 {
			t.Errorf("kind %d differs by %.1f dB of its peak", k, db)
		}
	}
}

func TestACymbalOfAShortDecayKeepsABandThatStillSwells(t *testing.T) {
	// A short decay has a band's a fall faster than its b, so it sounds on as a falls past hearing.
	for _, c := range []struct {
		k          int
		vel, decay float32
	}{{drModalHat, 0.3, 0.1}, {drModalCrash, 1, 0.1}} {
		a, ha := strikeCymbalAs(c.k, c.vel, c.decay)
		b, hb := strikeCymbalAs(c.k, c.vel, c.decay)
		var worst float64
		for i := int64(0); i < rate && !b.quiet(); i++ {
			if i%control == 0 && i > 0 {
				a.renorm()
				keepAll(b)
			}
			worst = max(worst, math.Abs(float64(a.step(i, ha.noise)-b.step(i, hb.noise))))
		}
		// Against full scale: a soft hit's peak is itself quiet.
		if db := 20 * math.Log10(worst); db > -60 {
			t.Errorf("kind %d differs by %.1f dB of full scale", c.k, db)
		}
	}
}

func TestACymbalAllocatesNothing(t *testing.T) {
	m, h := strikeCymbal(drTR909Crash)
	var i int64
	if n := testing.AllocsPerRun(100, func() {
		m.renorm()
		m.step(i, h.noise)
		i++
	}); n != 0 {
		t.Errorf("it allocates %v times", n)
	}
}
