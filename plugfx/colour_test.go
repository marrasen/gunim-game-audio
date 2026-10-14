package plugfx

import (
	"bytes"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// chains are the chains the tests run, one after another: as a user
// adds a unit, reorders, adds and removes.
var chains = [][]Unit{
	{{Kind: Chorus}},
	{{Kind: Chorus}, {Kind: Distortion, Slot: 1}},
	{{Kind: Distortion, Slot: 1}, {Kind: Chorus}},
	{{Kind: Distortion, Slot: 1}, {Kind: Phaser, Slot: 2}, {Kind: Chorus}},
	{{Kind: Phaser, Slot: 2}, {Kind: Chorus}},
	{{Kind: Chorus}, {Kind: Phaser, Slot: 2}},
	{},
	{{Kind: Distortion, Slot: 1}},
}

// swapEvery is how many frames apart the tests change the chain.
const swapEvery = 24 * 256

func colourWith(t *testing.T, units []Unit) plug.Plugin {
	t.Helper()
	p := ColourDescriptor.New(nil)
	if err := p.SetState(ColourState{Units: units}.Encode()); err != nil {
		t.Fatal(err)
	}
	return p
}

// colourChanges are the units' settings the tests run: a Juno's chorus
// II, and an amp driven hard.
var colourChanges = []plug.Change{
	{ID: UnitParamID(0, ChorusModeParam), Value: 2}, {ID: UnitParamID(0, ChorusMixParam), Value: 100},
	{ID: UnitParamID(1, DistortTypeParam), Value: 1}, {ID: UnitParamID(1, DistortDriveParam), Value: 80},
}

func TestColoursChainChangesMidPlayWithoutAClick(t *testing.T) {
	in := sine(swapEvery*(len(chains)+1), 110, 0.3)
	// How far a frame steps from the one before, at most, with each
	// chain held.
	var held float32
	for _, units := range chains {
		out := rig{pr: colourWith(t, units).Processor(), d: &ColourDescriptor, s: setup}.run(t, in, colourChanges...)
		held = max(held, biggestStep(out[0][4800:]), biggestStep(out[1][4800:]))
	}
	p := colourWith(t, chains[0])
	r := rig{pr: p.Processor(), d: &ColourDescriptor, s: setup}
	r.before = func(at int) {
		if at > 0 && at%swapEvery == 0 && at/swapEvery < len(chains) {
			if err := p.SetState(ColourState{Units: chains[at/swapEvery]}.Encode()); err != nil {
				t.Error(err)
			}
		}
	}
	out := r.run(t, in, colourChanges...)
	for i := 1; i < len(chains); i++ {
		at := i * swapEvery
		for ch := range out {
			if got := biggestStep(out[ch][at-256 : at+2048]); got > held*1.2 {
				t.Errorf("going to chain %d, %v, channel %d steps %g in a frame, where a chain held steps %g at most",
					i, chains[i], ch, got, held)
			}
		}
	}
}

func TestAUnitCarriedIntoANewChainSoundsOn(t *testing.T) {
	// A chorus alone, and the same chorus with a phaser added at 0% mix
	// after it: past the crossover, the two sound alike, sweep and all,
	// as the chorus carries on where it was.
	in := noise(4*swapEvery, 4*swapEvery)
	changes := slices.Concat(colourChanges, []plug.Change{{ID: UnitParamID(2, PhaserMixParam), Value: 0}})
	alone := rig{pr: colourWith(t, chains[0]).Processor(), d: &ColourDescriptor, s: setup}.run(t, in, changes...)
	p := colourWith(t, chains[0])
	r := rig{pr: p.Processor(), d: &ColourDescriptor, s: setup}
	r.before = func(at int) {
		if at == swapEvery {
			_ = p.SetState(ColourState{Units: []Unit{{Kind: Chorus}, {Kind: Phaser, Slot: 2}}}.Encode())
		}
	}
	out := r.run(t, in, changes...)
	for ch := range out {
		for i := swapEvery + 1024; i < len(in[0]); i++ {
			if d := out[ch][i] - alone[ch][i]; d > 1e-6 || d < -1e-6 {
				t.Fatalf("channel %d frame %d: the chorus sounds %g, where it sounded %g without the phaser",
					ch, i, out[ch][i], alone[ch][i])
			}
		}
	}
}

func TestColoursStateRoundTrips(t *testing.T) {
	p := ColourDescriptor.New(nil)
	if got := p.State(); !bytes.Equal(got, ColourState{Units: defaultUnits()}.Encode()) {
		t.Errorf("a new Colour's state is %s, not a Juno's chorus", got)
	}
	for _, units := range chains {
		b := ColourState{Units: units}.Encode()
		if err := p.SetState(b); err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		if got := p.State(); !bytes.Equal(got, b) {
			t.Errorf("the state set is %s, and comes back %s", b, got)
		}
		q := ColourDescriptor.New(nil)
		if err := q.SetState(p.State()); err != nil || !bytes.Equal(q.State(), b) {
			t.Errorf("a second Colour takes the first's state %s as %s, %v", b, q.State(), err)
		}
	}
	if err := p.SetState(nil); err != nil || !bytes.Equal(p.State(), ColourState{Units: defaultUnits()}.Encode()) {
		t.Errorf("an empty state sets %s, %v, not a Juno's chorus", p.State(), err)
	}
	if got := string(ColourState{Units: chains[2]}.Encode()); got != `{"Units":[{"Kind":"distortion","Slot":1},{"Kind":"chorus"}]}` {
		t.Errorf("the state writes %s", got)
	}
}

func TestColourTurnsAwayAStateItCannotRun(t *testing.T) {
	p := colourWith(t, chains[3])
	was := p.State()
	for _, b := range []string{
		`{"Units":[{"Kind":"flanger"}]}`,
		`{"Units":[{"Kind":"chorus"},{"Kind":"phaser"}]}`,
		`{"Units":[{"Kind":"chorus","Slot":6}]}`,
		`{"Units":[{"Kind":"chorus","Slot":-1}]}`,
		`{"Units":[{"Kind":"chorus"},{"Kind":"chorus","Slot":1},{"Kind":"chorus","Slot":2},{"Kind":"chorus","Slot":3},` +
			`{"Kind":"chorus","Slot":4},{"Kind":"chorus","Slot":5},{"Kind":"chorus","Slot":5}]}`,
		`{"Units":`,
	} {
		if err := p.SetState([]byte(b)); err == nil {
			t.Errorf("Colour takes %s", b)
		}
		if got := p.State(); !bytes.Equal(got, was) {
			t.Errorf("after %s, the state is %s, not %s as it was", b, got, was)
		}
	}
}

func TestTheChainChangesSafelyWhileBlocksRun(t *testing.T) {
	// Run with -race: SetState on one goroutine, blocks on another.
	p := colourWith(t, chains[1])
	pr := p.Processor()
	pr.Prepare(setup)
	in := noise(setup.MaxBlock, setup.MaxBlock)
	b := &plug.Block{Frames: setup.MaxBlock, In: in, Out: [][]float32{make([]float32, setup.MaxBlock),
		make([]float32, setup.MaxBlock)}}
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 200 {
			_ = p.SetState(ColourState{Units: chains[i%len(chains)]}.Encode())
			_ = pr.Tail()
		}
	})
	for range 400 {
		pr.Process(b)
	}
	wg.Wait()
}

