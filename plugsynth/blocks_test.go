package plugsynth

import (
	"math"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
)

// blockSizes are the block sizes a kit renders the same at.
var blockSizes = []int{256, 1, 37, 100}

// blockFrames is how long each block-size run plays, in frames: two
// seconds, so the busy first second's hits end by themselves, some
// early, some at their time, and some stolen for a new hit.
const blockFrames = 96000

// plugFrames is how long each run through the plugin plays: its first
// second, as a block of 1 frame costs the plugin a Process call a frame.
const plugFrames = 48000

// busyEvents are a second of hits on every General MIDI drum note, a
// few hundred frames apart at varied velocities, many ringing at once,
// with closed hats choking open ones.
func busyEvents() []plug.Event {
	ev := make([]plug.Event, 0, 180)
	at := 0
	for i := range 90 {
		key := 35 + (i*7)%48
		switch i % 9 {
		case 3:
			key = 46
		case 4:
			key = 42
		}
		ev = append(ev, plug.Event{Frame: at, Kind: plug.NoteOn, Note: int32(i), Pitch: float64(key), Velocity: 0.4 + 0.6*float64(i%5)/4},
			plug.Event{Frame: at + 150, Kind: plug.NoteOff, Note: int32(i)})
		at += 233 + (i*97)%700
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

func TestEveryKitIsTheSameInAnyBlockSize(t *testing.T) {
	events := busyEvents()
	for _, s := range kits() {
		name := s.Bank + "/" + s.Group + "/" + s.Name
		var want []float32
		for _, size := range blockSizes {
			in, err := synth.NewInstrument(s.Patch())
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			got := renderLive(in, size, events)
			if want == nil {
				want = got
				continue
			}
			if f := firstDiff(got, want); f >= 0 {
				t.Errorf("%s through the Instrument in blocks of %d differs from blocks of 256 from frame %d", name, size, f)
			}
		}
	}
}

func TestEveryKitIsTheSameInAnyBlockSizeThroughThePlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("plays every kit at four block sizes")
	}
	events := busyEvents()
	for _, s := range kits() {
		name := s.Bank + "/" + s.Group + "/" + s.Name
		for _, conn := range []struct {
			name string
			on   func(int) [synth.DrumParts]bool
		}{{"unconnected", none}, {"connected", every}} {
			var want outs
			for _, size := range blockSizes {
				p, _ := newSynth(t)
				if err := p.SetState(s.State()); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				got := drive(t, p.Processor(), 48000, size, plugFrames, events, conn.on)
				if want.main == nil {
					want = got
					continue
				}
				for c := range 2 {
					if f := firstDiff(got.main[c], want.main[c]); f >= 0 {
						t.Errorf("%s, aux outputs %s, in blocks of %d: channel %d differs from blocks of 256 from frame %d", name, conn.name, size, c, f)
					}
					for i := range got.aux {
						if got.aux[i] == nil {
							continue
						}
						if f := firstDiff(got.aux[i][c], want.aux[i][c]); f >= 0 {
							t.Errorf("%s in blocks of %d: aux output %d channel %d differs from blocks of 256 from frame %d", name, size, i, c, f)
						}
					}
				}
			}
		}
	}
}
