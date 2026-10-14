package plugmod

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"slices"
)

// State is the LFO's state, as [plug.Plugin]'s State writes it: the
// steps the Steps shape plays and the curve the Shaper shape plays. It is
// JSON:
//
//	{"Steps":[1,-0.5,0.5,...],"Count":8,"Curve":[{"Y":-1},{"X":0.1,"Y":1}]}
//
// The rest of the LFO's settings are parameters, beside the state.
type State struct {
	// Steps are the steps' values, from -1 to 1, MaxSteps of them, of
	// which the first Count play, each an equal part of the cycle. The
	// rest keep their values for a longer pattern again.
	Steps []float64
	Count int
	// Curve is the curve's points, from 1 to MaxPoints of them, in
	// order across the cycle: the curve runs straight from each to the
	// next, and from the last round to the first. Two at one place make
	// a jump.
	Curve []Point
}

// Point is a point of the Shaper's curve: X across the cycle, from 0 to
// 1, and Y its value, from -1 to 1.
type Point struct{ X, Y float64 }

// How many steps and points a state holds at most.
const (
	MaxSteps  = 16
	MaxPoints = 32
)

// DefaultState returns a new LFO's state: eight steps of a falling and
// rising pattern, and a curve that rises fast and falls slowly, as an
// envelope.
func DefaultState() State {
	return State{
		Steps: []float64{1, -0.5, 0.5, -1, 0.75, 0, 0.25, -0.75, 1, 0.5, 0, -0.5, -1, -0.5, 0, 0.5},
		Count: 8,
		Curve: []Point{{0, -1}, {0.1, 1}, {0.4, 0.2}, {1, -1}},
	}
}

// Clone returns a copy of s that shares nothing with it.
func (s State) Clone() State {
	return State{Steps: slices.Clone(s.Steps), Count: s.Count, Curve: slices.Clone(s.Curve)}
}

// Equal reports whether s and t are the same.
func (s State) Equal(t State) bool {
	return s.Count == t.Count && slices.Equal(s.Steps, t.Steps) && slices.Equal(s.Curve, t.Curve)
}

// wire is how the state is written: Go's field names as keys, zero
// values left out, so a state writes the same twice.
var wire = json.JoinOptions(json.OmitZeroStructFields(true), json.Deterministic(true))

// Encode returns the state as JSON, as State writes it.
func (s State) Encode() []byte {
	b, err := json.Marshal(s, wire)
	if err != nil {
		// Numbers always marshal.
		panic("plugmod: " + err.Error())
	}
	return b
}

// DecodeState reads a state as Encode writes it, and checks it. An empty
// state is [DefaultState]; so are the steps, the count or the curve a
// state leaves out.
func DecodeState(b []byte) (State, error) {
	def := DefaultState()
	if len(b) == 0 {
		return def, nil
	}
	var s State
	if err := json.Unmarshal(b, &s, wire); err != nil {
		return State{}, fmt.Errorf("plugmod: reading the LFO's state: %w", err)
	}
	if len(s.Steps) == 0 {
		s.Steps = def.Steps
	}
	if len(s.Steps) > MaxSteps {
		return State{}, fmt.Errorf("plugmod: the LFO's state holds %d steps, of %d at most", len(s.Steps), MaxSteps)
	}
	for _, v := range s.Steps {
		if !(v >= -1 && v <= 1) {
			return State{}, fmt.Errorf("plugmod: the LFO's state has a step at %v, outside -1 to 1", v)
		}
	}
	for len(s.Steps) < MaxSteps {
		s.Steps = append(s.Steps, 0)
	}
	if s.Count == 0 {
		s.Count = def.Count
	}
	if s.Count < 1 || s.Count > MaxSteps {
		return State{}, fmt.Errorf("plugmod: the LFO's state plays %d steps, of 1 to %d", s.Count, MaxSteps)
	}
	if len(s.Curve) == 0 {
		s.Curve = def.Curve
	}
	if len(s.Curve) > MaxPoints {
		return State{}, fmt.Errorf("plugmod: the LFO's curve has %d points, of %d at most", len(s.Curve), MaxPoints)
	}
	for i, p := range s.Curve {
		if !(p.X >= 0 && p.X <= 1 && p.Y >= -1 && p.Y <= 1) || i > 0 && p.X < s.Curve[i-1].X {
			return State{}, fmt.Errorf("plugmod: the LFO's curve has point %d at %v, out of place", i, p)
		}
	}
	return s, nil
}

// withStep returns s with step i at v, within -1 to 1.
func (s State) withStep(i int, v float64) State {
	t := s.Clone()
	if i >= 0 && i < len(t.Steps) {
		t.Steps[i] = min(max(v, -1), 1)
	}
	return t
}

// withCount returns s playing n steps, within 1 to MaxSteps.
func (s State) withCount(n int) State {
	t := s.Clone()
	t.Count = min(max(n, 1), MaxSteps)
	return t
}

