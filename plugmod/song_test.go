package plugmod

import (
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"slices"
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

// clock is a host's two counts: its own frame, and the song's.
type clock struct{ frame, song int64 }

// free are the changes to a free LFO of shape counted from start, with
// jitter and, for every other shape, smoothing.
func free(shape, start int) []plug.Change {
	return changes(float64(ShapeID), float64(shape), float64(StartID), float64(start), float64(SyncID), 0,
		float64(RateID), 37, float64(PhaseID), 45, float64(SeedID), 7, float64(JitterID), 0.3,
		float64(SmoothID), 0.2*float64(shape%2))
}

// play runs pr over blocks of 64 frames at pos from c on, the first with
// the changes ch, and returns what it put out, block by block, each
// block's four outputs in turn. The host's frame moves on, and the
// song's too unless pos is still.
func play(pr *processor, c *clock, blocks int, pos plug.Position, ch []plug.Change) []float32 {
	var out []float32
	for range blocks {
		pos.Frame, pos.Song = c.frame, c.song
		bl := newBlock(64, pos)
		bl.b.Changes, ch = ch, nil
		pr.Process(&bl.b)
		for _, o := range bl.outs {
			out = append(out, o...)
		}
		c.frame += 64
		if !pos.Still {
			c.song += 64
		}
	}
	return out
}

// prepared returns a new LFO, prepared.
func prepared(t *testing.T) *processor {
	t.Helper()
	pr := newLFO(t)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: 64})
	return pr
}

// same fails the test where got and want differ in any bit.
func same(t *testing.T, what string, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d values, want %d", what, len(got), len(want))
	}
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("%s: value %d is %v, want %v", what, i, got[i], want[i])
		}
	}
}

func TestAFreeLFOFromTheSongIsTheSameWhateverTheHostsFrame(t *testing.T) {
	for shape, name := range Shapes {
		for _, playing := range []bool{true, false} {
			pos := plug.Position{Playing: playing}
			// A render counts both from the song's start; a live host's
			// frame has run on long before the play starts.
			render := play(prepared(t), &clock{frame: 30000, song: 30000}, 40, pos, free(shape, FromSong))
			live := play(prepared(t), &clock{frame: 987654321, song: 30000}, 40, pos, free(shape, FromSong))
			same(t, fmt.Sprintf("%s, playing %v: with the host's frame apart from the song's", name, playing), live, render)
			// The song's place is what moves the wave.
			other := play(prepared(t), &clock{frame: 30000, song: 31000}, 40, pos, free(shape, FromSong))
			if slices.Equal(other, render) {
				t.Errorf("%s, playing %v: the wave is the same at another place in the song", name, playing)
			}
		}
	}
}

// A synced LFO whose host knows no tempo counts the song's seconds too.
func TestASyncedLFOWithNoTempoIsTheSameWhateverTheHostsFrame(t *testing.T) {
	for shape, name := range Shapes {
		ch := changes(float64(ShapeID), float64(shape), float64(SyncID), 1, float64(NoteID), 14, float64(SeedID), 7)
		render := play(prepared(t), &clock{frame: 30000, song: 30000}, 40, plug.Position{Playing: true}, ch)
		live := play(prepared(t), &clock{frame: 987654321, song: 30000}, 40, plug.Position{Playing: true}, ch)
		same(t, name+": with the host's frame apart from the song's", live, render)
		other := play(prepared(t), &clock{frame: 30000, song: 31000}, 40, plug.Position{Playing: true}, ch)
		if slices.Equal(other, render) {
			t.Errorf("%s: the wave is the same at another place in the song", name)
		}
	}
}

func TestAFreeLFOFromTheSongFollowsAJumpOnItsFirstFrame(t *testing.T) {
	for _, jump := range []struct {
		name string
		to   int64
	}{
		{"a seek forward", 500000},
		{"a loop back", 12345},
	} {
		for shape, name := range Shapes {
			pos := plug.Position{Playing: true}
			pr := prepared(t)
			c := &clock{frame: 7000000, song: 96000}
			play(pr, c, 10, pos, free(shape, FromSong))
			// The song jumps as the next block starts; the host's frame
			// runs on, and no Reset comes.
			c.song = jump.to
			got := play(pr, c, 10, pos, nil)
			want := play(prepared(t), &clock{frame: jump.to, song: jump.to}, 10, pos, free(shape, FromSong))
			same(t, fmt.Sprintf("%s, after %s", name, jump.name), got, want)
		}
	}
}

