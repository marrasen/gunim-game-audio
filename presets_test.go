package music

import (
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
)

func TestEveryPresetPlays(t *testing.T) {
	ps, err := Presets()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 40 {
		t.Fatalf("the library has %d presets, want 40 or more", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if p.Name == "" || p.Category == "" || p.About == "" {
			t.Errorf("a preset has no name, category or words: %+v", p)
		}
		if seen[p.Category+"/"+p.Name] {
			t.Errorf("%s/%s is in the library twice", p.Category, p.Name)
		}
		seen[p.Category+"/"+p.Name] = true
		var x []float32
		if p.Patch.Kind == "drums" {
			drum := map[string]string{"Commodore 64 kit": "sbd", "NES kit": "nbd"}[p.Name]
			if drum == "" {
				drum = "bd"
			}
			x, err = synth.PreviewDrum(p.Patch, drum, 0.5)
		} else {
			pitch := 60
			if p.Category == "Bass" {
				pitch = 36
			}
			x, err = synth.PreviewPatch(p.Patch, pitch, 0.4, 0.6)
		}
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
			continue
		}
		var sum, peak float64
		for _, s := range x {
			if math.IsNaN(float64(s)) {
				t.Fatalf("%s makes NaN", p.Name)
			}
			sum += float64(s * s)
			peak = max(peak, math.Abs(float64(s)))
		}
		rms := math.Sqrt(sum / float64(len(x)))
		if rms < 0.01 || peak > 4 {
			t.Errorf("%s plays at %.4f, its peak %.2f", p.Name, rms, peak)
		}
	}
}
