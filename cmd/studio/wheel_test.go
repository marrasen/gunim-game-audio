package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestAKnobScrolledUnderARestingPointerIsNotTurned(t *testing.T) {
	var w wheelGuard
	knob := new(int)
	t0 := time.Now()
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	turn := func(ms int, on any) bool {
		e := input.Scroll{Pos: geom.Pt(10, 10), Delta: geom.Pt(0, -40), Time: at(ms)}
		w.overhear(e)
		if on == nil {
			return false
		}
		return w.allows(on, e)
	}
	w.overhear(input.PointerMove{Pos: geom.Pt(10, 10), Time: at(0)})
	// The page scrolls under the resting pointer, until a knob is under
	// it.
	turn(1000, nil)
	turn(1100, nil)
	// A second later the wheel turns again, the pointer still: the knob
	// came under it by scrolling, and the page scrolls on.
	if turn(3000, knob) {
		t.Error("a knob the page scrolled under a resting pointer turned")
	}
	// The pointer moves onto it and rests: now it turns.
	w.overhear(input.PointerMove{Pos: geom.Pt(12, 10), Time: at(5000)})
	if !turn(5600, knob) {
		t.Error("a knob the pointer moved onto and rested on did not turn")
	}
}
