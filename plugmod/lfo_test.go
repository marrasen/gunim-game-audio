package plugmod

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

const rate = 48000

// host is a host for tests, which keeps the parameters it is sent.
type host struct{ values map[uint32]float64 }

func newHost() *host {
	h := &host{values: map[uint32]float64{}}
	for _, p := range Descriptor.Params {
		h.values[p.ID] = p.Default
	}
	return h
}

func (h *host) Param(id uint32) float64               { return h.values[id] }
func (h *host) Begin(uint32, *gunim.UI)               {}
func (h *host) Set(id uint32, v float64, _ *gunim.UI) { h.values[id] = v }
func (h *host) End(uint32, *gunim.UI)                 {}
func (h *host) BeginEdit(string, *gunim.UI)           {}
func (h *host) Changed(*gunim.UI)                     {}
func (h *host) EndEdit(*gunim.UI)                     {}
func (h *host) Sound() (in, out plug.Sound)           { return nil, nil }
func (h *host) Traced(uint32, int64) (float32, bool)  { return 0, false }

// newLFO returns a new LFO's processor, as the registry makes it.
func newLFO(t *testing.T) *processor {
	t.Helper()
	d, ok := plug.Lookup(Descriptor.ID)
	if !ok || !d.Generator() {
		t.Fatalf("the LFO registered as %+v, ok %v: no generator", d.Kind, ok)
	}
	pr, ok := d.New(newHost()).Processor().(*processor)
	if !ok {
		t.Fatal("the LFO's processor is of another type")
	}
	return pr
}

// changes returns changes at frame 0 setting each id to its value, in
// pairs.
func changes(kv ...float64) []plug.Change {
	var out []plug.Change
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, plug.Change{ID: uint32(kv[i]), Value: kv[i+1]})
	}
	return out
}

// near reports whether a and b are within tol.
func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestAFreeSineIsWhereTheSongIsAtEveryFrame(t *testing.T) {
	const frames = 5000 // 78 blocks of 64 and one of 8
	res := plugtest.Drive(t, &Descriptor, newLFO(t), plug.Setup{Rate: rate, MaxBlock: 64}, frames,
		plugtest.Ports{Changes: changes(float64(SyncID), 0, float64(RateID), 7.5)})
	if res.Out != nil {
		t.Errorf("the LFO puts out sound: %d channels", len(res.Out))
	}
	for f := range frames {
		for out := range 4 {
			want := math.Sin(2 * math.Pi * (float64(f)*7.5/rate + float64(out)/4))
			if got := float64(res.ControlOut[out][f]); !near(got, want, 1e-6) {
				t.Fatalf("output %d at frame %d is %v, want %v", out, f, got, want)
			}
		}
	}
}

func TestTheShapes(t *testing.T) {
	s := defaults()
	for _, c := range []struct {
		shape int
		at    float64
		want  float64
	}{
		{Sine, 0.25, 1}, {Sine, 0.75, -1},
		{Triangle, 0, 0}, {Triangle, 0.125, 0.5}, {Triangle, 0.25, 1}, {Triangle, 0.5, 0}, {Triangle, 0.75, -1}, {Triangle, 0.875, -0.5},
		{Saw, 0, -1}, {Saw, 0.5, 0}, {Saw, 0.75, 0.5},
		{Square, 0.1, 1}, {Square, 0.6, -1},
		{SampleHold, 3.2, random(3, 0)}, {SampleHold, 3.9, random(3, 0)}, {SampleHold, 4.0, random(4, 0)},
		{SmoothRandom, 5, random(5, 0)}, {SmoothRandom, 5.5, (random(5, 0) + random(6, 0)) / 2},
	} {
		s.shape = c.shape
		if got := s.wave(c.at); !near(got, c.want, 1e-9) {
			t.Errorf("%s at %v is %v, want %v", Shapes[c.shape], c.at, got, c.want)
		}
	}
	// The depth scales, the offset moves, and the output stays within
	// -1 to 1; unipolar runs from 0.
	s.shape = Square
	s.depth, s.offset = 0.5, 0.25
	if a, b := s.wave(0.1), s.wave(0.6); !near(a, 0.75, 1e-9) || !near(b, -0.25, 1e-9) {
		t.Errorf("a square at half depth, up a quarter, runs %v to %v, want 0.75 to -0.25", a, b)
	}
	s.depth, s.offset = 1, 0.5
	if a := s.wave(0.1); a != 1 {
		t.Errorf("a square up a half reaches %v, want it clamped at 1", a)
	}
	s.offset, s.unipolar = 0, true
	if a, b := s.wave(0.1), s.wave(0.6); a != 1 || b != 0 {
		t.Errorf("a unipolar square runs %v to %v, want 1 to 0", a, b)
	}
	// A seed picks other values.
	if random(7, 1) == random(7, 2) || random(7, 1) != random(7, 1) {
		t.Error("the seeds pick the same values, or a seed picks two")
	}
	for n := range int64(1000) {
		if v := random(n, 3); v < -1 || v >= 1 {
			t.Fatalf("cycle %d's random value is %v, outside -1 to 1", n, v)
		}
	}
}

