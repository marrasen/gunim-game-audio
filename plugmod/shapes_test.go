package plugmod

import (
	"bytes"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// withShape returns the default settings at shape, seeded with seed.
func withShape(shape int, seed uint64) settings {
	s := defaults()
	s.shape, s.seed = shape, seed
	return s
}

func TestEveryShapeStaysInRangeAndRepeats(t *testing.T) {
	for shape, name := range Shapes {
		s := withShape(shape, 3)
		for i := range 2000 {
			c := float64(i)/1000 - 0.37
			v := s.shapeAt(c)
			if v < -1 || v > 1 || math.IsNaN(v) {
				t.Fatalf("%s at %v is %v, outside -1 to 1", name, c, v)
			}
			if randomShape(shape) {
				continue
			}
			for _, k := range []float64{1, 7, 1000} {
				if w := s.shapeAt(c + k); !near(v, w, 1e-9) {
					t.Fatalf("%s at %v is %v, and %v cycles on %v", name, c, v, k, w)
				}
			}
		}
	}
	// The random shapes pick a value a cycle, the same each time.
	for _, shape := range []int{SampleHold, SmoothRandom} {
		s := withShape(shape, 3)
		again := withShape(shape, 3)
		if s.shapeAt(4) == s.shapeAt(5) || s.shapeAt(4.0) != again.shapeAt(4.0) {
			t.Errorf("%s: cycles 4 and 5 are alike, or cycle 4 differs from itself", Shapes[shape])
		}
	}
	for _, c := range []struct {
		shape int
		at    float64
		want  float64
	}{
		{SawDown, 0, 1}, {SawDown, 0.5, 0}, {SawDown, 0.75, -0.5},
		{ExpRise, 0, -1}, {ExpRise, 1 - 1e-12, 1}, {ExpFall, 0, 1}, {ExpFall, 1 - 1e-12, -1},
	} {
		s := withShape(c.shape, 0)
		if got := s.shapeAt(c.at); !near(got, c.want, 1e-9) {
			t.Errorf("%s at %v is %v, want %v", Shapes[c.shape], c.at, got, c.want)
		}
	}
	// The exponential shapes bend: half way, the rise is still low, and
	// the fall low already.
	r, f := withShape(ExpRise, 0), withShape(ExpFall, 0)
	if rise, fall := r.shapeAt(0.5), f.shapeAt(0.5); rise > -0.5 || fall > -0.5 {
		t.Errorf("half way, the rise is at %v and the fall at %v, want both under -0.5", rise, fall)
	}
}

func TestTheSmoothingIsTheWavesAverage(t *testing.T) {
	st := DefaultState()
	st.Curve = append(st.Curve[:1], Point{0.1, 1}, Point{0.1, -0.5}, Point{0.7, 0.4}) // a jump at 0.1
	pat := compile(st)
	for shape, name := range Shapes {
		s := withShape(shape, 11)
		s.pat, s.jitter, s.glide = pat, 0.4, 0.3
		for _, smooth := range []float64{0.05, 0.3, 1} {
			s.smooth = smooth
			w := smooth * maxSmooth
			raw := s
			raw.smooth = 0
			for _, c := range []float64{0, 0.13, 0.5, 0.97, 3.02, 41.6} {
				// The average of the wave, at many points across the
				// window.
				const n = 20000
				sum := 0.0
				for i := range n {
					sum += raw.shapeAt(c-w/2+w*(float64(i)+0.5)/n) + raw.jitterAt(c-w/2+w*(float64(i)+0.5)/n)
				}
				want := min(max(sum/n, -1), 1)
				if got := s.wave(c); !near(got, want, 2e-3) {
					t.Fatalf("%s smoothed %v at %v is %v, want the average %v", name, smooth, c, got, want)
				}
			}
		}
	}
}

func TestTheStepsPlayInTurn(t *testing.T) {
	st := DefaultState()
	st.Steps = append([]float64{1, -1, 0.5, 0}, make([]float64, MaxSteps-4)...)
	st.Count = 4
	s := withShape(Steps, 0)
	s.pat = compile(st)
	for _, c := range []struct{ at, glide, want float64 }{
		{0.1, 0, 1}, {0.3, 0, -1}, {0.6, 0, 0.5}, {0.9, 0, 0}, {1.1, 0, 1},
		// Half a step's glide: from the step before, half way over the
		// first quarter of the step, and there from the second half.
		{0.25, 0.5, 1}, {0.25 + 0.125/2, 0.5, 0}, {0.25 + 0.125, 0.5, -1}, {0, 0.5, 0}, {0.0625, 0.5, 0.5},
	} {
		s.glide = c.glide
		if got := s.wave(c.at); !near(got, c.want, 1e-9) {
			t.Errorf("steps at %v, glide %v: %v, want %v", c.at, c.glide, got, c.want)
		}
	}
	// The state reaches the sound as it is set, from the next block.
	p := newPlugin()
	if err := p.SetState(st.Encode()); err != nil {
		t.Fatal(err)
	}
	pr := &p.proc
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 4})
	bl := newBlock(4, plug.Position{Frame: rate / 4}) // 0.25 s at 1 Hz: the second step
	bl.b.Changes = changes(float64(ShapeID), Steps, float64(SyncID), 0, float64(RateID), 1)
	pr.Process(&bl.b)
	if got := bl.outs[MainOut][0]; got != -1 {
		t.Errorf("the second step plays %v, want -1", got)
	}
	// Synced to a bar, from the song: two and a half beats in is the
	// third step.
	bl = newBlock(4, plug.Position{Frame: 60000, Playing: true, Tempo: 120, Beat: 2.5})
	bl.b.Changes = changes(float64(SyncID), 1, float64(NoteID), 3, float64(StartID), FromSong)
	pr.Process(&bl.b)
	if got := bl.outs[MainOut][0]; got != 0.5 {
		t.Errorf("at beat 2.5, the steps play %v, want the third, 0.5", got)
	}
}

