package plugsynth

import (
	"math"
	"strconv"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
)

// blockSizes are the block sizes a kit renders the same at: 256, the
// largest; a frame, the smallest; and 37, odd, which falls across the
// control steps of 32 frames.
var blockSizes = []int{256, 1, 37}

// blockFrames is how long each block-size run plays, in frames: two
// seconds, so the busy first second's hits end by themselves, some
// early, some at their time, and some stolen for a new hit.
const blockFrames = 96000

// plugFrames is how long each run through the plugin plays: its first
// second, as a block of 1 frame costs the plugin a Process call a frame.
const plugFrames = 48000

// typeKit is a kit of every type of drum, each named by the note it is
// on, from 35 up, as General MIDI's kits are. Its twelve voices are too
// few for busyEvents' hits, so some are stolen.
func typeKit() Sound {
	p := &synth.Patch{Kind: "drums", Poly: 12, Kit: map[string]synth.Drum{}}
	for i, ty := range synth.DrumTypes {
		p.Kit[strconv.Itoa(35+i)] = synth.Drum{Type: ty}
	}
	return Sound{Bank: "Test", Group: "Drum kits", Name: "Every type", patch: p, drums: true}
}

// busyEvents are a second of hits on each of typeKit's drums, up the
// notes and back down, a few hundred frames apart at varied velocities,
// many ringing at once. Each closed hat's note comes before its open
// hat's, so on the way down a closed hat chokes its open one.
func busyEvents() []plug.Event {
	n := len(synth.DrumTypes)
	ev := make([]plug.Event, 0, 4*n)
	at := 0
	for i := range 2 * n {
		key := 35 + i
		if i >= n {
			key = 35 + 2*n - 1 - i
		}
		ev = append(ev, plug.Event{Frame: at, Kind: plug.NoteOn, Note: int32(i), Pitch: float64(key), Velocity: 0.4 + 0.6*float64(i%5)/4},
			plug.Event{Frame: at + 150, Kind: plug.NoteOff, Note: int32(i)})
		at += 160 + (i*97)%400
	}
	// Each NoteOff comes before the next NoteOn, so the list runs in
	// frame order.
	return ev
}

// renderLive plays events through in, in blocks of size frames, each
// event at its frame, and returns the left channel.
func renderLive(in *synth.Instrument, size int, events []plug.Event) []float32 {
	out := make([]float32, 0, blockFrames)
	l, r := make([]float32, size), make([]float32, size)
	next := 0
	for at := 0; at < blockFrames; {
		for next < len(events) && events[next].Frame == at {
			e := events[next]
			if e.Kind == plug.NoteOn {
				in.NoteOn(e.Note, e.Pitch, e.Velocity, 0)
			} else {
				in.NoteOff(e.Note)
			}
			next++
		}
		n := min(size, blockFrames-at)
		if next < len(events) {
			n = min(n, events[next].Frame-at)
		}
		in.Render(l[:n], r[:n])
		out = append(out, l[:n]...)
		at += n
	}
	return out
}

// firstDiff returns the first frame where got and want differ, or -1.
func firstDiff(got, want []float32) int {
	for i := range want {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			return i
		}
	}
	return -1
}

// TestEveryDrumTypeIsTheSameInAnyBlockSize plays typeKit, a drum of
// every type, through the Instrument, in each of blockSizes.
func TestEveryDrumTypeIsTheSameInAnyBlockSize(t *testing.T) {
	events := busyEvents()
	s := typeKit()
	var want []float32
	for _, size := range blockSizes {
		in, err := synth.NewInstrument(s.Patch())
		if err != nil {
			t.Fatal(err)
		}
		got := renderLive(in, size, events)
		if want == nil {
			want = got
			continue
		}
		if f := firstDiff(got, want); f >= 0 {
			t.Errorf("through the Instrument in blocks of %d differs from blocks of 256 from frame %d", size, f)
		}
	}
}

// TestEveryDrumTypeIsTheSameInAnyBlockSizeThroughThePlugin plays
// typeKit through the plugin, in each of blockSizes, its aux outputs
// unconnected and connected.
func TestEveryDrumTypeIsTheSameInAnyBlockSizeThroughThePlugin(t *testing.T) {
	events := busyEvents()
	s := typeKit()
	for _, conn := range []struct {
		name string
		on   func(int) [synth.DrumParts]bool
	}{{"unconnected", none}, {"connected", every}} {
		var want outs
		for _, size := range blockSizes {
			p, _ := newSynth(t)
			if err := p.SetState(s.State()); err != nil {
				t.Fatal(err)
			}
			got := drive(t, p.Processor(), 48000, size, plugFrames, events, conn.on)
			if want.main == nil {
				want = got
				continue
			}
			for c := range 2 {
				if f := firstDiff(got.main[c], want.main[c]); f >= 0 {
					t.Errorf("aux outputs %s, in blocks of %d: channel %d differs from blocks of 256 from frame %d", conn.name, size, c, f)
				}
				for i := range got.aux {
					if got.aux[i] == nil {
						continue
					}
					if f := firstDiff(got.aux[i][c], want.aux[i][c]); f >= 0 {
						t.Errorf("in blocks of %d: aux output %d channel %d differs from blocks of 256 from frame %d", size, i, c, f)
					}
				}
			}
		}
	}
}
