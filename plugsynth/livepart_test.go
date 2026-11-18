package plugsynth

import (
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// liveSetup is the host the live part's tests run at.
var liveSetup = plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}

// driveLive runs a synth playing the lead, with its live output connected,
// over n frames.
func driveLive(t *testing.T, n int, ports plugtest.Ports) plugtest.Result {
	t.Helper()
	p := synthOf(t, "Jupiter brass lead")
	return plugtest.Drive(t, &Descriptor, p.Processor(), liveSetup, n, ports)
}

// liveNote is note, played on the live part.
func liveNote(id int32, pitch float64, on, off int) []plug.Event {
	es := note(id, pitch, on, off)
	for i := range es {
		es[i].Live = true
	}
	return es
}

func TestLivePartPlaysOnItsOwnOutput(t *testing.T) {
	n := 48000
	song := note(1, 57, 0, 36000)
	alone := driveLive(t, n, plugtest.Ports{Events: song})
	// The live key uses the song note's ID too: the parts keep theirs apart.
	both := driveLive(t, n, plugtest.Ports{Events: append(slices.Clone(song), liveNote(1, 69, 12000, 36000)...)})
	for c := range alone.Out {
		if !slices.Equal(alone.Out[c], both.Out[c]) {
			t.Fatalf("a live note changed the main output's channel %d", c)
		}
	}
	if got := rms(both.LiveOut[0][:12000]); got != 0 {
		t.Errorf("before the live note, the live output sounds at %.5f", got)
	}
	// The lead's lowest saws play an octave down: A4 sounds at 220 Hz.
	if got := hz(both.LiveOut[0][18000:30000], 48000); math.Abs(got-220) > 3 {
		t.Errorf("the live A4 sounds at %.1f Hz on the live output, not 220", got)
	}
	if got := rms(alone.LiveOut[0]); got != 0 {
		t.Errorf("with no live note, the live output sounds at %.5f", got)
	}
}

func TestLivePartKeepsItsOwnBendAndPedal(t *testing.T) {
	n := 48000
	// The song's pedal goes down and its note 1 bends up an octave; the
	// live note 1, let go at 9600, takes neither.
	events := append(liveNote(1, 69, 0, 9600),
		plug.Event{Frame: 0, Kind: plug.Pedal, Value: 1},
		plug.Event{Frame: 100, Kind: plug.Expression, Note: 1, Expr: plug.Bend, Value: 12})
	res := driveLive(t, n, plugtest.Ports{Events: events})
	if got := hz(res.LiveOut[0][2400:9600], 48000); math.Abs(got-220) > 3 {
		t.Errorf("the song's bend moved the live A4 to %.1f Hz, not 220", got)
	}
	if got := rms(res.LiveOut[0][n-4800:]); got > 1e-4 {
		t.Errorf("the song's pedal held the live note let go: it sounds at %.5f a second on", got)
	}
	// The live part's own pedal does hold it.
	held := driveLive(t, n, plugtest.Ports{Events: append(liveNote(1, 69, 0, 9600),
		plug.Event{Frame: 0, Kind: plug.Pedal, Value: 1, Live: true})})
	if got := rms(held.LiveOut[0][n-4800:]); got < 0.01 {
		t.Errorf("the live pedal let the live note go: it sounds at %.5f", got)
	}
}

func TestResetLeavesTheLivePart(t *testing.T) {
	n := 48000
	reset := 47 * 512
	events := append(note(1, 57, 0, n-1), liveNote(1, 69, 0, n-1)...)
	plain := driveLive(t, n, plugtest.Ports{Events: events})
	sought := driveLive(t, n, plugtest.Ports{Events: events, ResetAt: []int{reset}})
	if !slices.Equal(plain.LiveOut[0], sought.LiveOut[0]) {
		t.Error("a Reset broke the held live note's sound")
	}
	if got := rms(sought.LiveOut[0][reset:]); got < 0.01 {
		t.Errorf("after Reset, the held live note sounds at %.5f", got)
	}
	if got := rms(sought.Out[0][reset+16:]); got != 0 {
		t.Errorf("after Reset, the song's note sounds on at %.5f", got)
	}
}