func TestTheShaperDrawsItsCurve(t *testing.T) {
	st := DefaultState()
	st.Curve = []Point{{0.2, 1}, {0.6, -1}}
	s := withShape(Shaper, 0)
	s.pat = compile(st)
	for _, c := range []struct{ at, want float64 }{
		{0.2, 1}, {0.4, 0}, {0.6, -1},
		// From the last point round to the first, a cycle on.
		{0.9, 0}, {0, 1.0 / 3}, {1.2, 1},
	} {
		if got := s.wave(c.at); !near(got, c.want, 1e-9) {
			t.Errorf("the curve at %v is %v, want %v", c.at, got, c.want)
		}
	}
	// Two points at one place jump; one point holds still.
	st.Curve = []Point{{0, -1}, {0.5, -1}, {0.5, 1}, {1, 1}}
	s.pat = compile(st)
	if a, b := s.wave(0.4999), s.wave(0.5); a != -1 || b != 1 {
		t.Errorf("the jump runs %v to %v, want -1 to 1", a, b)
	}
	st.Curve = []Point{{0.3, 0.25}}
	s.pat = compile(st)
	if a, b := s.wave(0), s.wave(0.8); a != 0.25 || b != 0.25 {
		t.Errorf("a curve of one point runs %v to %v, want 0.25", a, b)
	}
}

