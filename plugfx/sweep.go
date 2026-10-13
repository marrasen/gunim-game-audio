package plugfx

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A sweep follows the sound through an effect from each hit on, for a
// display to draw it along the display's time: the level going in and
// coming out, left and right, in steps of sweepStep frames, from the
// frame the hit came in. A display drawing the echoes or the decay its
// settings make draws the sweep over them, so the sound lands on them
// as it plays.
//
// A sweep starts at a hit: the sound going in jumps sweepJump dB over
// where it was. It runs for the display's span, and the next hit after
// that starts the next sweep; a sound going in that holds on starts
// the next at once. Once the sound has been quiet for sweepRest, the
// last sweep fades away, and the settings' shape shows alone.
//
// It reads the host's [plug.Sound]s a frame of the window at a time, on
// the window's goroutine, as the meters do. It allocates as it is made
// and never after.
type sweep struct {
	in, out plug.Listener
	// pend gathers each step's levels as frames come, by step number,
	// until both sounds have passed it; next is the next step to take.
	pend [sweepPend]sweepLevels
	next int64
	// ref is the level going in a hit must jump over, in dB, rising
	// back to the level as it falls quiet.
	ref float64
	// start is the step the sweep started on, -1 for none; steps holds
	// its levels from then, n of them; rate is the sound's rate.
	start int64
	steps []sweepLevels
	n     int
	rate  int
	// top is the loudest level going in over the sweep so far, which
	// the display draws at 0 dB, so the sound coming out shows as loud
	// as it is beside the hit; or coming out, where the host keeps
	// nothing going in.
	top float32
	// quiet is how long the sound has been quiet, up to sweepRest, and
	// fade how much the sweep shows.
	quiet time.Duration
	fade  *anim.Float
}

// sweepLevels are a step's levels: the peak going in, and the peaks
// coming out, left and right, as amplitudes. tag is the step's number
// plus one, in a step gathering.
type sweepLevels struct {
	tag           int64
	in, out, out2 float32
}

// The sweep's steps: how many frames each is, how many it gathers at
// once, and the most one sweep keeps, over 40 seconds at 48 kHz.
const (
	sweepShift = 8
	sweepStep  = 1 << sweepShift
	sweepPend  = 64
	sweepSteps = 1 << 13
)

// sweepJump is how far over its level the sound going in jumps at a
// hit, in dB, and sweepFloor the least a hit reaches. sweepRise is how
// fast the level a hit jumps over rises back to the level, in dB a
// second. sweepRest is how long the sound is quiet before a sweep
// fades away.
const (
	sweepJump  = 12
	sweepFloor = -50
	sweepRise  = 40
	sweepRest  = 1500 * time.Millisecond
)

func newSweep(h plug.Host) *sweep {
	in, out := h.Sound()
	s := &sweep{start: -1, ref: sweepFloor, steps: make([]sweepLevels, sweepSteps), fade: anim.NewFloat(0)}
	s.in.Sound, s.out.Sound = in, out
	return s
}

// dB returns an amplitude in decibels, -120 for silence.
func dB(a float32) float64 {
	if a <= 1e-6 {
		return -120
	}
	return 20 * math.Log10(float64(a))
}

// step takes the frames heard over dt, and reports whether the sweep
// still moves. span is how long a sweep runs, in seconds.
func (s *sweep) step(dt time.Duration, span float64) bool {
	frontier := int64(-1)
	for i, l := range []*plug.Listener{&s.in, &s.out} {
		if l.Sound == nil {
			continue
		}
		frames, rate := l.Listen(sweepPend * sweepStep)
		if rate > 0 {
			s.rate = rate
		}
		at := l.At()
		s.gather(frames, at, i == 1)
		if at > 0 && (frontier < 0 || at < frontier) {
			frontier = at
		}
	}
	loud := false
	if frontier > 0 {
		done := frontier >> sweepShift
		s.next = max(s.next, done-sweepPend+1)
		for ; s.next < done; s.next++ {
			lv := s.pend[s.next%sweepPend]
			if lv.tag != s.next+1 {
				lv = sweepLevels{}
			}
			loud = s.take(s.next, lv, span) || loud
		}
	}
	if loud {
		s.quiet = 0
	} else {
		s.quiet = min(s.quiet+dt, sweepRest)
	}
	switch {
	case s.n == 0:
	case s.quiet < sweepRest:
		s.fade.Jump(1)
	case s.fade.Target() != 0:
		s.fade.Animate(0, anim.Spring{Response: 0.8, Damping: 1})
	}
	moving := s.fade.Step(dt)
	return moving || s.quiet < sweepRest && s.n > 0
}

