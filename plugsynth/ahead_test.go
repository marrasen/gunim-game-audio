package plugsynth

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// lands is where the note the look-ahead tests play lands: a second in.
const lands = 48000

// ahead plays the bass, with the knobs set by changes at frame 0, in
// blocks of block frames, for two seconds: one note, landing at lands
// unless news moves it, told of a second ahead, then the news given.
func ahead(t *testing.T, block int, changes []plug.Change, events []plug.Event, news ...plugtest.Ahead) []float32 {
	t.Helper()
	p := synthOf(t, "Moog bass")
	s := plug.Setup{Rate: 48000, MaxBlock: block, Channels: 2}
	if events == nil {
		events = note(1, 45, lands, lands+12000)
	}
	all := append(plugtest.Announce(s, 48000, events...), news...)
	return plugtest.Drive(t, &Descriptor, p.Processor(), s, 2*48000,
		plugtest.Ports{Changes: changes, Events: events, Upcoming: all}).Out[0]
}

// knob sets parameter id to v from the start.
func set(id uint32, v float64) []plug.Change { return []plug.Change{{ID: id, Value: v}} }

func TestTheSwellRisesIntoItsNote(t *testing.T) {
	out := ahead(t, 256, set(SwellID, 500), nil)
	start := lands - 24000
	// Silent until the swell starts, then louder and louder.
	if r := rms(out[:start]); r != 0 {
		t.Errorf("before the swell starts, the synth sounds at %.5f", r)
	}
	last := 0.0
	for q := range 4 {
		a := start + q*6000
		r := rms(out[a : a+6000])
		if r <= last {
			t.Errorf("quarter %d of the swell sounds at %.4f, after %.4f", q, r, last)
		}
		last = r
	}
	if last < 0.02 {
		t.Errorf("the swell ends at %.4f, hardly heard", last)
	}
	// The knob turned to nothing, the note alone sounds.
	if r := rms(ahead(t, 256, nil, nil)[:lands]); r != 0 {
		t.Errorf("with no swell, the synth sounds at %.5f before the note", r)
	}
}

func TestTheSoundAheadIsTheSameInAnyBlockSize(t *testing.T) {
	both := append(set(SwellID, 300), plug.Change{ID: GhostID, Value: 150})
	want := ahead(t, 256, both, nil)
	for _, block := range []int{1, 37, 128} {
		got := ahead(t, block, both, nil)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("in blocks of %d the sound differs from blocks of 256 from frame %d", block, i)
				break
			}
		}
	}
}

func TestWithTheKnobsAtRestNewsLeavesTheSoundAsItWas(t *testing.T) {
	events := append(note(1, 45, 30000, 40000), note(2, 52, 52000, 60000)...)
	p := synthOf(t, "Juno pad")
	s := plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2}
	plain := plugtest.Play(t, p.Processor(), s, 96000, params(), events...)[0]
	p = synthOf(t, "Juno pad")
	told := plugtest.Drive(t, &Descriptor, p.Processor(), s, 96000,
		plugtest.Ports{Events: events, Upcoming: plugtest.Announce(s, 48000, events...)}).Out[0]
	for i := range plain {
		if plain[i] != told[i] {
			t.Fatalf("told of its notes ahead, the synth sounds otherwise from frame %d", i)
		}
	}
}

func TestAMovedNoteTakesItsSwellAlong(t *testing.T) {
	const moved, at = lands + 24000, 30000
	events := note(1, 45, moved, moved+12000)
	news := plugtest.Ahead{At: at, Upcoming: plug.Upcoming{Kind: plug.Moved, Frame: moved, Note: 1, Pitch: 45,
		Velocity: 0.9}}
	// It was told of as landing at lands first.
	first := plugtest.Ahead{At: 0, Upcoming: plug.Upcoming{Kind: plug.Coming, Frame: lands, Note: 1, Pitch: 45,
		Velocity: 0.9}}
	p := synthOf(t, "Moog bass")
	s := plug.Setup{Rate: 48000, MaxBlock: 256, Channels: 2}
	out := plugtest.Drive(t, &Descriptor, p.Processor(), s, 2*48000, plugtest.Ports{Changes: set(SwellID, 500),
		Events: events, Upcoming: []plugtest.Ahead{first, news}}).Out[0]
	still := ahead(t, 256, set(SwellID, 500), nil)
	// Up to the block of the edit, both swell alike; after it, the
	// swell falls back and rises again into where the note lands now.
	for i := range at - at%256 {
		if out[i] != still[i] {
			t.Fatalf("before the edit, the swells part at frame %d", i)
		}
	}
	if a, b := rms(out[lands-6000:lands]), rms(still[lands-6000:lands]); a > b/4 {
		t.Errorf("where the note landed before, the moved swell sounds at %.4f, the swell unmoved at %.4f", a, b)
	}
	if a, b := rms(out[moved-6000:moved]), rms(out[moved-18000:moved-12000]); a <= b || a < 0.02 {
		t.Errorf("into where the note lands now, the swell sounds at %.4f, after %.4f", a, b)
	}
}

