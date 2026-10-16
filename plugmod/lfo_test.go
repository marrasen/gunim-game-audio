package plugmod

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
	"github.com/marrasen/gunim/audio/tempo"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

const rate = 48000

// host is a host for tests, which keeps the parameters it is sent, and
// counts the edits of the state: those begun and ended, and the changes
// told.
type host struct {
	values               map[uint32]float64
	begun, ended, change int
	names                []string
}

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
func (h *host) BeginEdit(name string, _ *gunim.UI) {
	h.begun++
	h.names = append(h.names, name)
}
func (h *host) Changed(*gunim.UI)                    { h.change++ }
func (h *host) EndEdit(*gunim.UI)                    { h.ended++ }
func (h *host) Sound() (in, out plug.Sound)          { return nil, nil }
func (h *host) Traced(uint32, int64) (float32, bool) { return 0, false }

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
	for shape := range Shapes {
		ch := append(changes(float64(ShapeID), float64(shape)), synced[1:]...)
		ch = append(ch, changes(float64(NoteID), 12, float64(PhaseID), 45, float64(SeedID), 7,
			float64(JitterID), 0.3, float64(SmoothID), 0.2, float64(GlideID), 0.4)...)
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
				t.Fatalf("%s: at frame %d playback puts out %v, the render %v", Shapes[shape], f, v, whole[f])
			}
		}
	}
}

// stand runs pr for blocks of 100 frames with the song standing still at
// beat 3, the host's frame moving on as its engine runs on, after a
// first block playing up to there; it returns the main output of the
// blocks standing still.
func stand(pr *processor, blocks int, ch []plug.Change) []float32 {
	at := int64(3 * rate / 2)
	play := newBlock(100, plug.Position{Frame: at - 100, Playing: true, Tempo: 120, Beat: 3 - 100*2.0/rate})
	play.b.Changes = ch
	pr.Process(&play.b)
	out := make([]float32, 0, 100*blocks)
	for range blocks {
		bl := newBlock(100, plug.Position{Frame: at, Playing: true, Still: true, Tempo: 120, Beat: 3})
		pr.Process(&bl.b)
		out = append(out, bl.outs[MainOut]...)
		at += 100
	}
	return out
}

// An LFO counted from the song holds where the song stood still, at
// every frame, though the host's frame moves on: synced, and free.
func TestAnLFOFromTheSongHoldsWhileTheSongStandsStill(t *testing.T) {
	for _, c := range []struct {
		name string
		ch   []plug.Change
	}{
		{"synced", synced},
		{"free", changes(float64(ShapeID), Saw, float64(SyncID), 0, float64(RateID), 3)},
	} {
		pr := newLFO(t)
		pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 100})
		out := stand(pr, 5, c.ch)
		for f, v := range out {
			if v != out[0] {
				t.Fatalf("%s: standing still, frame %d puts out %v, frame 0 %v", c.name, f, v, out[0])
			}
		}
	}
}