func TestThePulseIsHighForItsWidth(t *testing.T) {
	s := withShape(Pulse, 0)
	if s.width != 0.25 {
		t.Errorf("a new pulse is %v wide, want 0.25", s.width)
	}
	for _, c := range []struct{ width, at, want float64 }{
		{0.25, 0, 1}, {0.25, 0.2, 1}, {0.25, 0.25, -1}, {0.25, 0.9, -1}, {0.25, 1.1, 1},
		{0.5, 0.4, 1}, {0.5, 0.6, -1},
		{0.1, 0.05, 1}, {0.1, 0.15, -1},
		// At none, it is low all the cycle, and at all of it, high.
		{0, 0, -1}, {0, 0.5, -1}, {1, 0, 1}, {1, 0.9999, 1},
	} {
		s.width = c.width
		if got := s.wave(c.at); got != c.want {
			t.Errorf("a pulse %v wide at %v is %v, want %v", c.width, c.at, got, c.want)
		}
	}
	// Unipolar, it runs from 0 to the depth, as the square does.
	s.width, s.unipolar, s.depth = 0.25, true, 0.5
	if a, b := s.wave(0.1), s.wave(0.6); a != 0.5 || b != 0 {
		t.Errorf("a unipolar pulse at half depth runs %v to %v, want 0.5 to 0", a, b)
	}
	// Smoothed, it is its average: over a whole cycle, high for the
	// width and low for the rest.
	s.unipolar, s.depth = false, 1
	for _, w := range []float64{0.1, 0.25, 0.8} {
		s.width = w
		if got, want := s.area(0.3, 1.3), 2*w-1; !near(got, want, 1e-9) {
			t.Errorf("a pulse %v wide has %v under a cycle, want %v", w, got, want)
		}
	}
}

func TestTheStateRoundTrips(t *testing.T) {
	st := DefaultState()
	st.Steps[3], st.Count = -0.125, 12
	st.Curve = []Point{{0, 0}, {0.25, 1}, {0.25, -0.5}, {1, 0.75}}
	got, err := DecodeState(st.Encode())
	if err != nil || !got.Equal(st) {
		t.Fatalf("the state reads back as %+v, %v, want %+v", got, err, st)
	}
	p := newPlugin()
	if b := p.State(); b != nil {
		t.Errorf("a new LFO's state is %s, want none", b)
	}
	if err = p.SetState(st.Encode()); err != nil {
		t.Fatal(err)
	}
	if b := p.State(); !bytes.Equal(b, st.Encode()) {
		t.Errorf("the LFO keeps %s, want %s", b, st.Encode())
	}
	// A state it cannot read changes nothing.
	for _, bad := range []string{`{"Steps":[2]}`, `{"Count":17}`, `{"Curve":[{"X":0.5},{"X":0.25}]}`,
		`{"Curve":[{"X":1.5}]}`, `{"Steps":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]}`, `[`} {
		if p.SetState([]byte(bad)) == nil {
			t.Errorf("the LFO takes %s", bad)
		}
	}
	if !p.current().Equal(st) {
		t.Error("a state the LFO cannot read changes it")
	}
	// What a state leaves out is a new LFO's.
	part, err := DecodeState([]byte(`{"Count":3}`))
	if err != nil || part.Count != 3 || !part.withCount(8).Equal(DefaultState()) {
		t.Errorf("a state of a count alone reads as %+v, %v", part, err)
	}
	if err := p.SetState(nil); err != nil || p.State() != nil {
		t.Errorf("an empty state leaves %s, %v", p.State(), err)
	}
}

func TestTheEditsKeepTheCurveInOrder(t *testing.T) {
	st := DefaultState() // points at 0, 0.1, 0.4, 1
	st2, i := st.withPoint(Point{0.25, 2})
	if i != 2 || st2.Curve[2] != (Point{0.25, 1}) || len(st.Curve) != 4 {
		t.Errorf("a point added at 0.25 is %d, %v, of the curve %v", i, st2.Curve, st.Curve)
	}
	if moved := st2.movingPoint(2, Point{0.9, -0.5}); moved.Curve[2] != (Point{0.4, -0.5}) {
		t.Errorf("a point dragged past the next stops at %v, want at 0.4", moved.Curve[2])
	}
	if gone := st2.withoutPoint(2); !gone.Equal(st) {
		t.Errorf("taking the point away leaves %v", gone.Curve)
	}
	one := State{Curve: []Point{{0.5, 0}}}
	if one.withoutPoint(0).Curve == nil {
		t.Error("the curve's last point is taken away")
	}
	full := st
	for len(full.Curve) < MaxPoints {
		full, _ = full.withPoint(Point{0.5, 0})
	}
	if _, i := full.withPoint(Point{0.5, 0}); i != -1 {
		t.Error("a full curve takes another point")
	}
}