// withPoint returns s with a point at p, after any at its X, and its
// index: or s as it is, and -1, where the curve has all its points.
func (s State) withPoint(p Point) (t State, at int) {
	if len(s.Curve) >= MaxPoints {
		return s, -1
	}
	p = Point{min(max(p.X, 0), 1), min(max(p.Y, -1), 1)}
	t = s.Clone()
	at = len(t.Curve)
	for j, q := range t.Curve {
		if q.X > p.X {
			at = j
			break
		}
	}
	t.Curve = slices.Insert(t.Curve, at, p)
	return t, at
}

// movingPoint returns s with point i moved to p, kept between the
// points either side of it, and within the cycle and -1 to 1.
func (s State) movingPoint(i int, p Point) State {
	if i < 0 || i >= len(s.Curve) {
		return s
	}
	lo, hi := 0.0, 1.0
	if i > 0 {
		lo = s.Curve[i-1].X
	}
	if i+1 < len(s.Curve) {
		hi = s.Curve[i+1].X
	}
	t := s.Clone()
	t.Curve[i] = Point{min(max(p.X, lo), hi), min(max(p.Y, -1), 1)}
	return t
}

// withoutPoint returns s without point i: or s as it is, where i is its
// last point.
func (s State) withoutPoint(i int) State {
	if i < 0 || i >= len(s.Curve) || len(s.Curve) <= 1 {
		return s
	}
	t := s.Clone()
	t.Curve = slices.Delete(t.Curve, i, i+1)
	return t
}

// pattern is a state as the sound plays it: never changed once made, so
// the audio thread reads it as the app makes the next.
type pattern struct {
	steps [MaxSteps]float64
	count int
	curve []Point
}

// defaultPattern is a new LFO's pattern.
var defaultPattern = compile(DefaultState())

// compile returns the pattern of s, which DecodeState has checked.
func compile(s State) *pattern {
	p := &pattern{count: min(max(s.Count, 1), MaxSteps), curve: slices.Clone(s.Curve)}
	copy(p.steps[:], s.Steps)
	if len(p.curve) == 0 {
		p.curve = []Point{{0, 0}}
	}
	return p
}

// stepOf returns the step at x, from 0 to 1 through the cycle, and how
// far through it x is, from 0 to 1.
func (p *pattern) stepOf(x float64) (i int, t float64) {
	n := float64(p.count)
	i = min(max(int(math.Floor(x*n)), 0), p.count-1)
	return i, x*n - float64(i)
}

// step returns the steps' value at x, from 0 to 1 through the cycle,
// each step sliding from the one before over glide of its length.
func (p *pattern) step(x, glide float64) float64 {
	i, t := p.stepOf(x)
	cur := p.steps[i]
	if t >= glide {
		return cur
	}
	prev := p.steps[(i+p.count-1)%p.count]
	return prev + (cur-prev)*t/glide
}

// stepArea returns the area under step i, from its start to t through
// it, in steps.
func (p *pattern) stepArea(i int, t, glide float64) float64 {
	cur := p.steps[i]
	a := cur * max(t-glide, 0)
	if glide > 0 {
		prev := p.steps[(i+p.count-1)%p.count]
		g := min(t, glide)
		a += prev*g + (cur-prev)*g*g/(2*glide)
	}
	return a
}

// stepsIntegral returns the area under the steps from the cycle's start
// to x, from 0 to 1.
func (p *pattern) stepsIntegral(x, glide float64) float64 {
	i, t := p.stepOf(x)
	a := p.stepArea(i, t, glide)
	for j := range i {
		a += p.stepArea(j, 1, glide)
	}
	return a / float64(p.count)
}

// segment returns the ends of the curve's line j, from 0 to the count
// of its points: the first from the last point, a cycle back, to the
// first, and the last from the last point to the first, a cycle on.
func (p *pattern) segment(j int) (a, b Point) {
	at := func(k int) Point {
		switch {
		case k < 0:
			q := p.curve[len(p.curve)-1]
			return Point{q.X - 1, q.Y}
		case k >= len(p.curve):
			q := p.curve[0]
			return Point{q.X + 1, q.Y}
		}
		return p.curve[k]
	}
	return at(j - 1), at(j)
}

// lerp returns the value at x of the line from a to b.
func lerp(a, b Point, x float64) float64 {
	return a.Y + (b.Y-a.Y)*(x-a.X)/(b.X-a.X)
}

// curveAt returns the curve's value at x, from 0 to 1 through the cycle.
func (p *pattern) curveAt(x float64) float64 {
	for j := range len(p.curve) + 1 {
		if a, b := p.segment(j); x >= a.X && x < b.X {
			return lerp(a, b, x)
		}
	}
	return p.curve[0].Y
}

// curveIntegral returns the area under the curve from the cycle's start
// to x, from 0 to 1.
func (p *pattern) curveIntegral(x float64) float64 {
	sum := 0.0
	for j := range len(p.curve) + 1 {
		a, b := p.segment(j)
		if u0, u1 := max(a.X, 0), min(b.X, x); u1 > u0 {
			sum += (u1 - u0) * (lerp(a, b, u0) + lerp(a, b, u1)) / 2
		}
	}
	return sum
}
