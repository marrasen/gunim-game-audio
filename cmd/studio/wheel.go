package main

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The wheel scrolls the page, and turns a knob, a fader or a selector
// only when it is meant to, so looking round a page does not change it.
// A gesture is the wheel's events less than wheelGap apart, and belongs
// to what its first event lands on: a gesture begun on the page scrolls
// the page to its end, though knobs pass under the pointer. A control
// takes a gesture only where the pointer moved onto it, since the page
// last scrolled, and rested there wheelDwell before the wheel turned, or
// it was pressed or dragged within wheelTouched: a knob the page
// scrolled under a pointer at rest is not taken for one meant;
// Ctrl with the wheel turns it always. A finger's scroll on a touch
// screen never turns one.
const (
	wheelGap     = 500 * time.Millisecond
	wheelDwell   = 400 * time.Millisecond
	wheelTouched = 3 * time.Second
)

// wheelGuard is the wheel's state: when its last event came and when
// the gesture began, what the gesture belongs to, nil for the page,
// when the page last scrolled, and when the pointer last moved; touched
// is the control last pressed, and when.
type wheelGuard struct {
	last, begun time.Time
	owner       any
	paged       time.Time
	moved       time.Time
	at          geom.Point
	touched     any
	touchedAt   time.Time
}

// wheel is the studio's one guard; it has the one window.
var wheel wheelGuard

// overhear hears each move and wheel's turn in the window, before what
// is under the pointer takes it.
func (w *wheelGuard) overhear(e input.Event) {
	switch e := e.(type) {
	case input.PointerMove:
		if e.Pos != w.at {
			w.at, w.moved = e.Pos, eventTime(e.Time)
		}
	case input.Scroll:
		t := eventTime(e.Time)
		if t.Sub(w.last) > wheelGap {
			if w.owner == nil && !w.last.IsZero() {
				// The gesture before scrolled the page, to its last event.
				w.paged = w.last
			}
			w.begun, w.owner = t, nil
		}
		w.last = t
	}
}

// touch says control c was pressed or dragged now.
func (w *wheelGuard) touch(c any) { w.touched, w.touchedAt = c, time.Now() }

// allows reports whether the wheel's event e, which landed on control
// c, turns it, rather than scrolling the page.
func (w *wheelGuard) allows(c any, e input.Scroll) bool {
	if e.Touch {
		return false
	}
	if e.Mods.Has(input.ModControl) {
		w.owner = c
		return true
	}
	t := eventTime(e.Time)
	if w.owner == nil && t.Equal(w.begun) {
		// The gesture's first event: c takes it where the pointer
		// rested on it, or it was just touched.
		rested := w.moved.After(w.paged) && w.begun.Sub(w.moved) >= wheelDwell
		touched := w.touched == c && time.Since(w.touchedAt) < wheelTouched
		if rested || touched {
			w.owner = c
		}
	}
	return w.owner == c
}

// eventTime returns an event's time, or now where it has none.
func eventTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// fader is the mixer's fader, which takes the wheel only as the guard
// allows.
type fader struct{ *audioui.Fader }

func newFader(value func() float32, set func(float32, *gunim.UI) gunim.Intent) *fader {
	return &fader{Fader: audioui.NewFader(value, set)}
}

func (f *fader) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.Scroll:
		if !wheel.allows(f, e) {
			return false
		}
	case input.PointerDown:
		wheel.touch(f)
	}
	return f.Fader.Handle(e, u)
}
