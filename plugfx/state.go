package plugfx

import (
	"encoding/json/v2"
	"fmt"
	"slices"
)

// ColourState is Colour's state, as [plug.Plugin]'s State writes it: the
// chain's units, in the order the sound runs through them. It is JSON:
//
//	{"Units":[{"Kind":"distortion"},{"Kind":"chorus","Slot":1}]}
//
// Each unit's controls are parameters, beside the state: the unit in
// slot s uses those [UnitParamID] gives for s, which stay its own as the
// chain is reordered.
type ColourState struct {
	Units []Unit
}

// Unit is a unit of Colour's chain: its kind, one of [UnitKinds], and
// its slot, from 0 to [MaxUnits]-1, a slot of its own in the chain,
// which picks its parameters.
type Unit struct {
	Kind string
	Slot int
}

// wire is how the state is written: Go's field names as keys, zero
// values left out, and maps in key order, so a state writes the same
// twice.
var wire = json.JoinOptions(json.OmitZeroStructFields(true), json.Deterministic(true))

// Encode returns the state as JSON, as State writes it.
func (s ColourState) Encode() []byte {
	if s.Units == nil {
		s.Units = []Unit{}
	}
	b, err := json.Marshal(s, wire)
	if err != nil {
		// Names and numbers always marshal.
		panic("plugfx: " + err.Error())
	}
	return b
}

// DecodeColourState reads a state as State writes it, and checks it:
// each unit of a kind known, in a slot of its own.
func DecodeColourState(b []byte) (ColourState, error) {
	var s ColourState
	if err := json.Unmarshal(b, &s, wire); err != nil {
		return ColourState{}, fmt.Errorf("plugfx: reading Colour's state: %w", err)
	}
	if len(s.Units) > MaxUnits {
		return ColourState{}, fmt.Errorf("plugfx: Colour's state holds %d units, of %d at most", len(s.Units), MaxUnits)
	}
	var used [MaxUnits]bool
	for _, u := range s.Units {
		if !slices.Contains(UnitKinds, u.Kind) {
			return ColourState{}, fmt.Errorf("plugfx: Colour has no unit of kind %q", u.Kind)
		}
		if u.Slot < 0 || u.Slot >= MaxUnits || used[u.Slot] {
			return ColourState{}, fmt.Errorf("plugfx: Colour's state puts two units in slot %d, or one outside", u.Slot)
		}
		used[u.Slot] = true
	}
	return s, nil
}

// withUnit returns units with a unit of kind after them, in the first
// slot free, and whether there was room.
func withUnit(units []Unit, kind string) ([]Unit, bool) {
	if len(units) >= MaxUnits {
		return units, false
	}
	for slot := range MaxUnits {
		if !slices.ContainsFunc(units, func(u Unit) bool { return u.Slot == slot }) {
			return append(slices.Clone(units), Unit{Kind: kind, Slot: slot}), true
		}
	}
	return units, false
}

// withoutUnit returns units without the one at i.
func withoutUnit(units []Unit, i int) []Unit { return slices.Delete(slices.Clone(units), i, i+1) }

// moved returns units with the one at i moved to j.
func moved(units []Unit, i, j int) []Unit {
	out := slices.Clone(units)
	u := out[i]
	out = slices.Delete(out, i, i+1)
	return slices.Insert(out, j, u)
}
