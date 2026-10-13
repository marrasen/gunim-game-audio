package plugfx

import (
	"bytes"
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