// block is a block a test's host runs, with its own outputs.
type block struct {
	b    plug.Block
	outs [][]float32
}

// newBlock returns a block of n frames at pos, every output connected.
func newBlock(n int, pos plug.Position) *block {
	bl := &block{outs: make([][]float32, len(Descriptor.ControlOut))}
	for i := range bl.outs {
		bl.outs[i] = make([]float32, n)
	}
	bl.b = plug.Block{Frames: n, ControlOut: bl.outs, Position: pos}
	return bl
}

// tempoMap is a song's tempos: tempo[i] from frame at[i] on.
type tempoMap struct {
	at    []int64
	tempo []float64
}

// position returns the position of frame f, as a host counts its beat.
func (m tempoMap) position(f int64) plug.Position {
	beat := 0.0
	for i := range m.at {
		end := f
		if i+1 < len(m.at) {
			end = min(f, m.at[i+1])
		}
		if end > m.at[i] {
			beat += float64(end-m.at[i]) * m.tempo[i] / 60 / rate
		}
		if i+1 >= len(m.at) || f < m.at[i+1] {
			return plug.Position{Frame: f, Playing: true, Tempo: m.tempo[i], Beat: beat}
		}
	}
	return plug.Position{Frame: f, Playing: true}
}

// run runs pr from frame from to frame to in blocks of size frames,
// each ending at a tempo change, as a host splits its blocks there, and
// returns the main output from frame from on.
func run(pr *processor, m tempoMap, from, to int64, size int, ch []plug.Change) []float32 {
	out := make([]float32, 0, to-from)
	first := true
	for f := from; f < to; {
		n := int64(size)
		for _, at := range m.at {
			if at > f && at < f+n {
				n = at - f
			}
		}
		n = min(n, to-f)
		bl := newBlock(int(n), m.position(f))
		if first {
			bl.b.Changes, first = ch, false
		}
		pr.Process(&bl.b)
		out = append(out, bl.outs[MainOut]...)
		f += n
	}
	return out
}

// synced are the changes to a saw at a quarter note, synced, from the
// song.
var synced = changes(float64(ShapeID), Saw, float64(SyncID), 1, float64(NoteID), 8)

func TestASyncedLFOFollowsTheTempoAsItChanges(t *testing.T) {
	m := tempoMap{at: []int64{0, 30000, 50000}, tempo: []float64{120, 90, 150}}
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 512})
	out := run(pr, m, 0, 80000, 512, synced)
	for f := range int64(80000) {
		beat := m.position(f).Beat
		want := 2*(beat-math.Floor(beat)) - 1
		got := float64(out[f])
		// At a wrap, a frame either side may read the other end.
		if !near(got, want, 1e-5) && !near(math.Abs(got-want), 2, 1e-5) {
			t.Fatalf("at frame %d, beat %.6f, the saw is %v, want %v", f, beat, got, want)
		}
		if f == 0 || beat-math.Floor(beat) < 0.01 {
			continue
		}
		// Each frame moves on by the tempo of the frame before.
		step, was := float64(out[f]-out[f-1]), m.position(f-1).Tempo
		if want := 2 * was / 60 / rate; !near(step, want, 1e-5) {
			t.Fatalf("from frame %d to %d the saw moves %v, want %v at %v beats a minute", f-1, f, step, want, was)
		}
	}
}