func TestLivePartRunsItsOwnEffects(t *testing.T) {
	// Hej Synth! has no reverb of its own: a note's tail is its release
	// and the sound ringing out of a patch picked before. Both run on
	// the live part's own resampler.
	n := 48000
	off := 9600
	p := synthOf(t, "Jupiter brass lead")
	pr := p.Processor()
	res := plugtest.Drive(t, &Descriptor, pr, liveSetup, n, plugtest.Ports{Events: liveNote(1, 69, 0, off)})
	if got := rms(res.LiveOut[0][off+256 : off+2400]); got < 1e-3 {
		t.Errorf("the live note's release is silent on the live output, at %.5f", got)
	}
	if got := rms(res.Out[0]); got != 0 {
		t.Errorf("the live note sounds on the main output, at %.5f", got)
	}
	// With the tail gone, the live part's own sound stops running.
	if pr.(*processor).live {
		t.Error("a second after its release, the live part still runs")
	}
}

func TestWithNoLiveOutputTheSynthPlaysAsBefore(t *testing.T) {
	// A host that connects no live output sees no change: the main output
	// is the same, sample for sample, as with one connected and unused.
	n := 24064
	events := append(note(1, 57, 0, 12000), plug.Event{Frame: 6000, Kind: plug.Pedal, Value: 1})
	ports := plugtest.Ports{Events: events, ResetAt: []int{18432}}
	with := driveLive(t, n, ports)
	d := Descriptor
	d.LivePart = false
	without := plugtest.Drive(t, &d, synthOf(t, "Jupiter brass lead").Processor(), liveSetup, n, ports)
	if without.LiveOut != nil {
		t.Fatal("the live output was connected")
	}
	if !slices.Equal(with.Out[0], without.Out[0]) || !slices.Equal(with.Out[1], without.Out[1]) {
		t.Error("connecting the live output changed the main output")
	}
}

func TestPlayingLiveAllocatesNothing(t *testing.T) {
	p := synthOf(t, "Jupiter brass lead")
	pr := p.Processor()
	events := append(note(1, 57, 0, 4000), liveNote(1, 69, 1000, 5000)...)
	n := plugtest.AllocsUnder(t, "plugsynth.(*processor).Process", func() {
		plugtest.Drive(t, &Descriptor, pr, liveSetup, 9600, plugtest.Ports{Events: events, ResetAt: []int{4608}})
	})
	if n != 0 {
		t.Errorf("playing the live part allocated %d times", n)
	}
}

func TestAMonoPatchPlaysEachPartsOwnLine(t *testing.T) {
	// The acid line is mono, with glide and slide: each part keeps its own
	// voice, so neither line glides from, or cuts, the other's notes.
	n := 16384
	song := append(note(1, 45, 0, 6000), note(2, 52, 4000, 12000)...)
	live := append(liveNote(1, 57, 2000, 8000), liveNote(2, 60, 7000, 14000)...)
	drive := func(events []plug.Event) plugtest.Result {
		slices.SortStableFunc(events, func(a, b plug.Event) int { return a.Frame - b.Frame })
		return plugtest.Drive(t, &Descriptor, synthOf(t, "Acid line").Processor(), liveSetup, n, plugtest.Ports{Events: events})
	}
	songAlone := drive(slices.Clone(song))
	liveAlone := drive(slices.Clone(live))
	both := drive(append(slices.Clone(song), live...))
	for c := range both.Out {
		if !slices.Equal(both.Out[c], songAlone.Out[c]) {
			t.Errorf("the live line changed the song's line on the main output's channel %d", c)
		}
		if !slices.Equal(both.LiveOut[c], liveAlone.LiveOut[c]) {
			t.Errorf("the song's line changed the live line on the live output's channel %d", c)
		}
	}
	if rms(both.LiveOut[0][2400:8000]) < 0.01 || rms(both.Out[0][2400:8000]) < 0.01 {
		t.Error("the two lines do not both sound")
	}
}

func TestAHeldKeyKeepsTheLivePartWhenASongNoteTakesItsVoice(t *testing.T) {
	// The lead has four voices: the fourth song note takes the held key's.
	// The key is still down, so the live part runs on: where the patch
	// changes, the key plays again, on the live output.
	n := 24576
	// The key is never let go within the run.
	events := liveNote(1, 69, 0, n-1)[:1]
	for i := range int32(4) {
		events = append(events, note(i+2, 57+float64(i)*3, 2000, n-1)...)
	}
	slices.SortStableFunc(events, func(a, b plug.Event) int { return a.Frame - b.Frame })
	pr := synthOf(t, "Jupiter brass lead").Processor()
	res := plugtest.Drive(t, &Descriptor, pr, liveSetup, n, plugtest.Ports{Events: events})
	if got := rms(res.LiveOut[0][n-4800:]); got != 0 {
		t.Fatalf("the song took no voice from the held key: it sounds at %.5f", got)
	}
	if !pr.(*processor).live {
		t.Error("the live part stopped while its key is held")
	}
}