func TestARemovedNoteFadesItsSwell(t *testing.T) {
	const at = lands - 8000
	news := plugtest.Ahead{At: at, Upcoming: plug.Upcoming{Kind: plug.Removed, Note: 1}}
	// Its NoteOn is called off.
	out := ahead(t, 256, set(SwellID, 500), []plug.Event{}, append(
		plugtest.Announce(plug.Setup{MaxBlock: 256}, 48000, note(1, 45, lands, lands+12000)...), news)...)
	if r := rms(out[at-6000 : at]); r < 0.01 {
		t.Errorf("before the note is taken back, its swell sounds at %.4f", r)
	}
	// It fades within 20 ms, and a block, of the news.
	if r := rms(out[at+256+960:]); r > 1e-6 {
		t.Errorf("after the note is taken back, its swell sounds at %.6f", r)
	}
}

func TestTheGhostSoundsQuietlyBeforeItsNote(t *testing.T) {
	out := ahead(t, 256, set(GhostID, 200), nil)
	ghost := lands - 9600
	if r := rms(out[:ghost]); r != 0 {
		t.Errorf("before the ghost, the synth sounds at %.5f", r)
	}
	g, n := rms(out[ghost:ghost+4800]), rms(out[lands:lands+4800])
	if g < 0.005 || g > n/2 {
		t.Errorf("the ghost sounds at %.4f, against the note's %.4f", g, n)
	}
}

func TestResetForgetsTheNotesToldOf(t *testing.T) {
	p := synthOf(t, "Moog bass")
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2})
	l, r := make([]float32, 512), make([]float32, 512)
	pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512, Changes: set(SwellID, 500),
		Upcoming: []plug.Upcoming{{Kind: plug.Coming, Frame: 4000, Note: 1, Pitch: 45, Velocity: 1}}})
	for range 4 {
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512})
	}
	if rms(l) == 0 {
		t.Fatal("the swell is silent")
	}
	pr.Reset()
	for range 16 {
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512})
		if s := rms(l); s != 0 {
			t.Fatalf("after a reset, the swell sounds at %.5f", s)
		}
	}
}

func TestSoundingAheadAllocatesNothing(t *testing.T) {
	p := synthOf(t, "Juno pad")
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	l, r := make([]float32, 256), make([]float32, 256)
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 256, Changes: []plug.Change{{ID: SwellID, Value: 400},
		{ID: GhostID, Value: 100}}}
	pr.Process(b)
	b.Changes = nil
	b.Upcoming = make([]plug.Upcoming, 0, 2)
	b.Events = make([]plug.Event, 0, 2)
	n := plugtest.AllocsUnder(t, "plugsynth.(*processor).Process", func() {
		for i := range 600 {
			b.Upcoming, b.Events = b.Upcoming[:0], b.Events[:0]
			id := int32(i)
			// Each block tells of a note 40 blocks on, moves the one told
			// of before, takes back every fifth, and lands the one told
			// of 40 blocks back.
			b.Upcoming = append(b.Upcoming, plug.Upcoming{Kind: plug.Coming, Frame: 40 * 256, Note: id, Pitch: 48 + float64(i%12), Velocity: 0.8})
			if i%5 == 0 {
				b.Upcoming = append(b.Upcoming, plug.Upcoming{Kind: plug.Removed, Note: id - 1})
			} else {
				b.Upcoming = append(b.Upcoming, plug.Upcoming{Kind: plug.Moved, Frame: 39*256 + 10, Note: id - 1, Pitch: 50, Velocity: 0.8})
			}
			b.Events = append(b.Events, plug.Event{Frame: 0, Kind: plug.NoteOn, Note: id - 40, Pitch: 48, Velocity: 0.8},
				plug.Event{Frame: 200, Kind: plug.NoteOff, Note: id - 41})
			pr.Process(b)
		}
	})
	if n != 0 {
		t.Errorf("600 blocks allocate %d times", n)
	}
}
