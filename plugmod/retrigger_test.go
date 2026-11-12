package plugmod

import (
	"hash/fnv"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// hitsAt returns n frames of a trigger with a hit of velocity v at each
// frame of at.
func hitsAt(n int, v float32, at ...int) []float32 {
	m := make([]float32, n)
	for _, f := range at {
		m[f] = v
	}
	return m
}

// set returns the settings a processor holds after the changes kv, in
// pairs, and hits hits.
func set(hits int, kv ...float64) settings {
	s := defaults()
	for i := 0; i+1 < len(kv); i += 2 {
		s.take(uint32(kv[i]), kv[i+1])
	}
	s.draw = int64(hits) * drawStep
	return s
}

func TestTheRetriggerInputIsATrigger(t *testing.T) {
	if RetriggerIn != 4 || len(Descriptor.ControlIn) != 5 {
		t.Fatalf("the Retrigger input is input %d of %d, want 4 of 5", RetriggerIn, len(Descriptor.ControlIn))
	}
	if p := Descriptor.ControlIn[RetriggerIn]; p.Name != "Retrigger" || !p.Trigger || p.Min != 0 || p.Max != 1 {
		t.Errorf("the Retrigger input is %+v, want a trigger named Retrigger from 0 to 1", p)
	}
	if HitsID != 22 {
		t.Errorf("the hits report as parameter %d, want 22", HitsID)
	}
}

func TestAHitPutsTheWaveAtItsPhaseOnItsFrame(t *testing.T) {
	// The hit falls on frame 40 of a block of 64.
	const frames, n = 1500, 1000
	for shape, name := range Shapes {
		for start, from := range Starts {
			for sync := range 2 {
				kv := []float64{float64(ShapeID), float64(shape), float64(StartID), float64(start),
					float64(SyncID), float64(sync), float64(RateID), 31, float64(NoteID), 14, float64(PhaseID), 90,
					float64(SeedID), 7, float64(JitterID), 0.2, float64(SmoothID), 0.2 * float64(shape%2)}
				out := modded(t, frames, RetriggerIn, hitsAt(frames, 0.6, n), kv...)
				s := set(1, kv...)
				for o := range out {
					if want := float32(s.wave(float64(o) / 4)); out[o][n] != want {
						t.Errorf("%s from %s, sync %d: output %d on the frame of the hit is %v, want %v", name, from, sync,
							o, out[o][n], want)
					}
				}
				if sync == 1 {
					continue
				}
				// A free LFO goes on from there at its rate.
				if want := float32(s.wave(31.0 / rate)); out[MainOut][n+1] != want {
					t.Errorf("%s from %s: the frame after the hit is %v, want %v", name, from, out[MainOut][n+1], want)
				}
			}
		}
	}
}

func TestAHitStartsTheWaveThereAndNotAtTheStartOfTheBlock(t *testing.T) {
	const frames, n = 256, 100
	kv := []float64{float64(ShapeID), Saw, float64(SyncID), 0, float64(RateID), 100, float64(StartID), FromPlay}
	plain := modded(t, frames, 0, nil, kv...)[MainOut]
	out := modded(t, frames, RetriggerIn, hitsAt(frames, 1, n), kv...)[MainOut]
	for f := range n {
		if out[f] != plain[f] {
			t.Fatalf("frame %d, before the hit, is %v, want %v as with no hit", f, out[f], plain[f])
		}
	}
	for f := n; f < frames; f++ {
		if out[f] != plain[f-n] {
			t.Fatalf("frame %d is %v, want %v as %d frames from the start", f, out[f], plain[f-n], f-n)
		}
	}
}

func TestTwoHitsInOneBlockBothRestartTheWave(t *testing.T) {
	const first, second, span = 50, 180, 70
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 256})
	bl := newBlock(256, plug.Position{})
	kv := []float64{float64(ShapeID), Saw, float64(SyncID), 0, float64(RateID), 100, float64(PhaseID), 30}
	bl.b.Changes = changes(kv...)
	bl.b.ControlIn = [][]float32{nil, nil, nil, nil, hitsAt(256, 1, first, second)}
	pr.Process(&bl.b)
	out := bl.outs[MainOut]
	s := set(2, kv...)
	want := float32(s.wave(0))
	if out[first] != want || out[second] != want {
		t.Fatalf("the saw is at %v and %v on the frames of the hits, want %v at both", out[first], out[second], want)
	}
	if out[first-1] == want || out[second-1] == want {
		t.Errorf("the saw is at its phase a frame before a hit: %v and %v", out[first-1], out[second-1])
	}
	for k := range span {
		if out[first+k] != out[second+k] {
			t.Fatalf("%d frames after each hit the saw is at %v and %v, want the same", k, out[first+k], out[second+k])
		}
	}
	if got := pr.Report(HitsID); got != 2 {
		t.Errorf("the LFO reports %v hits, want 2", got)
	}
}