// gather adds frames, which end at frame at, to the steps gathering:
// to the level going in, or coming out where out says so.
func (s *sweep) gather(frames []float32, at int64, out bool) {
	n := int64(len(frames) / 2)
	for i := range n {
		f := at - n + i
		k := f >> sweepShift
		lv := &s.pend[k%sweepPend]
		if lv.tag != k+1 {
			*lv = sweepLevels{tag: k + 1}
		}
		l, r := abs32(frames[2*i]), abs32(frames[2*i+1])
		if out {
			lv.out, lv.out2 = max(lv.out, l), max(lv.out2, r)
		} else {
			lv.in = max(lv.in, l, r)
		}
	}
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

// take takes step k's levels, lv: a hit starts a sweep, and a sweep
// running keeps them. It reports whether the step was loud.
func (s *sweep) take(k int64, lv sweepLevels, span float64) bool {
	in := lv.in
	if s.in.Sound == nil || s.in.At() == 0 {
		// The host keeps no sound going in: the hits are heard coming
		// out.
		in = max(lv.out, lv.out2)
	}
	level := dB(in)
	secs := float64(sweepStep) / float64(max(s.rate, 1))
	hit := level > sweepFloor && level-s.ref >= sweepJump
	if hit || level < s.ref {
		s.ref = level
	} else {
		s.ref = min(level, s.ref+sweepRise*secs)
	}
	over := s.start < 0 || float64(k-s.start)*secs >= span || s.n == len(s.steps)
	if over && (hit || s.start >= 0 && level > sweepFloor) {
		s.start, s.n, s.top = k, 0, 0
	}
	if s.start >= 0 && (!over || s.start == k) {
		s.steps[s.n] = lv
		s.n++
		if s.in.Sound != nil && s.in.At() != 0 {
			s.top = max(s.top, lv.in)
		} else {
			s.top = max(s.top, lv.out, lv.out2)
		}
	}
	return dB(max(lv.in, lv.out, lv.out2)) > sweepFloor
}

// shown reports whether there is a sweep to draw, and how much it
// shows.
func (s *sweep) shown() (float32, bool) {
	f := s.fade.Value()
	return f, s.n > 0 && f > 0.01
}

// levels returns the sweep's loudest levels from t0 to t1 seconds after
// it started, in dB from its loudest: going in, coming out left, and
// coming out right; and whether the sweep has reached t0.
func (s *sweep) levels(t0, t1 float64) (in, l, r float64, ok bool) {
	per := float64(max(s.rate, 1)) / sweepStep
	from, to := int(t0*per), int(math.Ceil(t1*per))
	if from >= s.n || from < 0 {
		return 0, 0, 0, false
	}
	var lv sweepLevels
	for i := from; i < max(min(to, s.n), from+1); i++ {
		x := s.steps[i]
		lv.in, lv.out, lv.out2 = max(lv.in, x.in), max(lv.out, x.out), max(lv.out2, x.out2)
	}
	top := dB(max(s.top, 1e-6))
	return dB(lv.in) - top, dB(lv.out) - top, dB(lv.out2) - top, true
}

// length is how long the sweep has run, in seconds.
func (s *sweep) length() float64 {
	return float64(s.n) * sweepStep / float64(max(s.rate, 1))
}

// paint draws the sweep across r, a column at a time: at each x, at
// the seconds secsAt gives it, column draws the levels there, in dB
// from the loudest. Then a line marks where the sweep has reached,
// while it runs, at xOf of its length.
func (s *sweep) paint(p *paint.Painter, r geom.Rect, secsAt func(x float32) float64, xOf func(secs float64) float32,
	col color.NRGBA, column func(x float32, in, l, rt float64)) {
	if _, ok := s.shown(); !ok {
		return
	}
	const w = 2
	for x := r.Min.X; x < r.Max.X; x += w {
		in, l, rt, ok := s.levels(secsAt(x), secsAt(x+w))
		if !ok {
			break
		}
		column(x, in, l, rt)
	}
	if s.quiet < plug.StillAfter {
		x := xOf(s.length())
		if x >= r.Min.X && x <= r.Max.X {
			fade, _ := s.shown()
			p.RRect(geom.Rc(x-0.5, r.Min.Y, 1, r.Size().H), 0, paint.Solid(audioui.Faded(col, 0.35*fade)))
		}
	}
}