// One running free, synced, runs on at the tempo while the song stands
// still.
func TestAFreeRunningLFORunsOnWhileTheSongStandsStill(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 100})
	out := stand(pr, 2, append(slices.Clone(synced), changes(float64(StartID), FreeRunning)...))
	// A saw at a quarter note, at 120, climbs 2 a beat: 4 a second.
	if step, want := float64(out[150]-out[50]), 100*4.0/rate; !near(step, want, 1e-5) {
		t.Fatalf("standing still, the saw moves %v over 100 frames, want %v", step, want)
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
	bl.b.Changes = make([]plug.Change, 3)
	mod := make([]float32, 256)
	for i := range mod {
		mod[i] = float32(math.Sin(float64(i) / 40))
	}
	bl.b.ControlIn = [][]float32{mod, mod, mod, mod}
	n := plugtest.AllocsUnder(t, "plugmod.(*processor).Process", func() {
		for i := range 400 {
			bl.b.Position = plug.Position{Frame: int64(i) * 256, Playing: i%50 > 3, Tempo: 120, Beat: float64(i) * 256 / 24000}
			bl.b.Changes[0] = plug.Change{Frame: 10, ID: ShapeID, Value: float64(i % len(Shapes))}
			bl.b.Changes[1] = plug.Change{Frame: 100, ID: StartID, Value: float64(i % 3)}
			bl.b.Changes[2] = plug.Change{Frame: 200, ID: SmoothID, Value: float64(i%3) / 4}
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
	// One knob shows the rate or the note value, as Sync says, and the
	// controls a setting leaves idle grey as it changes.
	knob := func(id uint32) bool {
		for _, k := range pn.knobs {
			if k.Param().ID == id {
				return k.Disabled
			}
		}
		t.Fatalf("the panel has no knob %d", id)
		return false
	}
	if pn.rate.active().Param().ID != NoteID || !knob(SeedID) || !knob(GlideID) || !pn.count.Locked {
		t.Error("synced, a sine shows another knob than the note's, or leaves the seed or the steps' controls lit")
	}
	if pn.shape.Selected() != Sine || pn.sync.Selected() != 1 {
		t.Errorf("the selectors show shape %d, sync %d", pn.shape.Selected(), pn.sync.Selected())
	}
	h.values[SyncID], h.values[ShapeID] = 0, SampleHold
	pn.show(nil)
	if pn.rate.active().Param().ID != RateID || knob(SeedID) || pn.shape.Selected() != SampleHold || pn.sync.Selected() != 0 {
		t.Error("free, a sample and hold shows another knob than the rate's, or greys the seed")
	}
	h.values[ShapeID], h.values[JitterID] = Saw, 0.5
	pn.show(nil)
	if knob(SeedID) {
		t.Error("a saw with jitter greys the seed")
	}
	h.values[ShapeID] = Steps
	pn.show(nil)
	if knob(GlideID) || pn.count.Locked || pn.count.Selected() != 7 {
		t.Errorf("the Steps shape leaves its controls grey, or shows %d steps", pn.count.Selected()+1)
	}
}

// runMap runs pr over frames from 0 to end in blocks of size frames,
// as a host fills each block's position from m, splitting the blocks at
// each tempo change, and returns the main output.
func runMap(pr *processor, m *tempo.Map, end int64, size int, ch []plug.Change) []float32 {
	edges := make([]int64, 0, len(m.Tempos()))
	for _, c := range m.Tempos() {
		e := m.Frames(c.At, rate)
		if m.QuartersAtFrame(e, rate) < float64(c.At)/tempo.PPQ {
			e++
		}
		edges = append(edges, e)
	}
	out := make([]float32, 0, end)
	for f := int64(0); f < end; {
		n := min(int64(size), end-f)
		for _, e := range edges {
			if e > f && e < f+n {
				n = e - f
			}
		}
		bl := newBlock(int(n), plug.Position{Frame: f, Playing: true, Beat: m.QuartersAtFrame(f, rate),
			Tempo: m.BPMAtFrame(f, rate), EndTempo: m.BPMReachingFrame(f+n, rate)})
		if f == 0 {
			bl.b.Changes = ch
		}
		pr.Process(&bl.b)
		out = append(out, bl.outs[MainOut]...)
		f += n
	}
	return out
}

func TestASyncedLFOIsExactAcrossARampInAnyBlocks(t *testing.T) {
	// 60 to 180 over four quarter notes, two seconds, then steady.
	m := tempo.New(60, 4, 4)
	m.SetTempos([]tempo.Tempo{{At: 0, BPM: 60, Ramp: true}, {At: 4 * tempo.PPQ, BPM: 180}})
	ch := changes(float64(ShapeID), Saw, float64(SyncID), 1, float64(NoteID), 14) // a 16th
	const end = 3 * rate
	outs := make([][]float32, 0, 2)
	for _, size := range []int{37, 1024} {
		pr := newLFO(t)
		pr.Prepare(plug.Setup{Rate: rate, MaxBlock: size})
		out := runMap(pr, m, end, size, ch)
		for f, v := range out {
			q := m.QuartersAtFrame(int64(f), rate) * 4
			want := 2*(q-math.Floor(q)) - 1
			if !near(float64(v), want, 1e-5) && !near(math.Abs(float64(v)-want), 2, 1e-5) {
				t.Fatalf("blocks of %d: at frame %d the saw is %v, want %v", size, f, v, want)
			}
		}
		outs = append(outs, out)
	}
	// The saw falls back, a 16th's edge, on the same frames in both,
	// but where an edge falls on a frame to within rounding: there it
	// may fall on that frame or the next.
	tie := func(f int) bool {
		q := m.QuartersAtFrame(int64(f), rate) * 4
		return math.Abs(q-math.Round(q)) < 1e-9
	}
	for f := 1; f < end; f++ {
		if tie(f) || tie(f-1) {
			continue
		}
		a, b := outs[0][f] < outs[0][f-1], outs[1][f] < outs[1][f-1]
		if a != b {
			t.Fatalf("at frame %d the saw falls back in blocks of 37 %v, of 1024 %v", f, a, b)
		}
	}
}

// modded runs the LFO over frames in blocks of 64 at 120 beats a
// minute, with the changes and input in at mod, or unconnected where mod
// is nil, and returns its outputs.
func modded(t *testing.T, frames, in int, mod []float32, kv ...float64) [][]float32 {
	t.Helper()
	ports := plugtest.Ports{Changes: changes(kv...), Tempo: 120}
	if mod != nil {
		ports.ControlIn = make([][]float32, in+1)
		ports.ControlIn[in] = mod
	}
	return plugtest.Drive(t, &Descriptor, newLFO(t), plug.Setup{Rate: rate, MaxBlock: 64}, frames, ports).ControlOut
}

// steady returns n frames of an input at v.
func steady(n int, v float32) []float32 {
	m := make([]float32, n)
	for i := range m {
		m[i] = v
	}
	return m
}

func TestTheRateInputMovesTheRate(t *testing.T) {
	const frames = 6000
	// A third of the way is an octave up: twice as fast.
	for _, start := range []float64{FromSong, FromPlay, FreeRunning} {
		for _, c := range []struct {
			sync float64
			hz   float64
		}{{0, 7.5}, {1, 2}} { // synced, a quarter note at 120 is 2 Hz
			out := modded(t, frames, RateIn, steady(frames, 1.0/3), float64(ShapeID), Saw, float64(SyncID), c.sync,
				float64(RateID), 7.5, float64(NoteID), 8, float64(StartID), start)
			for f := range frames {
				x := float64(f) * 2 * c.hz / rate
				want := 2*(x-math.Floor(x)) - 1
				got := float64(out[MainOut][f])
				if !near(got, want, 1e-5) && !near(math.Abs(got-want), 2, 1e-5) {
					t.Fatalf("%s, sync %v: at frame %d the saw is %v, want %v at %v Hz", Starts[int(start)], c.sync, f, got, want, 2*c.hz)
				}
			}
		}
	}
	// The input stepping from 0 to full turns the rate eight times
	// over, and the wave moves on from where it was, with no jump: a
	// sine, and a sine smoothed and jittered, whose steepest step is
	// no steeper.
	mod := make([]float32, frames)
	for f := frames / 2; f < frames; f++ {
		mod[f] = 1
	}
	for _, kv := range [][]float64{nil, {float64(SmoothID), 0.4}} {
		kv = append(kv, float64(SyncID), 0, float64(RateID), 5)
		out := modded(t, frames, RateIn, mod, kv...)
		most := 2 * math.Pi * 40 / rate // the sine's steepest step at 40 Hz
		for f := 1; f < frames; f++ {
			if d := math.Abs(float64(out[MainOut][f] - out[MainOut][f-1])); d > most*1.001 {
				t.Fatalf("%v: from frame %d to %d the sine jumps %v, more than %v", kv, f-1, f, d, most)
			}
		}
	}
}

func TestAModdedLFOFromTheSongStartsInStepAgain(t *testing.T) {
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 100})
	mod := [][]float32{steady(100, 1)}
	ch := changes(float64(ShapeID), Saw, float64(SyncID), 0, float64(RateID), 10)
	at := int64(0)
	block := func(c []plug.Change, in [][]float32) []float32 {
		bl := newBlock(100, plug.Position{Frame: at, Playing: true})
		bl.b.Changes, bl.b.ControlIn = c, in
		pr.Process(&bl.b)
		at += 100
		return bl.outs[MainOut]
	}
	block(ch, mod)
	// Unplugged, it keeps its lead on the song.
	song := func() float64 { x := float64(at) * 10 / rate; return 2*(x-math.Floor(x)) - 1 }
	if want := song(); near(float64(block(nil, nil)[0]), want, 1e-3) {
		t.Error("the LFO is back with the song as soon as the Rate input stops")
	}
	pr.Reset()
	if want := song(); !near(float64(block(nil, nil)[0]), want, 1e-6) {
		t.Errorf("after a Reset, the LFO is not where the song is, at %v", want)
	}
}

func TestTheDepthAndOffsetInputs(t *testing.T) {
	const frames = 200
	for _, c := range []struct {
		in            int
		m, hi, low    float32
		depth, offset float64
	}{
		{DepthIn, -0.5, 0.5, -0.5, 1, 0},
		{DepthIn, 0.5, 1, -1, 0.8, 0}, // 0.8 + 0.5, held at 1
		{DepthIn, -1, 0, 0, 0.5, 0},   // 0.5 - 1, held at 0
		{OffsetIn, 0.25, 0.75, -0.25, 0.5, 0},
		{OffsetIn, 1, 1, 0.5, 0.5, 0.5}, // 0.5 + 1, held at 1, and then the output too
	} {
		out := modded(t, frames, c.in, steady(frames, c.m), float64(ShapeID), Square, float64(SyncID), 0, float64(RateID), 1,
			float64(DepthID), c.depth, float64(OffsetID), c.offset)
		// At 1 Hz, the square is high for the first half second.
		if got := out[MainOut][0]; got != c.hi {
			t.Errorf("input %d at %v, depth %v, offset %v: the square is at %v, want %v", c.in, c.m, c.depth, c.offset, got, c.hi)
		}
		if got := out[Out180][0]; got != c.low {
			t.Errorf("input %d at %v, depth %v, offset %v: its other half is at %v, want %v", c.in, c.m, c.depth, c.offset, got, c.low)
		}
	}
}

func TestThePhaseInputMovesThePhase(t *testing.T) {
	const frames = 3000
	// Half way is a quarter cycle on.
	out := modded(t, frames, PhaseIn, steady(frames, 0.5), float64(SyncID), 0, float64(RateID), 7.5)
	for f := range frames {
		for o := range 4 {
			want := math.Sin(2 * math.Pi * (float64(f)*7.5/rate + 0.25 + float64(o)/4))
			if got := float64(out[o][f]); !near(got, want, 1e-6) {
				t.Fatalf("output %d at frame %d is %v, want %v", o, f, got, want)
			}
		}
	}
}

func TestAnInputAtNothingMovesNothing(t *testing.T) {
	const frames = 4000
	plain := modded(t, frames, 0, nil, float64(ShapeID), SmoothRandom, float64(SyncID), 1, float64(NoteID), 12)
	for in, port := range Descriptor.ControlIn {
		out := modded(t, frames, in, steady(frames, 0), float64(ShapeID), SmoothRandom, float64(SyncID), 1, float64(NoteID), 12)
		for o := range out {
			for f := range frames {
				if out[o][f] != plain[o][f] {
					t.Fatalf("%s at 0: output %d at frame %d is %v, want %v", port.Name, o, f, out[o][f], plain[o][f])
				}
			}
		}
	}
}