func TestAFreeLFOFromTheSongHoldsAtTheSongsPlaceWhileItStandsStill(t *testing.T) {
	for shape, name := range Shapes {
		pr := prepared(t)
		c := &clock{frame: 7000000, song: 96000}
		play(pr, c, 10, plug.Position{Playing: true}, free(shape, FromSong))
		// The song stands where the play stopped, and then at another
		// place, as a seek while stopped puts it.
		for _, place := range []int64{c.song, 31337} {
			c.song = place
			held := play(pr, c, 5, plug.Position{Playing: true, Still: true}, nil)
			want := play(prepared(t), &clock{frame: place, song: place}, 1, plug.Position{Playing: true}, free(shape, FromSong))
			for i, v := range held {
				out := i % (64 * 4) / 64
				if w := want[out*64]; math.Float32bits(v) != math.Float32bits(w) {
					t.Fatalf("%s: still at song frame %d, output %d puts out %v, want %v, the wave there", name, place, out, v, w)
				}
			}
		}
		got := play(pr, c, 5, plug.Position{Playing: true}, nil)
		want := play(prepared(t), &clock{frame: 31337, song: 31337}, 5, plug.Position{Playing: true}, free(shape, FromSong))
		same(t, name+": playing on from where the song stood", got, want)
	}
}

// others plays every shape counted from the play and running free, free
// and synced, stopped, then playing, then standing still, then on a
// tempo ramp. The song's frame is apart frames on from the host's as it
// starts, and holds while the song stands still. tempo is the tempo the
// host tells while stopped, 0 for none. It returns all the LFO put out.
func others(t *testing.T, tempo float64, apart int64) []float32 {
	t.Helper()
	var out []float32
	for shape := range Shapes {
		for _, start := range []int{FromPlay, FreeRunning} {
			for sync := range 2 {
				ch := append(free(shape, start), changes(float64(SyncID), float64(sync), float64(NoteID), 14)...)
				pr := prepared(t)
				frame := int64(48000)
				song := frame + apart
				for i, part := range []plug.Position{
					{Tempo: tempo},
					{Playing: true, Tempo: 97, EndTempo: 97},
					{Playing: true, Still: true, Tempo: 97},
					{Playing: true, Tempo: 140, EndTempo: 150},
				} {
					for k := range 6 {
						part.Frame, part.Song = frame, song
						part.Beat = float64(frame) / rate * 2
						bl := newBlock(64, part)
						if i == 0 && k == 0 {
							bl.b.Changes = ch
						}
						pr.Process(&bl.b)
						for _, o := range bl.outs {
							out = append(out, o...)
						}
						frame += 64
						if !part.Still {
							song += 64
						}
					}
				}
			}
		}
	}
	return out
}

func TestFromThePlayAndRunningFreeAreAsTheyWereToTheBit(t *testing.T) {
	const with, apart = 0, 5_000_000
	same(t, "with the song's frame apart from the host's", others(t, 97, apart), others(t, 97, with))
	// A synced LFO whose host knows no tempo steps by the song's frame,
	// so its last bits round by where the song is.
	together := others(t, 0, with)
	for i, v := range others(t, 0, apart) {
		if !near(float64(v), float64(together[i]), 1e-6) {
			t.Fatalf("with no tempo and the song's frame apart from the host's: value %d is %v, want %v", i, v, together[i])
		}
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("the hash was taken on amd64, and another processor rounds the sums of the wave another way")
	}
	h := fnv.New64a()
	var b [4]byte
	for _, v := range append(together, others(t, 97, with)...) {
		bits := math.Float32bits(v)
		b[0], b[1], b[2], b[3] = byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)
		h.Write(b[:])
	}
	// The hash of the output at commit 86d268f, where the LFO counted
	// the song by the host's frame.
	const before = 0x25287a89154dd627
	if got := h.Sum64(); got != before {
		t.Errorf("counted from the play and running free, the outputs hash to %#x, want %#x as before", got, uint64(before))
	}
}