func TestAHitDrawsANewRandomValueAtOnce(t *testing.T) {
	const frames, first, second = 300, 100, 200
	for _, shape := range []int{SampleHold, SmoothRandom} {
		// A cycle lasts 100 seconds, so only a hit changes the value.
		out := modded(t, frames, RetriggerIn, hitsAt(frames, 1, first, second), float64(ShapeID), float64(shape),
			float64(SyncID), 0, float64(RateID), 0.01, float64(SeedID), 3)[MainOut]
		a, b, c := float64(out[first-1]), float64(out[first]), float64(out[second])
		if near(a, b, 1e-3) || near(b, c, 1e-3) || near(a, c, 1e-3) {
			t.Errorf("%s: the values before, at the first hit and at the second are %v, %v and %v, want three apart",
				Shapes[shape], a, b, c)
		}
		if !near(float64(out[second-1]), b, 1e-3) {
			t.Errorf("%s: between the hits the value moves from %v to %v", Shapes[shape], b, out[second-1])
		}
	}
	// The jitter draws new values too.
	out := modded(t, frames, RetriggerIn, hitsAt(frames, 1, first), float64(ShapeID), Square, float64(SyncID), 0,
		float64(RateID), 0.01, float64(JitterID), 1, float64(DepthID), 0.25)[MainOut]
	if out[first-1] == out[first] {
		t.Errorf("the jitter stays at %v across a hit", out[first])
	}
}

func TestTheVelocityOfAHitChangesNothing(t *testing.T) {
	const frames = 600
	kv := []float64{float64(ShapeID), SmoothRandom, float64(SyncID), 1, float64(NoteID), 14, float64(JitterID), 0.3}
	loud := modded(t, frames, RetriggerIn, hitsAt(frames, 1, 70, 333), kv...)
	soft := modded(t, frames, RetriggerIn, hitsAt(frames, 0.01, 70, 333), kv...)
	for o := range loud {
		for f := range frames {
			if loud[o][f] != soft[o][f] {
				t.Fatalf("output %d at frame %d is %v for a loud hit and %v for a soft one", o, f, loud[o][f], soft[o][f])
			}
		}
	}
}

// song runs pr over one block of 64 frames at pos, with the changes kv
// and a hit at each frame of at, and returns the main output.
func song(pr *processor, pos plug.Position, kv []float64, at ...int) []float32 {
	bl := newBlock(64, pos)
	bl.b.Changes = changes(kv...)
	bl.b.ControlIn = [][]float32{nil, nil, nil, nil, hitsAt(64, 1, at...)}
	pr.Process(&bl.b)
	return bl.outs[MainOut]
}

