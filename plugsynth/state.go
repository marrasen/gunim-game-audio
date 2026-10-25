package plugsynth

import (
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/marrasen/gunim-game-audio/synth"
)

// State is the plugin's state, as [plug.Plugin]'s State writes it: the
// sound playing, by its name and where it comes from, and its whole
// patch, so a song keeps its sound as it was picked, whatever becomes
// of the library, but for a kit [DecodeState] moves to the new drums.
// It is JSON:
//
//	{"Bank":"Library","Group":"Pads & strings","Name":"Juno pad","Patch":{"Osc":[…],…}}
//
// The knobs are parameters, beside the state: they turn the patch from
// how it is made.
type State struct {
	// Bank, Group and Name are where Presets lists the sound, as
	// [Sound] has them.
	Bank, Group, Name string
	// Patch is the patch the plugin plays.
	Patch *synth.Patch
}

// wire is how the state is written: Go's field names as keys, zero
// values left out, and maps in key order, so a state writes the same
// twice.
var wire = json.JoinOptions(json.OmitZeroStructFields(true), json.Deterministic(true))

// Encode returns the state as JSON, as State writes it.
func (s State) Encode() []byte {
	b, err := json.Marshal(s, wire)
	if err != nil {
		// A patch is numbers, names and lists of them, which always
		// marshal.
		panic("plugsynth: " + err.Error())
	}
	return b
}

// errNoPatch is the error of a state that holds no patch.
var errNoPatch = errors.New("plugsynth: the state holds no patch")

// DecodeState reads a state as State writes it. The patch it holds is
// the caller's own.
func DecodeState(b []byte) (State, error) {
	var s State
	if err := json.Unmarshal(b, &s, wire); err != nil {
		return State{}, fmt.Errorf("plugsynth: reading the state: %w", err)
	}
	if s.Patch == nil {
		return State{}, errNoPatch
	}
	// A state from outside, as a song a user edited, may hold any
	// patch: one that would fault as it plays is turned away here. A
	// state checked before, or a sound of the module's own, plays as it
	// is: a sound picked is read twice, for the views and for the sound.
	if !checked.has(b) {
		if err := synth.CheckPatch(s.Patch); err != nil {
			return State{}, fmt.Errorf("plugsynth: %s: %w", s.Name, err)
		}
		checked.add(b)
	}
	return moved(s), nil
}

// movedKits are the library's drum kits that moved from the old drums to the modelled and circuit ones.
var movedKits = []string{"TR-808", "Linn style", "Hi-NRG kit", "Simmons kit", "TR-909 techno", "Industrial kit", "Noise pop kit"}

// moved returns s with the library's kit as it is now in place of a moved kit's that still plays the old drums.
func moved(s State) State {
	if s.Bank != Library || !slices.Contains(movedKits, s.Name) || !oldDrums(s.Patch) {
		return s
	}
	for _, snd := range ownSounds() {
		if snd.Bank == Library && snd.Name == s.Name {
			s.Group, s.Patch = snd.Group, snd.Patch()
		}
	}
	return s
}

// oldDrums reports whether drum kit p plays only the old drums, as a kit names them without a type.
func oldDrums(p *synth.Patch) bool {
	if p.Kind != "drums" {
		return false
	}
	for _, d := range p.Kit {
		if d.Type != "" {
			return false
		}
	}
	return true
}

// checked are the states known to play, by their hashes: those the
// module's sounds write, and those CheckPatch passed.
var checked checkedStates

// checkedStates is a set of states, by their hashes, safe on any
// goroutine. It forgets them all once it holds maxChecked.
type checkedStates struct {
	mu sync.Mutex
	m  map[[sha256.Size]byte]bool
}

// maxChecked is the most states checked keeps.
const maxChecked = 4096

func (c *checkedStates) has(b []byte) bool {
	h := sha256.Sum256(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[h]
}

func (c *checkedStates) add(b []byte) {
	h := sha256.Sum256(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= maxChecked {
		c.m = map[[sha256.Size]byte]bool{}
	}
	c.m[h] = true
}

// clonePatch returns a copy of p that shares nothing with it.
func clonePatch(p *synth.Patch) *synth.Patch {
	b, err := json.Marshal(p, wire)
	if err != nil {
		panic("plugsynth: " + err.Error())
	}
	c := new(synth.Patch)
	if err := json.Unmarshal(b, c, wire); err != nil {
		panic("plugsynth: " + err.Error())
	}
	return c
}
