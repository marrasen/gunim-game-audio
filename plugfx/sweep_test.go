package plugfx

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// recorded is a sound for tests: frames, interleaved, heard up to the
// frame *at, and silence after them.
type recorded struct {
	frames []float32
	at     *int64
}

// Heard implements [plug.Sound].
func (s recorded) Heard(dst []float32) (frames int, at int64, rate int) {
	at = *s.at
	if at == 0 {
		return 0, 0, 0
	}
	n := min(int64(len(dst)/2), at)
	for i := range n {
		f := at - n + i
		dst[2*i], dst[2*i+1] = 0, 0
		if 2*f+1 < int64(len(s.frames)) {
			dst[2*i], dst[2*i+1] = s.frames[2*f], s.frames[2*f+1]
		}
	}
	return int(n), at, synth.Rate
}

// soundHost is a host for tests that keeps the sound going into an
// effect and coming out, heard up to the frame at.
type soundHost struct {
	*host
	in, out []float32
	at      int64
}

// Sound implements [plug.Host].
func (h *soundHost) Sound() (in, out plug.Sound) {
	return recorded{frames: h.in, at: &h.at}, recorded{frames: h.out, at: &h.at}
}

// Param implements [plug.Host]: of a ReadOnly parameter, what the
// processor reports.
func (h *soundHost) Param(id uint32) float64 { return h.values[id] }

// interleave returns two channels as one slice, frame by frame.
func interleave(ch [][]float32) []float32 {
	out := make([]float32, 2*len(ch[0]))
	for i := range ch[0] {
		out[2*i], out[2*i+1] = ch[0][i], ch[1][i]
	}
	return out
}

// burst is 1 kHz tones at half the full level, ms long, from each frame in at, in
// frames of silence.
func burst(frames, ms int, at ...int) [][]float32 {
	in := [][]float32{make([]float32, frames), make([]float32, frames)}
	for _, from := range at {
		for i := range ms * synth.Rate / 1000 {
			v := float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/synth.Rate))
			in[0][from+i], in[1][from+i] = v, v
		}
	}
	return in
}

// playThrough runs in through an effect d, with changes, and returns a
// host that hears it.
func playThrough(t *testing.T, d *plug.Descriptor, in [][]float32, changes ...plug.Change) *soundHost {
	t.Helper()
	p := d.New(nil)
	out := rig{pr: p.Processor(), d: d, s: setup, tempo: func(int) float64 { return 120 }}.run(t, in, changes...)
	h := &soundHost{host: newHost(d), in: interleave(in), out: interleave(out)}
	for _, c := range changes {
		h.values[c.ID] = c.Value
	}
	return h
}

// frameStep is how far the sound moves on in a frame of the window.
const frameStep = synth.Rate / 60

func TestASweepFollowsTheEchoesFromTheHit(t *testing.T) {
	const hitAt = 4800
	in := burst(5*synth.Rate, 30, hitAt, 3*synth.Rate)
	h := playThrough(t, &DelayDescriptor, in, plug.Change{ID: SyncID, Value: 0}, plug.Change{ID: TimeID, Value: 300},
		plug.Change{ID: FeedbackID, Value: 50}, plug.Change{ID: PingPongID, Value: 0})
	sw := newSweep(h)
	const span = 2.0
	// Before the hit, the sweep waits.
	for h.at = frameStep; h.at < hitAt; h.at += frameStep {
		sw.step(time.Second/60, span)
	}
	if sw.n != 0 {
		t.Fatalf("silence starts a sweep of %d steps", sw.n)
	}
	// From the hit, it grows a frame's sound a frame, never back, and
	// starts at the hit.
	last := 0.0
	for ; h.at < hitAt+int64(1.5*synth.Rate); h.at += frameStep {
		if !sw.step(time.Second/60, span) && sw.n > 0 {
			t.Fatal("the sweep stands still as the echoes sound")
		}
		got := sw.length()
		if got < last || got-last > 2*float64(frameStep)/synth.Rate {
			t.Fatalf("the sweep moves from %.3f s to %.3f s in a frame", last, got)
		}
		last = got
	}
	if start := sw.start * sweepStep; start > hitAt || hitAt-start >= sweepStep {
		t.Fatalf("the sweep starts at frame %d, not on the hit at %d", start, hitAt)
	}
	off := float64(hitAt-sw.start*sweepStep) / synth.Rate
	if in, _, _, _ := sw.levels(off, off+0.02); in < -1 {
		t.Errorf("the hit reads %.1f dB going in, not the loudest", in)
	}
	// Each echo, 300 ms on from the one before, half as loud: about 6
	// dB down, the filters taking a little; and quiet between.
	for k := 1; k <= 3; k++ {
		at := off + 0.3*float64(k)
		_, l, r, ok := sw.levels(at, at+0.03)
		want := -6.0 * float64(k-1)
		if !ok || math.Abs(l-want) > 3 || math.Abs(r-want) > 3 {
			t.Errorf("echo %d reads %.1f and %.1f dB, not about %.0f", k, l, r, want)
		}
		if _, l, _, _ := sw.levels(at+0.1, at+0.2); l > want-24 {
			t.Errorf("between echo %d and the next reads %.1f dB", k, l)
		}
	}
}