func TestAHitWinsOverTheSongUntilTheSongJumps(t *testing.T) {
	const hit = 10
	at := func(f int64) plug.Position {
		return plug.Position{Frame: f, Tempo: 120, EndTempo: 120, Beat: float64(f) / rate * 2}
	}
	for _, shape := range []int{Saw, SmoothRandom} {
		for sync := range 2 {
			kv := []float64{float64(ShapeID), float64(shape), float64(StartID), FromSong, float64(SyncID), float64(sync),
				float64(RateID), 300, float64(NoteID), 16}
			// fresh is an LFO that no hit reached, at the same place.
			fresh := func(pos plug.Position) []float32 {
				pr := newLFO(t)
				pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
				return song(pr, pos, kv)
			}
			for _, jump := range []string{"Reset", "the play starting"} {
				pr := newLFO(t)
				pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
				first := song(pr, at(6400), kv, hit)
				s := set(1, kv...)
				if want := float32(s.wave(0)); first[hit] != want {
					t.Fatalf("shape %d, sync %d: on the frame of the hit the wave is at %v, want %v", shape, sync,
						first[hit], want)
				}
				// The wave counts on from the hit, in the next block
				// and while the song stands still.
				next := song(pr, at(6464), kv)
				if c := pr.at(at(6528), 0, 64); !near(c, float64(128-hit)*pr.perFrame(at(6464), 0, 64), 1e-9) {
					t.Errorf("shape %d, sync %d: two blocks on, the LFO is %v cycles on from the hit", shape, sync, c)
				}
				if plain := fresh(at(6464)); next[0] == plain[0] {
					t.Errorf("shape %d, sync %d: a block after the hit the wave is back where the song is", shape, sync)
				}
				still := at(6528)
				still.Still = true
				held := song(pr, still, kv)
				if held[0] == held[63] {
					t.Errorf("shape %d, sync %d: after a hit the wave holds at %v while the song stands still", shape,
						sync, held[0])
				}
				// A jump of the song puts the LFO back where the song is.
				pos := at(96000)
				if jump == "Reset" {
					pr.Reset()
				} else {
					pos.Playing = true
				}
				got, want := song(pr, pos, kv), fresh(pos)
				for f := range got {
					if got[f] != want[f] {
						t.Fatalf("shape %d, sync %d: after %s, frame %d is %v, want %v as the song has it", shape, sync,
							jump, f, got[f], want[f])
					}
				}
			}
		}
	}
}

// unwired runs the LFO with no input connected over every shape, start
// and Sync, with jitter and, for every other shape, smoothing, and
// returns a hash of all it put out.
func unwired(t *testing.T) uint64 {
	t.Helper()
	const frames = 3000
	h := fnv.New64a()
	var b [4]byte
	for shape := range Shapes {
		for start := range Starts {
			for sync := range 2 {
				out := modded(t, frames, 0, nil, float64(ShapeID), float64(shape), float64(StartID), float64(start),
					float64(SyncID), float64(sync), float64(RateID), 31, float64(NoteID), 14, float64(PhaseID), 45,
					float64(SeedID), 7, float64(JitterID), 0.3, float64(SmoothID), 0.2*float64(shape%2))
				for _, o := range out {
					for _, v := range o {
						bits := math.Float32bits(v)
						b[0], b[1], b[2], b[3] = byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)
						h.Write(b[:])
					}
				}
			}
		}
	}
	return h.Sum64()
}

func TestWithNoWireTheOutputIsWhatItWasBeforeTheRetriggerInput(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the hash was taken on amd64, and another processor rounds the sums of the wave another way")
	}
	// The hash of the output at commit 0e6f460, before the input.
	const before = 0xe9c670c313a0d32d
	if got := unwired(t); got != before {
		t.Errorf("unwired, the outputs hash to %#x, want %#x as before the Retrigger input", got, uint64(before))
	}
}

