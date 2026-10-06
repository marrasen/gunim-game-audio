package calls

import (
	"math"
	"testing"
)

// rms returns how loud x is from a to b seconds.
func rms(x []float64, a, b float64) float64 {
	var s float64
	i, j := int(a*rate), min(int(b*rate), len(x))
	for _, v := range x[i:j] {
		s += v * v
	}
	return math.Sqrt(s / float64(max(j-i, 1)))
}

// sung renders one note sung on syllable syl.
func sung(syl string) []float64 {
	m := models["sing"]
	p := params{}
	for _, q := range m.Params {
		p[q.Name] = q.Def
	}
	i := 0
	for k, s := range syllables {
		if s == syl {
			i = k
		}
	}
	p["notes"], p["syllable"], p["lastsyl"], p["length"], p["last"], p["breath"] = 1, float64(i), float64(i), 0.3, 1, 0
	return m.render(p, newRNG(1))
}

// voiced returns how surely x repeats at a pitch near hz from a to b
// seconds: its autocorrelation at the period that repeats best, 1 for a
// steady tone and near 0 for breath.
func voiced(x []float64, hz, a, b float64) float64 {
	i, j := int(a*rate), min(int(b*rate), len(x))
	var best float64
	for lag := int(rate / hz * 0.85); lag <= int(rate/hz*1.15); lag++ {
		var xy, xx, yy float64
		for k := i; k+lag < j; k++ {
			xy += x[k] * x[k+lag]
			xx += x[k] * x[k]
			yy += x[k+lag] * x[k+lag]
		}
		best = max(best, xy/math.Sqrt(max(xx*yy, 1e-30)))
	}
	return best
}

func TestAnHBreathesBeforeItsVowelSounds(t *testing.T) {
	ha, ah := sung("ha"), sung("ah")
	hz := models["sing"].Params[0].Def
	// Over its first 20 ms "ha" is breath, where "ah" is already a
	// voice; by the vowel's middle both are voiced, and as loud.
	if v := voiced(ha, hz, 0, 0.02); v > 0.5 {
		t.Errorf("over its first 20 ms \"ha\" is %.2f voiced, not breath", v)
	}
	if v := voiced(ah, hz, 0, 0.02); v < 0.8 {
		t.Errorf("over its first 20 ms \"ah\" is only %.2f voiced", v)
	}
	if v := voiced(ha, hz, 0.1, 0.2); v < 0.8 {
		t.Errorf("in its middle \"ha\" is only %.2f voiced", v)
	}
	if rms(ha, 0, 0.02) < 0.05 {
		t.Error("\"ha\" is silent where its \"h\" should breathe")
	}
	if mid := rms(ha, 0.1, 0.2) / rms(ah, 0.1, 0.2); mid < 0.8 || mid > 1.25 {
		t.Errorf("in its middle \"ha\" is %.2f as loud as \"ah\"", mid)
	}
}