func TestASweepRunsItsSpanThenWaitsForTheNextHit(t *testing.T) {
	// Hits at 0.1 s, 0.6 s, inside the first sweep, and 3 s, after it.
	in := burst(5*synth.Rate, 30, 4800, 28800, 3*synth.Rate)
	h := playThrough(t, &DelayDescriptor, in, plug.Change{ID: SyncID, Value: 0}, plug.Change{ID: TimeID, Value: 300},
		plug.Change{ID: FeedbackID, Value: 30})
	sw := newSweep(h)
	var starts []int64
	for h.at = frameStep; h.at < 4*synth.Rate; h.at += frameStep {
		sw.step(time.Second/60, 2)
		if len(starts) == 0 || starts[len(starts)-1] != sw.start {
			starts = append(starts, sw.start)
		}
	}
	if len(starts) != 3 || starts[1] != 4800/sweepStep || starts[2] != 3*synth.Rate/sweepStep {
		t.Errorf("the sweeps start at steps %v, not on the first and the last hit", starts)
	}
	// Quiet, the sweep fades away and the display rests.
	still := 0
	for range 4 * 60 {
		h.at += frameStep
		if !sw.step(time.Second/60, 2) {
			still++
		}
	}
	if f, shown := sw.shown(); shown || still == 0 {
		t.Errorf("four seconds after the sound ends, the sweep shows at %.2f, resting %d frames", f, still)
	}
}

func TestASweepFollowsTheReverbsDecay(t *testing.T) {
	in := burst(4*synth.Rate, 40, 4800)
	h := playThrough(t, &ReverbDescriptor, in, plug.Change{ID: DecayID, Value: 1}, plug.Change{ID: PreDelayID, Value: 20})
	sw := newSweep(h)
	for h.at = frameStep; h.at < 3*synth.Rate; h.at += frameStep {
		sw.step(time.Second/60, 2)
	}
	// The tail falls about 60 dB a second.
	_, l1, r1, ok1 := sw.levels(0.3, 0.35)
	_, l2, r2, ok2 := sw.levels(0.8, 0.85)
	fall := max(l1, r1) - max(l2, r2)
	if !ok1 || !ok2 || fall < 20 || fall > 40 {
		t.Errorf("the tail falls %.1f dB from 0.3 s to 0.8 s, not about 30", fall)
	}
}

func TestASweepTakesTheHitsFromWhatComesOutWhereTheHostKeepsNothingGoingIn(t *testing.T) {
	in := burst(2*synth.Rate, 30, 4800)
	h := playThrough(t, &DelayDescriptor, in, plug.Change{ID: DelayMixID, Value: 50})
	sw := newSweep(h)
	sw.in.Sound = nil
	for h.at = frameStep; h.at < synth.Rate; h.at += frameStep {
		sw.step(time.Second/60, 2)
	}
	if sw.start != 4800/sweepStep {
		t.Errorf("the sweep starts at step %d, not on the hit", sw.start)
	}
}

func TestASweepStepsWithoutAllocating(t *testing.T) {
	in := burst(3*synth.Rate, 30, 4800)
	h := playThrough(t, &DelayDescriptor, in)
	sw := newSweep(h)
	h.at = frameStep
	sw.step(time.Second/60, 2)
	if n := testing.AllocsPerRun(100, func() {
		h.at += frameStep
		sw.step(time.Second/60, 2)
	}); n != 0 {
		t.Errorf("a step allocates %v times", n)
	}
}

func TestTheDelayReportsItsTempo(t *testing.T) {
	p := DelayDescriptor.New(nil)
	pr := p.Processor()
	rep := as[plug.Reporter](t, pr)
	rig{pr: pr, d: &DelayDescriptor, s: setup, tempo: func(int) float64 { return 93 }}.run(t, impulse(1024, 0))
	if got := rep.Report(DelayTempoID); got != 93 {
		t.Errorf("the delay reports a tempo of %g, not 93", got)
	}
	if prm, _ := DelayDescriptor.Param(DelayTempoID); prm.Flags&(plug.ReadOnly|plug.Hidden) != plug.ReadOnly|plug.Hidden {
		t.Error("the tempo reported is a parameter the host may set or show")
	}
}

func TestTheDisplaysDrawTheSweep(t *testing.T) {
	for _, c := range []struct {
		d       *plug.Descriptor
		changes []plug.Change
	}{
		{&DelayDescriptor, []plug.Change{{ID: FeedbackID, Value: 60}}},
		{&ReverbDescriptor, nil},
	} {
		h := playThrough(t, c.d, burst(3*synth.Rate, 30, 4800), c.changes...)
		v := as[plug.Viewer](t, c.d.New(h)).View(plug.Panel, h)
		_, run := stage(t, panelSize, v)
		var sw *sweep
		switch d := as[*panel](t, v).disp.(type) {
		case *taps:
			sw = d.sw
		case *decayGraph:
			sw = d.sw
		}
		for h.at = frameStep; h.at < synth.Rate; h.at += frameStep {
			run(1)
		}
		if f, ok := sw.shown(); !ok || f < 0.99 || sw.n == 0 {
			t.Errorf("%s: the display shows no sweep while the sound plays", c.d.Name)
		}
	}
}