func TestTheChainsUnitsEditInPlace(t *testing.T) {
	units := []Unit{{Kind: Chorus}}
	units, ok := withUnit(units, Phaser)
	if !ok || units[1] != (Unit{Kind: Phaser, Slot: 1}) {
		t.Fatalf("a phaser added goes in as %v", units)
	}
	units = withoutUnit(units, 0)
	units, _ = withUnit(units, Distortion)
	if units[1] != (Unit{Kind: Distortion, Slot: 0}) {
		t.Errorf("a distortion added takes slot %d, where the first free is 0", units[1].Slot)
	}
	if got := moved(units, 1, 0); got[0].Kind != Distortion || got[1].Kind != Phaser {
		t.Errorf("moving the second first makes %v", got)
	}
	for range MaxUnits {
		units, _ = withUnit(units, Chorus)
	}
	if len(units) != MaxUnits {
		t.Errorf("the chain grows to %d units, past %d", len(units), MaxUnits)
	}
	_ = synth.Rate
}

// rms returns the root mean square of x, in decibels.
func rms(x ...[]float32) float64 {
	var sum float64
	n := 0
	for _, ch := range x {
		for _, v := range ch {
			sum += float64(v) * float64(v)
		}
		n += len(ch)
	}
	return 10 * math.Log10(sum/float64(n)+1e-30)
}

// distorted runs in through a lone distortion of kind at drive, with
// more changes after, and returns what comes out.
func distorted(t *testing.T, in [][]float32, kind int, drive float64, more ...plug.Change) [][]float32 {
	t.Helper()
	changes := append([]plug.Change{{ID: UnitParamID(0, DistortTypeParam), Value: float64(kind)},
		{ID: UnitParamID(0, DistortDriveParam), Value: drive}}, more...)
	return rig{pr: colourWith(t, []Unit{{Kind: Distortion}}).Processor(), d: &ColourDescriptor, s: setup}.run(t, in,
		changes...)
}

