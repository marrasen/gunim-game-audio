package soundfont

import (
	"bytes"
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestTheDefaultModulatorsActAsTheyAlwaysHave(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	loud, soft := f.Regions(0, 0, 60, 127)[0], f.Regions(0, 0, 60, 64)[0]
	// The velocity's level is its square, as 960 cB on a concave curve
	// is.
	if got, want := soft.Gain/loud.Gain, (64.0/127)*(64.0/127); math.Abs(got-want) > 1e-3 {
		t.Errorf("velocity 64 plays at %.4f of 127's level, want %.4f", got, want)
	}
	// The filter closes by two octaves at most, in a line.
	if got, want := 1200*math.Log2(soft.Cutoff/loud.Cutoff), -2400*(1-64.0/127); math.Abs(got-want) > 1 {
		t.Errorf("velocity 64 closes the filter by %.0f cents, want %.0f", got, want)
	}
}

func TestAZonesModulatorTakesTheDefaultsPlace(t *testing.T) {
	var sum [genCount]float64
	open := mod{src: 0x0102, dest: gFilterFc, amount: 0}
	modulate(&sum, 60, 0, &zone{}, &zone{mods: []mod{open}}, &zone{}, &zone{})
	if sum[gFilterFc] != 0 {
		t.Errorf("a zone's own velocity-to-filter of 0 left %v cents", sum[gFilterFc])
	}
	sum = [genCount]float64{}
	modulate(&sum, 60, 0, &zone{}, &zone{}, &zone{}, &zone{})
	if sum[gFilterFc] != -2400 {
		t.Errorf("the default closes the filter by %v cents at velocity 0, want 2400", sum[gFilterFc])
	}
}

func TestAPresetsModulatorsAddToTheInstruments(t *testing.T) {
	var sum [genCount]float64
	// MuseScore General's piano: the velocity opens the filter's
	// envelope, and the key moves its cutoff from the middle each way.
	env := mod{src: srcVelocity, dest: gModEnvToFilter, amount: 6500}
	key := mod{src: 0x0203, dest: gFilterFc, amount: 1929}
	inst := mod{src: srcVelocity, dest: gModEnvToFilter, amount: 1000}
	modulate(&sum, 127, 127, &zone{}, &zone{mods: []mod{inst}}, &zone{mods: []mod{env}}, &zone{mods: []mod{key}})
	if sum[gModEnvToFilter] != 7500 {
		t.Errorf("the instrument's 1000 and the preset's 6500 sum to %v", sum[gModEnvToFilter])
	}
	if !near(sum[gFilterFc], 1929) {
		t.Errorf("the top key moves the cutoff %v cents, want 1929", sum[gFilterFc])
	}
	sum = [genCount]float64{}
	modulate(&sum, 0, 127, &zone{}, &zone{}, &zone{}, &zone{mods: []mod{key}})
	if !near(sum[gFilterFc], -1929) {
		t.Errorf("the bottom key moves the cutoff %v cents, want -1929", sum[gFilterFc])
	}
}

func TestAModulatorOfAControllerIsLeftOut(t *testing.T) {
	var sum [genCount]float64
	wheel := mod{src: srcCC | 1, dest: gVibLFOToPitch, amount: 50}
	scaled := mod{src: srcVelocity, dest: gReverb, amount: 200, amtSrc: srcCC | 91}
	modulate(&sum, 60, 100, &zone{}, &zone{mods: []mod{wheel, scaled}}, &zone{}, &zone{})
	if sum[gVibLFOToPitch] != 0 || sum[gReverb] != 0 {
		t.Errorf("controllers moved the vibrato by %v and the reverb by %v", sum[gVibLFOToPitch], sum[gReverb])
	}
}

func TestTheSourcesCurves(t *testing.T) {
	for _, c := range []struct {
		src      uint16
		key, vel int
		want     float64
	}{
		{srcVelocity, 0, 127, 1},
		{0x0102, 0, 127, 0}, // negative
		{0x0203, 0, 0, -1},  // bipolar, the bottom key
		{0x0C02, 0, 63, 0},  // a switch, below the half
		{0x0C02, 0, 64, 1},  // and above it
		{0x0402, 0, 127, 1}, // concave, at the top
		{0x0802, 0, 0, 0},   // convex, at the bottom
		{srcNone, 0, 0, 1},  // no controller
	} {
		if got, ok := source(c.src, c.key, c.vel); !ok || !near(got, c.want) {
			t.Errorf("source %#04x for key %d, velocity %d gives %v, want %v", c.src, c.key, c.vel, got, c.want)
		}
	}
	if a, b := shape(1, 0.5), shape(2, 0.5); !(a < 0.5 && b > 0.5) {
		t.Errorf("at a half, concave gives %v and convex %v: concave should stay under, convex over", a, b)
	}
}
