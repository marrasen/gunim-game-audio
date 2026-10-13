package plugsynth

import (
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// kitHits are a drum of each part, a few hundred milliseconds apart:
// a kick, a snare, a hat, a tom, a crash and a cowbell, then the kick
// again under a clap.
var kitHits = []struct {
	key, part int
	at        int
}{
	{36, KickOut, 100}, {38, SnareOut, 6000}, {42, HatsOut, 12000}, {45, TomsOut, 18000},
	{49, CymbalOut, 24000}, {56, PercOut, 30000}, {36, KickOut, 36000}, {39, SnareOut, 36000},
}

func kitEvents() []plug.Event {
	ev := make([]plug.Event, 0, 2*len(kitHits))
	for i, h := range kitHits {
		ev = append(ev, plug.Event{Frame: h.at, Kind: plug.NoteOn, Note: int32(i), Pitch: float64(h.key), Velocity: 0.9},
			plug.Event{Frame: h.at + 200, Kind: plug.NoteOff, Note: int32(i)})
	}
	slices.SortStableFunc(ev, func(a, b plug.Event) int { return a.Frame - b.Frame })
	return ev
}

// outs is what a test's host heard: the main output, and each drum
// output, nil where it was never connected.
type outs struct {
	main [][]float32
	aux  [synth.DrumParts][][]float32
}

// drive plays pr as a host does, in blocks of size frames, events in
// frame order, with the drum outputs connected in each block as
// connected says, and returns
// what came out. Each output connected is filled with NaN before the
// block, so a frame left unwritten shows; one unconnected reads as
// silence.
func drive(t *testing.T, pr plug.Processor, rate float64, size, frames int, events []plug.Event,
	connected func(block int) [synth.DrumParts]bool) outs {
	t.Helper()
	var o outs
	o.main = [][]float32{make([]float32, frames), make([]float32, frames)}
	main := [][]float32{make([]float32, size), make([]float32, size)}
	var aux [synth.DrumParts][][]float32
	for i := range aux {
		aux[i] = [][]float32{make([]float32, size), make([]float32, size)}
	}
	auxOut := make([][][]float32, synth.DrumParts)
	pr.Prepare(plug.Setup{Rate: rate, MaxBlock: size, Channels: 2})
	nan := float32(math.NaN())
	next := 0
	for at, k := 0, 0; at < frames; at, k = at+size, k+1 {
		n := min(size, frames-at)
		b := plug.Block{Frames: n, Out: [][]float32{main[0][:n], main[1][:n]}, AuxOut: auxOut,
			Position: plug.Position{Frame: int64(at), Playing: true}}
		if k == 0 {
			for _, p := range Descriptor.Params {
				b.Changes = append(b.Changes, plug.Change{ID: p.ID, Value: p.Default})
			}
		}
		for ; next < len(events) && events[next].Frame < at+n; next++ {
			e := events[next]
			e.Frame -= at
			b.Events = append(b.Events, e)
		}
		on := connected(k)
		for i := range auxOut {
			auxOut[i] = nil
			if on[i] {
				auxOut[i] = [][]float32{aux[i][0][:n], aux[i][1][:n]}
				for _, ch := range auxOut[i] {
					for f := range ch {
						ch[f] = nan
					}
				}
			}
		}
		pr.Process(&b)
		for c := range 2 {
			copy(o.main[c][at:], b.Out[c])
		}
		for i := range auxOut {
			if auxOut[i] == nil {
				continue
			}
			if o.aux[i] == nil {
				o.aux[i] = [][]float32{make([]float32, frames), make([]float32, frames)}
			}
			for c := range 2 {
				copy(o.aux[i][c][at:], auxOut[i][c])
			}
		}
	}
	return o
}

// every connects every drum output, and none none.
func every(int) [synth.DrumParts]bool {
	return [synth.DrumParts]bool{true, true, true, true, true, true}
}

func none(int) [synth.DrumParts]bool { return [synth.DrumParts]bool{} }

func TestTheDrumOutputsAreNamedByPart(t *testing.T) {
	want := []string{"Kick", "Snare", "Hats", "Toms", "Cymbals", "Perc"}
	if len(Descriptor.AuxOut) != len(want) {
		t.Fatalf("the synth has %d aux outputs, want %d", len(Descriptor.AuxOut), len(want))
	}
	for i, p := range Descriptor.AuxOut {
		if p.Name != want[i] || p.Channels != 0 {
			t.Errorf("aux output %d is %q of %d channels, want %q as the main sound", i, p.Name, p.Channels, want[i])
		}
	}
	for _, h := range kitHits {
		if got := synth.DrumPart(h.key); got != h.part {
			t.Errorf("note %d plays into part %d, want %d", h.key, got, h.part)
		}
	}
}

func TestUnconnectedTheDrumsPlayAsBefore(t *testing.T) {
	for _, rate := range []float64{48000, 44100} {
		want := plugtest.Play(t, synthOf(t, "Standard kit").Processor(), plug.Setup{Rate: rate, MaxBlock: 256, Channels: 2},
			48000, params(), kitEvents()...)
		got := drive(t, synthOf(t, "Standard kit").Processor(), rate, 256, 48000, kitEvents(), none)
		for c := range 2 {
			for f := range want[c] {
				if math.Float32bits(got.main[c][f]) != math.Float32bits(want[c][f]) {
					t.Fatalf("at %v Hz, channel %d frame %d is %v, want %v as before", rate, c, f, got.main[c][f], want[c][f])
				}
			}
		}
	}
}

// silentUntil reports the first frame of x louder than a whisper, or -1.
func silentUntil(x []float32) int {
	for f, v := range x {
		if math.Abs(float64(v)) > 1e-6 {
			return f
		}
	}
	return -1
}

func TestEachDrumPlaysIntoItsOwnOutput(t *testing.T) {
	for _, rate := range []float64{48000, 44100} {
		ref := drive(t, synthOf(t, "Standard kit").Processor(), rate, 256, 48000, kitEvents(), none)
		got := drive(t, synthOf(t, "Standard kit").Processor(), rate, 256, 48000, kitEvents(), every)
		for c := range 2 {
			if f := silentUntil(got.main[c]); f >= 0 {
				t.Fatalf("at %v Hz, with every drum's output connected, the main output sounds at frame %d", rate, f)
			}
		}
		for part := range synth.DrumParts {
			first := -1
			for _, h := range kitHits {
				if h.part == part && first < 0 {
					first = h.at
				}
			}
			on := silentUntil(got.aux[part][0])
			// The drum sounds within a few frames of its note: the
			// resampling's latency.
			if on < first || on > first+4 {
				t.Errorf("at %v Hz, the %s output first sounds at frame %d, want its drum's, %d", rate, Descriptor.AuxOut[part].Name, on, first)
			}
		}
		// Together, the outputs sound as the main output did alone.
		for c := range 2 {
			for f := range ref.main[c] {
				var sum float32
				for part := range synth.DrumParts {
					sum += got.aux[part][c][f]
				}
				if math.IsNaN(float64(sum)) || math.Abs(float64(sum-ref.main[c][f])) > 1e-5 {
					t.Fatalf("at %v Hz, channel %d frame %d: the outputs add up to %v, want %v", rate, c, f, sum, ref.main[c][f])
				}
			}
		}
	}
}

func TestADrumOfAnUnconnectedOutputPlaysInTheMain(t *testing.T) {
	ref := drive(t, synthOf(t, "TR-808 kit").Processor(), 48000, 128, 48000, kitEvents(), none)
	kickAndSnare := func(int) [synth.DrumParts]bool { return [synth.DrumParts]bool{KickOut: true, SnareOut: true} }
	got := drive(t, synthOf(t, "TR-808 kit").Processor(), 48000, 128, 48000, kitEvents(), kickAndSnare)
	for part := range synth.DrumParts {
		if (got.aux[part] != nil) != (part == KickOut || part == SnareOut) {
			t.Errorf("the %s output was written as unconnected, or skipped as connected", Descriptor.AuxOut[part].Name)
		}
	}
	// The hat, at 12000, is the first drum in the main output.
	if f := silentUntil(got.main[0]); f < 12000 || f > 12004 {
		t.Errorf("the main output first sounds at frame %d, want the hat's, 12000", f)
	}
	for c := range 2 {
		for f := range ref.main[c] {
			sum := got.main[c][f] + got.aux[KickOut][c][f] + got.aux[SnareOut][c][f]
			if math.Abs(float64(sum-ref.main[c][f])) > 1e-5 {
				t.Fatalf("channel %d frame %d: the main, kick and snare add up to %v, want %v", c, f, sum, ref.main[c][f])
			}
		}
	}
}

func TestADrumMovesBetweenOutputsAsTheyConnect(t *testing.T) {
	// The kick's output connects and disconnects block by block, as the
	// kick rings across the edges: at every frame the sound is whole,
	// in the main output or the kick's, and none is lost or doubled.
	for _, rate := range []float64{48000, 44100} {
		ref := drive(t, synthOf(t, "Standard kit").Processor(), rate, 64, 12000, kitEvents()[:4], none)
		flip := func(k int) [synth.DrumParts]bool { return [synth.DrumParts]bool{KickOut: k%3 != 1, SnareOut: k%5 == 0} }
		got := drive(t, synthOf(t, "Standard kit").Processor(), rate, 64, 12000, kitEvents()[:4], flip)
		for c := range 2 {
			for f := range ref.main[c] {
				k := f / 64
				sum := got.main[c][f]
				if flip(k)[KickOut] {
					sum += got.aux[KickOut][c][f]
				}
				if flip(k)[SnareOut] {
					sum += got.aux[SnareOut][c][f]
				}
				if math.IsNaN(float64(sum)) || math.Abs(float64(sum-ref.main[c][f])) > 1e-5 {
					t.Fatalf("at %v Hz, channel %d frame %d (block %d): the outputs add up to %v, want %v", rate, c, f, k, sum, ref.main[c][f])
				}
			}
		}
	}
}

func TestASynthOtherThanDrumsLeavesTheDrumOutputsSilent(t *testing.T) {
	ev := note(1, 57, 0, 20000)
	ref := drive(t, synthOf(t, "Moog bass").Processor(), 48000, 256, 24000, ev, none)
	got := drive(t, synthOf(t, "Moog bass").Processor(), 48000, 256, 24000, ev, every)
	for part := range synth.DrumParts {
		if f := silentUntil(got.aux[part][0]); f >= 0 {
			t.Errorf("the %s output sounds at frame %d", Descriptor.AuxOut[part].Name, f)
		}
	}
	for c := range 2 {
		for f := range ref.main[c] {
			if got.main[c][f] != ref.main[c][f] {
				t.Fatalf("channel %d frame %d is %v, want %v", c, f, got.main[c][f], ref.main[c][f])
			}
		}
	}
}

func TestADrumRingingOutOfASoundChangedPlaysIntoItsOutput(t *testing.T) {
	// The kit changes to another under a crash: the crash rings out, on
	// the cymbals' output.
	p := synthOf(t, "Standard kit")
	pr := p.Processor()
	// The note lets go before the change, so the new sound takes none.
	ev := []plug.Event{{Frame: 10, Kind: plug.NoteOn, Note: 1, Pitch: 49, Velocity: 1}, {Frame: 300, Kind: plug.NoteOff, Note: 1}}
	swapped := false
	cymbals := func(k int) [synth.DrumParts]bool {
		if k == 10 && !swapped {
			swapped = true
			s, _ := Named("Moog bass")
			if err := p.SetState(s.State()); err != nil {
				t.Fatal(err)
			}
		}
		return [synth.DrumParts]bool{CymbalOut: true}
	}
	got := drive(t, pr, 48000, 256, 12000, ev, cymbals)
	if f := silentUntil(got.main[0]); f >= 0 {
		t.Errorf("the main output sounds at frame %d", f)
	}
	if rms(got.aux[CymbalOut][0][6000:]) < 1e-3 {
		t.Error("the crash stopped ringing on the cymbals' output as the sound changed")
	}
}

func TestPlayingIntoTheDrumOutputsAllocatesNothing(t *testing.T) {
	p := synthOf(t, "Standard kit")
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	states := swapStates(t)
	l, r := make([]float32, 256), make([]float32, 256)
	aux := make([][][]float32, synth.DrumParts)
	room := make([][][]float32, synth.DrumParts)
	for i := range room {
		room[i] = [][]float32{make([]float32, 256), make([]float32, 256)}
	}
	b := &plug.Block{Out: [][]float32{l, r}, AuxOut: aux, Frames: 256, Events: make([]plug.Event, 2)}
	n := plugtest.AllocsUnder(t, "plugsynth.(*processor).Process", func() {
		for i := range 400 {
			if i%53 == 0 {
				if err := p.SetState(states[i/53%len(states)]); err != nil {
					t.Fatal(err)
				}
			}
			for k := range aux {
				aux[k] = nil
				if (i+k)%4 != 0 {
					aux[k] = room[k]
				}
			}
			b.Events[0] = plug.Event{Frame: 10, Kind: plug.NoteOn, Note: int32(i), Pitch: float64(35 + i%25), Velocity: 0.8}
			b.Events[1] = plug.Event{Frame: 200, Kind: plug.NoteOff, Note: int32(i)}
			b.Position.Frame = int64(i) * 256
			pr.Process(b)
		}
	})
	if n != 0 {
		t.Errorf("400 blocks allocate %d times", n)
	}
}

func TestDrivenAsAHostDoesTheDrumsComeOutApart(t *testing.T) {
	// plugtest.Drive connects every output.
	s := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	res := plugtest.Drive(t, &Descriptor, synthOf(t, "Electronic kit").Processor(), s, 40000, plugtest.Ports{Events: kitEvents()})
	want := plugtest.Play(t, synthOf(t, "Electronic kit").Processor(), s, 40000, params(), kitEvents()...)
	if f := silentUntil(res.Out[0]); f >= 0 {
		t.Errorf("the main output sounds at frame %d", f)
	}
	for c := range 2 {
		for f := range want[c] {
			var sum float32
			for part := range res.AuxOut {
				sum += res.AuxOut[part][c][f]
			}
			if math.IsNaN(float64(sum)) || math.Abs(float64(sum-want[c][f])) > 1e-5 {
				t.Fatalf("channel %d frame %d: the drum outputs add up to %v, want %v", c, f, sum, want[c][f])
			}
		}
	}
}
