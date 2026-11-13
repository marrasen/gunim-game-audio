package plugmod

import (
	"fmt"
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

// span runs pr over n frames from c on, in blocks of 64 and a last one of
// what is left, the first with the changes ch, with a hit at each of the
// song's frames in hits, and returns the main output. The tempo is 120.
// The host's frame moves on, and the song's too unless pos is still.
func span(pr *processor, c *clock, n int, pos plug.Position, ch []plug.Change, hits ...int64) []float32 {
	out := make([]float32, 0, n)
	for n > 0 {
		size := min(n, 64)
		pos.Frame, pos.Song = c.frame, c.song
		pos.Tempo, pos.EndTempo, pos.Beat = 120, 120, float64(c.song)/rate*2
		bl := newBlock(size, pos)
		bl.b.Changes, ch = ch, nil
		trig := make([]float32, size)
		for _, h := range hits {
			if !pos.Still && h >= c.song && h < c.song+int64(size) {
				trig[h-c.song] = 1
			}
		}
		bl.b.ControlIn = [][]float32{nil, nil, nil, nil, trig}
		pr.Process(&bl.b)
		out = append(out, bl.outs[MainOut]...)
		c.frame += int64(size)
		if !pos.Still {
			c.song += int64(size)
		}
		n -= size
	}
	return out
}

// jumpy are the settings of an LFO of shape counted from start, with
// jitter, at 37 Hz free and a 16th note synced.
func jumpy(shape, start, sync int) []float64 {
	return []float64{float64(ShapeID), float64(shape), float64(StartID), float64(start), float64(SyncID), float64(sync),
		float64(RateID), 37, float64(NoteID), 14, float64(SeedID), 7, float64(JitterID), 0.3}
}

func TestAHitInALoopPlaysEveryPassAlike(t *testing.T) {
	// The loop's end falls inside a block of 64, so the host splits the block there.
	const from, length, hit = 96000, 20000, 7000
	for _, shape := range []int{Saw, SampleHold} {
		for _, start := range []int{FromSong, FromPlay} {
			for sync := range 2 {
				pr := prepared(t)
				c := &clock{frame: 7000000, song: from}
				ch := changes(jumpy(shape, start, sync)...)
				var passes [3][]float32
				for i := range passes {
					// The song wraps, the host's frame runs on, and no Reset comes.
					c.song = from
					passes[i] = span(pr, c, length, plug.Position{Playing: true}, ch, from+hit)
					ch = nil
				}
				what := fmt.Sprintf("%s from %s, sync %d", Shapes[shape], Starts[start], sync)
				if passes[0][hit] == passes[0][hit-1] {
					t.Errorf("%s: the hit moves nothing", what)
				}
				same(t, what+": the second pass against the first", passes[1], passes[0])
				same(t, what+": the third pass against the first", passes[2], passes[0])
			}
		}
	}
}

func TestASeekWhileStoppedPutsAStruckLFOOnTheSongAtOnce(t *testing.T) {
	for _, shape := range []int{Saw, SampleHold} {
		for sync := range 2 {
			kv := jumpy(shape, FromSong, sync)
			pr := prepared(t)
			c := &clock{frame: 7000000, song: 96000}
			span(pr, c, 640, plug.Position{}, changes(kv...), 96100)
			still := plug.Position{Still: true}
			held := span(pr, c, 640, still, nil)
			if !pr.struck || held[0] == held[639] {
				t.Fatalf("%s, sync %d: stopped after a hit, the LFO is struck %v and moves from %v to %v, want it running on",
					Shapes[shape], sync, pr.struck, held[0], held[639])
			}
			// The song's frame moves while the song stands still.
			c.song = 31337
			got := span(pr, c, 640, still, nil)
			want := span(prepared(t), &clock{frame: 31337, song: 31337}, 640, still, changes(kv...))
			same(t, fmt.Sprintf("%s, sync %d: after a seek while stopped", Shapes[shape], sync), got, want)
		}
	}
}

func TestAHitOnTheFirstFrameAfterAWrapStrikes(t *testing.T) {
	const from, length = 96000, 20000
	for _, start := range []int{FromSong, FromPlay} {
		for sync := range 2 {
			kv := jumpy(SampleHold, start, sync)
			pr := prepared(t)
			c := &clock{frame: 7000000, song: from}
			span(pr, c, length, plug.Position{Playing: true}, changes(kv...), from+7000)
			c.song = from
			out := span(pr, c, 64, plug.Position{Playing: true}, nil, from)
			what := fmt.Sprintf("from %s, sync %d", Starts[start], sync)
			// The wrap puts the random values back, and the hit moves them on once.
			s := set(1, kv...)
			if want := float32(s.wave(0)); out[0] != want {
				t.Errorf("%s: on the frame of the wrap and the hit the wave is at %v, want %v", what, out[0], want)
			}
			if pr.struck != (start == FromSong) {
				t.Errorf("%s: after the hit the LFO is struck %v", what, pr.struck)
			}
			step := 37.0 / rate
			if sync == 1 {
				step = 2.0 / rate / Beats[14]
			}
			if !near(pr.cycles, 64*step, 1e-9) {
				t.Errorf("%s: a block after the hit the LFO is %v cycles on, want %v", what, pr.cycles, 64*step)
			}
		}
	}
}

func TestBlocksThatFollowOnNeverRejoin(t *testing.T) {
	for sync := range 2 {
		pr := prepared(t)
		c := &clock{frame: 7000000, song: 96000}
		span(pr, c, 64, plug.Position{Playing: true}, changes(jumpy(SampleHold, FromSong, sync)...), 96010)
		check := func(when string) {
			t.Helper()
			if !pr.struck || pr.s.draw != drawStep {
				t.Fatalf("sync %d, %s: the LFO is struck %v with draw %d, want it struck with draw %d", sync, when,
					pr.struck, pr.s.draw, int64(drawStep))
			}
		}
		// Blocks of many lengths, each starting where the last ended.
		for _, n := range []int{64, 1, 17, 640, 63, 1000} {
			span(pr, c, n, plug.Position{Playing: true}, nil)
			check(fmt.Sprintf("after %d frames more of playing", n))
		}
		// The song stands where the playing stopped, block after block.
		for i := range 10 {
			span(pr, c, 64+i, plug.Position{Still: true}, nil)
			check(fmt.Sprintf("after %d blocks standing still", i+1))
		}
		span(pr, c, 640, plug.Position{}, nil)
		check("playing on from where the song stood")
	}
}

func TestAJumpLeavesAFreeRunningLFORunningOn(t *testing.T) {
	for _, shape := range []int{Saw, SampleHold} {
		// A free LFO's step is the same at every place, so the outputs are the same to the bit.
		kv := jumpy(shape, FreeRunning, 0)
		pr, straight := prepared(t), prepared(t)
		c, d := &clock{frame: 7000000, song: 96000}, &clock{frame: 7000000, song: 96000}
		span(pr, c, 1000, plug.Position{Playing: true}, changes(kv...), 96300)
		span(straight, d, 1000, plug.Position{Playing: true}, changes(kv...), 96300)
		c.song = 12345
		got := span(pr, c, 1000, plug.Position{Playing: true}, nil)
		want := span(straight, d, 1000, plug.Position{Playing: true}, nil)
		same(t, Shapes[shape]+": running free through a jump", got, want)
		// The song's frame moves while the song stands still.
		still := plug.Position{Still: true}
		span(pr, c, 100, still, nil)
		span(straight, d, 100, still, nil)
		c.song = 500000
		same(t, Shapes[shape]+": running free through a seek while stopped", span(pr, c, 1000, still, nil),
			span(straight, d, 1000, still, nil))
	}
	// Synced, it keeps its count and its random values through the jump.
	pr := prepared(t)
	c := &clock{frame: 7000000, song: 96000}
	span(pr, c, 1000, plug.Position{Playing: true}, changes(jumpy(SampleHold, FreeRunning, 1)...), 96300)
	before := pr.cycles
	c.song = 12345
	span(pr, c, 64, plug.Position{Playing: true}, nil)
	if want := before + 64*2.0/rate/Beats[14]; !near(pr.cycles, want, 1e-9) || pr.s.draw != drawStep {
		t.Errorf("synced and running free, a block after a jump the LFO is %v cycles on with draw %d, want %v with draw %d",
			pr.cycles, pr.s.draw, want, int64(drawStep))
	}
}