func TestProcessAllocatesNothingAsHitsArrive(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 256})
	bl := newBlock(256, plug.Position{})
	bl.b.Changes = make([]plug.Change, 2)
	mod := make([]float32, 256)
	for i := range mod {
		mod[i] = float32(math.Sin(float64(i) / 40))
	}
	bl.b.ControlIn = [][]float32{mod, mod, mod, mod, hitsAt(256, 0.8, 0, 17, 18, 130, 255)}
	i := 0
	n := testing.AllocsPerRun(400, func() {
		bl.b.Position = plug.Position{Frame: int64(i) * 256, Playing: i%50 > 3, Tempo: 120, Beat: float64(i) * 256 / 24000}
		bl.b.Changes[0] = plug.Change{Frame: 10, ID: ShapeID, Value: float64(i % len(Shapes))}
		bl.b.Changes[1] = plug.Change{Frame: 100, ID: StartID, Value: float64(i % 3)}
		pr.Process(&bl.b)
		i++
	})
	if n != 0 {
		t.Errorf("a block with five hits allocates %v times", n)
	}
	if pr.Report(HitsID) == 0 {
		t.Error("the LFO reports no hits")
	}
}

func TestAHitFlashesTheBoltOfThePanelAndThePanelRestsAgain(t *testing.T) {
	const frame = time.Second / 60
	h := newHost()
	p, ok := Descriptor.New(h).(*plugin)
	if !ok {
		t.Fatal("New makes another type")
	}
	pn, okp := p.View(plug.Panel, h).(*panel)
	c, okc := p.View(plug.Chip, h).(*chip)
	if !okp || !okc {
		t.Fatal("the views are of other types")
	}
	w := gunimtest.New(t, geom.Sz(900, 420), nil)
	gunim.RegisterView(w, "lfo", func(struct{}) gunim.Node { return pn }, nil)
	if err := w.Client().Mount(gunim.Root, "lfo", "lfo", nil); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		w.Frame(frame)
	}
	if pn.Step(frame) || pn.sc.flash.Value() != 0 {
		t.Fatalf("with no hit the panel moves, its bolt lit %v", pn.sc.flash.Value())
	}
	// A hit, and the new place of the dot with it.
	h.values[HitsID], h.values[CycleID] = 1, 0.25
	w.Frame(frame)
	if pn.sc.cycle != 0.25 {
		t.Errorf("the frame of the hit, the dot is at %v of the cycle, want 0.25", pn.sc.cycle)
	}
	top, frames := float32(0), 0
	for ; frames < 600 && pn.Step(0); frames++ {
		w.Frame(frame)
		top = max(top, pn.sc.flash.Value())
	}
	if top < flashTop {
		t.Errorf("the bolt lights to %v, want %v or more", top, flashTop)
	}
	if frames == 600 || pn.sc.flash.Value() > 0.01 {
		t.Errorf("after %d frames the panel still moves, its bolt lit %v", frames, pn.sc.flash.Value())
	}
	// The chip has no bolt.
	if c.Step(frame); c.sc.flash.Value() != 0 || c.Step(frame) {
		t.Errorf("the chip moves at a hit, a bolt lit %v", c.sc.flash.Value())
	}
}

func TestPrepareAfterHitsPlaysTheRandomValuesOfTheSeed(t *testing.T) {
	for _, shape := range []int{SampleHold, SmoothRandom} {
		for start, from := range Starts {
			for _, playing := range []bool{false, true} {
				kv := []float64{float64(ShapeID), float64(shape), float64(StartID), float64(start), float64(SyncID), 0,
					float64(RateID), 300, float64(SeedID), 5, float64(JitterID), 0.3}
				pos := plug.Position{Frame: 6400, Playing: playing}
				pr := newLFO(t)
				pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
				song(pr, pos, kv, 3, 20, 41)
				pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
				if pr.s.draw != 0 || pr.struck {
					t.Fatalf("from %s: prepared again after hits, the LFO keeps draw %d, struck %v", from, pr.s.draw, pr.struck)
				}
				fresh := newLFO(t)
				fresh.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
				got, want := song(pr, pos, kv), song(fresh, pos, kv)
				for f := range got {
					if got[f] != want[f] {
						t.Fatalf("shape %d from %s, playing %v: frame %d is %v, want %v as a new LFO plays", shape, from,
							playing, f, got[f], want[f])
					}
				}
			}
		}
	}
}
