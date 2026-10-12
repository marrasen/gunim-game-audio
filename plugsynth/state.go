package plugsynth

import (
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/marrasen/gunim-game-audio/synth"
)

// State is the plugin's state, as [plug.Plugin]'s State writes it: the
// sound playing, by its name and where it comes from, and its whole
// patch, so a song keeps its sound as it was picked, whatever becomes
// of the library. It is JSON:
//
//	{"Bank":"Library","Group":"Pads & strings","Name":"Juno pad","Patch":{"Osc":[…],…}}
//
// The knobs are parameters, beside the state: they turn the patch from
// how it is made.
type State struct {
	// Bank, Group and Name are where the browser lists the sound, as
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
	// patch: one that would fault as it plays is turned away here.
	if err := synth.CheckPatch(s.Patch); err != nil {
		return State{}, fmt.Errorf("plugsynth: %s: %w", s.Name, err)
	}
	return s, nil
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
