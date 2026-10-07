package calls

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
)

func TestALoopRunsOnIntoItsStartWithoutASeam(t *testing.T) {
	lib := library(t)
	for seed := range uint64(5) {
		tk, err := lib.Take("rymden", "ufo", seed)
		if err != nil {
			t.Fatal(err)
		}
		x := tk.Samples
		if !tk.Stats.Loop || math.Abs(tk.Stats.Length-lib.Companion("rymden").Calls["ufo"].Loop) > 1e-3 {
			t.Fatalf("the ufo's take lasts %.3f s, a loop %v", tk.Stats.Length, tk.Stats.Loop)
		}
		// The step from the loop's end to its start is as small as the
		// steps within it.
		var steps []float64
		for i := 1; i < len(x); i++ {
			steps = append(steps, math.Abs(float64(x[i]-x[i-1])))
		}
		slices.Sort(steps)
		usual := steps[len(steps)*99/100]
		if seam := math.Abs(float64(x[0] - x[len(x)-1])); seam > 1.5*usual {
			t.Errorf("take %d: the seam steps %.4f, where 99%% of steps are under %.4f", seed, seam, usual)
		}
	}
}

func TestAPlayerStartsALoopAndStopsIt(t *testing.T) {
	lib := library(t)
	mix := audio.NewMixer()
	p := NewPlayer(mix, lib)
	v, err := p.Start("rymden", "ufo", audio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := p.Start("rymden", "ufo", audio.Options{}); again != v {
		t.Error("starting the ufo again started a second one")
	}
	// Played well past its loop, it plays on.
	buf := make([]float32, 2*4800)
	for range 60 {
		mix.Mix(buf)
	}
	if done(v) {
		t.Fatal("the loop ended by itself")
	}
	p.Stop("rymden", "ufo", 50*time.Millisecond)
	for range 10 {
		mix.Mix(buf)
	}
	if !done(v) {
		t.Error("the loop plays on after Stop")
	}
	if _, err := p.Start("rymden", "bliip", audio.Options{}); err == nil {
		t.Error("the bliip, no loop, started as one")
	}
}

func TestAnEffectsSetNamesItsOwnSounds(t *testing.T) {
	c := library(t).Companion("rymden")
	if c.Role != Effects || !slices.Equal(c.Kinds(), []string{"whoosh", "launch", "star", "ufo", "bliip"}) {
		t.Fatalf("rymden is a %q making %v", c.Role, c.Kinds())
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "library.json"), []byte(`{"Order":["fx"]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "fx.json"), []byte(`{"Name":"FX","Role":"effects","Sounds":["zap"],"Calls":{"pow":{"Layers":[]}}}`), 0o644)
	if _, err := LoadDir(dir); err == nil {
		t.Error("an effect not in its set's sounds was read without complaint")
	}
}

func TestRingModulationMovesATonesPitchesApart(t *testing.T) {
	// A pure tone at 1 kHz, ring-modulated by 300 Hz, sounds at 700 Hz
	// and 1.3 kHz, and no longer at 1 kHz.
	lib := &Library{Master: Master{HighPass: 90, Slope: 12, Loudness: -14, Ceiling: -1}}
	call := &Call{Layers: []*Layer{{Model: "theremin", Ring: 300, RingMix: 1,
		Params: map[string]float64{"pitch": 1000, "notes": 1, "length": 0.5, "last": 1, "vibrato": 0, "bright": 0}}}}
	x := lib.Make(call, 0).Samples
	mid := x[len(x)/4 : len(x)*3/4]
	at := func(hz float64) float64 {
		var re, im float64
		for i, v := range mid {
			a := 2 * math.Pi * hz * float64(i) / rate
			re += float64(v) * math.Cos(a)
			im += float64(v) * math.Sin(a)
		}
		return math.Hypot(re, im)
	}
	lo, hi, f := at(700), at(1300), at(1000)
	if lo < 10*f || hi < 10*f {
		t.Errorf("at 700 Hz %.1f, 1.3 kHz %.1f, and 1 kHz still %.1f", lo, hi, f)
	}
}