func TestASyncedLFOIsTheSameHoweverTheSongIsPlayed(t *testing.T) {
	m := tempoMap{at: []int64{0, 30000, 50000}, tempo: []float64{120, 90, 150}}
	for _, shape := range []float64{Sine, Triangle, Saw, Square, SampleHold, SmoothRandom} {
		ch := append(changes(float64(ShapeID), shape), synced[1:]...)
		ch = append(ch, changes(float64(NoteID), 12, float64(PhaseID), 45, float64(SeedID), 7)...)
		render := newLFO(t)
		render.Prepare(plug.Setup{Rate: rate, MaxBlock: 512, Offline: true})
		whole := run(render, m, 0, 90000, 512, ch)
		// Played from the middle of the song, in blocks of another
		// size, after a stop.
		play := newLFO(t)
		play.Prepare(plug.Setup{Rate: rate, MaxBlock: 37})
		part := run(play, m, 41234, 90000, 37, ch)
		for i, v := range part {
			f := 41234 + i
			if !near(float64(v), float64(whole[f]), 1e-5) {
				t.Fatalf("%s: at frame %d playback puts out %v, the render %v", Shapes[int(shape)], f, v, whole[f])
			}
		}
	}
}

func TestAnLFOCountedFromThePlayStartsAsThePlayingDoes(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 100})
	ch := changes(float64(ShapeID), Saw, float64(SyncID), 0, float64(RateID), 10, float64(StartID), FromPlay)
	at := int64(5000)
	block := func(playing bool, c []plug.Change) []float32 {
		bl := newBlock(100, plug.Position{Frame: at, Playing: playing, Tempo: 120})
		bl.b.Changes = c
		pr.Process(&bl.b)
		at += 100
		return bl.outs[MainOut]
	}
	first := block(false, ch)
	if first[0] != -1 {
		t.Errorf("the first block starts at %v, want -1, the saw's start", first[0])
	}
	stopped := block(false, nil)
	if want := float32(2*(100*10.0/rate)) - 1; !near(float64(stopped[0]), float64(want), 1e-6) {
		t.Errorf("stopped, the LFO runs on to %v, want %v", stopped[0], want)
	}
	if started := block(true, nil); started[0] != -1 || !near(float64(started[99]), 2*99*10.0/rate-1, 1e-6) {
		t.Errorf("as the playing starts, the saw runs %v to %v, want it from -1", started[0], started[99])
	}
	if on := block(true, nil); on[0] == -1 {
		t.Error("the saw starts again while it plays on")
	}
	pr.Reset()
	if again := block(true, nil); again[0] != -1 {
		t.Errorf("after a jump, the saw starts at %v, want -1", again[0])
	}
	// One free-running starts nowhere new.
	pr.Process(&plug.Block{Frames: 1, Changes: changes(float64(StartID), FreeRunning), Position: plug.Position{Frame: at}})
	at++
	block(false, nil)
	pr.Reset()
	if free := block(true, nil); free[0] == -1 {
		t.Error("a free-running LFO starts its cycle again")
	}
}

func TestAChangeTakesEffectAtItsFrame(t *testing.T) {
	const frames = 300
	res := plugtest.Drive(t, &Descriptor, newLFO(t), plug.Setup{Rate: rate, MaxBlock: 128}, frames, plugtest.Ports{
		Changes: append(changes(float64(ShapeID), Square, float64(SyncID), 0, float64(RateID), 1),
			plug.Change{Frame: 150, ID: DepthID, Value: 0.25}, plug.Change{Frame: 150, ID: OffsetID, Value: -0.5}),
	})
	for f := range frames {
		want := float32(1)
		if f >= 150 {
			want = -0.25
		}
		if got := res.ControlOut[MainOut][f]; got != want {
			t.Fatalf("at frame %d the square is %v, want %v", f, got, want)
		}
	}
}