func TestTheScopeEditsTheStepsAndTheCurve(t *testing.T) {
	h := newHost()
	p := newPlugin()
	pn := newPanel(h, p, true)
	w := gunimtest.New(t, geom.Sz(900, 420), nil)
	gunim.RegisterView(w, "lfo", func(struct{}) gunim.Node { return pn }, nil)
	if err := w.Client().Mount(gunim.Root, "lfo", "lfo", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	a := pn.sc.area
	if a.Size().W < 100 || a.Size().H < 40 {
		t.Fatalf("the scope's cycle is %v", a)
	}
	at := func(x, v float64) geom.Point {
		return geom.Pt(a.Min.X+a.Size().W*float32(x), a.Max.Y-a.Size().H*float32((v+1)/2))
	}
	// A sine edits nothing.
	if pn.Handle(input.PointerDown{Pos: at(0.5, 0.5)}, nil) {
		t.Error("the scope of a sine takes a press")
	}
	// Steps: a drag across draws the steps it passes, as one edit.
	h.values[ShapeID] = Steps
	pn.FollowState(nil)
	pn.Handle(input.PointerDown{Pos: at(2.5/8, 0.5)}, nil)
	pn.Handle(input.PointerMove{Pos: at(5.5/8, -1)}, nil)
	pn.Handle(input.PointerUp{Pos: at(5.5/8, -1)}, nil)
	st := p.current()
	if !near(st.Steps[2], 0.5, 0.01) || st.Steps[3] != -1 || st.Steps[5] != -1 || st.Steps[6] != DefaultState().Steps[6] {
		t.Errorf("the drag leaves the steps at %v", st.Steps[:8])
	}
	if h.begun != 1 || h.ended != 1 || h.change != 2 {
		t.Errorf("the drag makes %d edits, ends %d, and tells of %d changes", h.begun, h.ended, h.change)
	}
	pn.count.OnPick(3, nil)
	if p.current().Count != 4 || pn.count.Selected() != 3 {
		t.Errorf("picking 4 steps plays %d", p.current().Count)
	}
	// Shaper: a press adds a point, and drags it; a right click takes
	// it away.
	h.values[ShapeID] = Shaper
	pn.FollowState(nil)
	pn.Handle(input.PointerDown{Pos: at(0.7, 0.5)}, nil)
	pn.Handle(input.PointerMove{Pos: at(0.75, -0.25)}, nil)
	pn.Handle(input.PointerUp{Pos: at(0.75, -0.25)}, nil)
	c := p.current().Curve
	if len(c) != 5 || !near(c[3].X, 0.75, 0.01) || !near(c[3].Y, -0.25, 0.01) {
		t.Errorf("the press and drag leave the curve %v", c)
	}
	pn.Handle(input.PointerDown{Pos: at(0.75, -0.25), Button: input.ButtonSecondary}, nil)
	if c := p.current().Curve; !slices.Equal(c, DefaultState().Curve) {
		t.Errorf("the right click leaves the curve %v", c)
	}
	if want := []string{"Draw steps", "Steps", "Add point", "Delete point"}; len(h.names) != len(want) ||
		h.names[0] != want[0] || h.names[2] != want[2] || h.names[3] != want[3] || h.begun != h.ended {
		t.Errorf("the edits are %q, %d begun and %d ended", h.names, h.begun, h.ended)
	}
	w.Frame(time.Second / 60)
}