func TestAutoGainKeepsTheDistortionAsLoudAsItsInput(t *testing.T) {
	// A second of each, measured past the first half second. Lower, the
	// amp's cabinet cuts more of a sine than Auto gain makes up at
	// most, at a low drive.
	const settle = 24000
	for _, hz := range []float64{220, 440} {
		for _, level := range []float64{0.25, 0.05} {
			in := sine(48000, hz, level)
			want := rms(in[0][settle:], in[1][settle:])
			for kind, name := range DistortTypeNames {
				for _, drive := range []float64{10, 40, 70, 100} {
					out := distorted(t, in, kind, drive)
					if got := rms(out[0][settle:], out[1][settle:]); math.Abs(got-want) > 2 {
						t.Errorf("%s at %g%%, a sine of %g Hz at %.1f dB RMS comes out at %.1f dB", name, drive, hz, want,
							got)
					}
				}
			}
		}
	}
}

func TestAutoGainBringsTheFirstNoteInQuietly(t *testing.T) {
	// A note out of silence comes in no louder than it settles, give
	// or take a little, though the gain has had nothing to measure.
	in := sine(48000, 110, 0.25)
	for i := range 4800 {
		in[0][i], in[1][i] = 0, 0
	}
	for kind, name := range DistortTypeNames {
		out := distorted(t, in, kind, 100)
		settled := peak(out[0][24000:], out[1][24000:])
		if got := peak(out[0][:7200], out[1][:7200]); got > settled*1.5 {
			t.Errorf("%s: the note comes in at %g, and settles at %g", name, got, settled)
		}
	}
}

func TestAutoGainKeepsSilenceSilent(t *testing.T) {
	// Silence, and then a whisper at -90 dBFS after a note, stay as
	// quiet as they come in.
	in := sine(48000, 110, 0.25)
	for i := 12000; i < len(in[0]); i++ {
		v := float32(0)
		if i >= 36000 {
			v = in[0][i] * 1.3e-4
		}
		in[0][i], in[1][i] = v, v
	}
	for kind, name := range DistortTypeNames {
		silent := distorted(t, [][]float32{make([]float32, 24000), make([]float32, 24000)}, kind, 100)
		if p := peak(silent...); p != 0 {
			t.Errorf("%s: silence comes out at %g", name, p)
		}
		out := distorted(t, in, kind, 100)
		if got, settled := peak(out[0][36000:], out[1][36000:]), peak(out[0][6000:12000]); got > settled*0.01 {
			t.Errorf("%s: a whisper after a note comes out at %g, where the note was %g", name, got, settled)
		}
	}
}

func TestOutputTurnsTheDistortionUpAndDown(t *testing.T) {
	in := sine(48000, 110, 0.25)
	base := distorted(t, in, 0, 40)
	want := rms(base[0][24000:], base[1][24000:])
	for _, db := range []float64{-24, -6, 6, 12} {
		out := distorted(t, in, 0, 40, plug.Change{ID: UnitParamID(0, DistortOutputParam), Value: db})
		if got := rms(out[0][24000:], out[1][24000:]) - want; math.Abs(got-db) > 0.1 {
			t.Errorf("Output at %+g dB turns the sound by %+.2f dB", db, got)
		}
	}
}

func TestAutoGainRendersTheSameEachTime(t *testing.T) {
	in := noise(48000, 30000)
	changes := []plug.Change{{ID: UnitParamID(0, DistortDriveParam), Value: 30},
		{Frame: 9000, ID: UnitParamID(0, DistortDriveParam), Value: 90},
		{Frame: 20000, ID: UnitParamID(0, DistortAutoParam), Value: 0},
		{Frame: 26000, ID: UnitParamID(0, DistortAutoParam), Value: 1}}
	first := distorted(t, in, 1, 50, changes...)
	for range 3 {
		again := distorted(t, in, 1, 50, changes...)
		for ch := range again {
			if !slices.Equal(first[ch], again[ch]) {
				t.Fatalf("channel %d renders differently the second time", ch)
			}
		}
	}
}

func TestAutoGainTurnsOnAndOffWithoutAClick(t *testing.T) {
	in := sine(48000, 110, 0.25)
	held := distorted(t, in, 0, 70)
	off := distorted(t, in, 0, 70, plug.Change{ID: UnitParamID(0, DistortAutoParam), Value: 0})
	step := max(biggestStep(held[0][4800:]), biggestStep(off[0][4800:]))
	out := distorted(t, in, 0, 70, plug.Change{Frame: 12000, ID: UnitParamID(0, DistortAutoParam), Value: 0},
		plug.Change{Frame: 30000, ID: UnitParamID(0, DistortAutoParam), Value: 1})
	if got := biggestStep(out[0][4800:]); got > step*1.2 {
		t.Errorf("turning Auto gain off and on steps %g in a frame, where it steps %g at most held", got, step)
	}
}