func TestAnUnconnectedOutputIsLeftAndTheValueReported(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
	main := make([]float32, 64)
	b := &plug.Block{Frames: 64, ControlOut: [][]float32{main, nil}, Position: plug.Position{Frame: 12000, Playing: true, Tempo: 120, Beat: 0.5},
		Changes: changes(float64(ShapeID), Saw, float64(NoteID), 3)}
	pr.Process(b)
	// A bar's saw at beat 0.5, and 63 frames on.
	beat := 0.5 + 63*2.0/rate
	if want := 2*beat/4 - 1; !near(float64(main[63]), want, 1e-6) || !near(pr.Report(ValueID), want, 1e-6) ||
		!near(pr.Report(CycleID), beat/4, 1e-9) {
		t.Errorf("the saw ends the block at %v, reporting %v at %v of its cycle, want %v at %v",
			main[63], pr.Report(ValueID), pr.Report(CycleID), want, beat/4)
	}
	pr.Process(&plug.Block{Frames: 64, Position: plug.Position{Frame: 12064}})
}

func TestProcessAllocatesNothing(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 256})
	bl := newBlock(256, plug.Position{})
	bl.b.Changes = make([]plug.Change, 2)
	n := plugtest.AllocsUnder(t, "plugmod.(*processor).Process", func() {
		for i := range 400 {
			bl.b.Position = plug.Position{Frame: int64(i) * 256, Playing: i%50 > 3, Tempo: 120, Beat: float64(i) * 256 / 24000}
			bl.b.Changes[0] = plug.Change{Frame: 10, ID: ShapeID, Value: float64(i % len(Shapes))}
			bl.b.Changes[1] = plug.Change{Frame: 100, ID: StartID, Value: float64(i % 3)}
			pr.Process(&bl.b)
		}
	})
	if n != 0 {
		t.Errorf("400 blocks allocate %d times", n)
	}
}

func TestTheViewsShowAndTheScopeMoves(t *testing.T) {
	h := newHost()
	p, ok := Descriptor.New(h).(*plugin)
	if !ok {
		t.Fatal("the LFO's New makes another type")
	}
	c, okc := p.View(plug.Chip, h).(*chip)
	pn, okp := p.View(plug.Panel, h).(*panel)
	full, okf := p.View(plug.Full, h).(*panel)
	if !okc || !okp || !okf {
		t.Fatal("the views are of other types")
	}
	views := []gunim.Node{c, pn, full}
	scopes := []*scope{c.sc, pn.sc, full.sc}
	for i, v := range views {
		size := geom.Sz(900, 420)
		if i == 0 {
			size = geom.Sz(140, 24)
		}
		w := gunimtest.New(t, size, nil)
		gunim.RegisterView(w, "lfo", func(struct{}) gunim.Node { return v }, nil)
		if err := w.Client().Mount(gunim.Root, "lfo", "lfo", nil); err != nil {
			t.Fatal(err)
		}
		// Each frame, the LFO reports a value on, and the scope shows
		// it that frame: the dot, and the trace's newest.
		for fr := range 30 {
			h.values[ValueID] = math.Sin(float64(fr) / 5)
			h.values[CycleID] = float64(fr%10) / 10
			w.Frame(time.Second / 60)
			sc := scopes[i]
			if sc.value != h.values[ValueID] || sc.cycle != h.values[CycleID] {
				t.Fatalf("view %d, frame %d: the scope shows %v at %v, want %v at %v",
					i, fr, sc.value, sc.cycle, h.values[ValueID], h.values[CycleID])
			}
			if sc.traced && sc.trace[(sc.next+len(sc.trace)-1)%len(sc.trace)] != float32(h.values[ValueID]) {
				t.Fatalf("view %d, frame %d: the trace ends at another value", i, fr)
			}
		}
	}
	// The knobs a setting leaves idle grey as it changes.
	knob := func(id uint32) bool {
		for _, s := range pn.sections {
			for _, k := range s.Knobs {
				if k.Param().ID == id {
					return k.Disabled
				}
			}
		}
		t.Fatalf("the panel has no knob %d", id)
		return false
	}
	if !knob(RateID) || knob(NoteID) || !knob(SeedID) {
		t.Error("synced, a sine greys other knobs than the rate and the seed")
	}
	h.values[SyncID], h.values[ShapeID] = 0, SampleHold
	pn.dim()
	if knob(RateID) || !knob(NoteID) || knob(SeedID) {
		t.Error("free, a sample and hold greys other knobs than the note")
	}
}
