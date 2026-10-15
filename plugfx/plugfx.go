// Package plugfx is the engine's effects as plugins in gunim's [plug]
// format, for a host's sends and inserts: a reverb, a tempo-synced
// delay, and Colour, a chain of choruses, phasers and distortions the
// user builds. Each runs the effect a song runs, from package synth, at
// the host's own rate, so at 48 kHz it sounds as the song does, frame
// for frame. They register themselves as the package starts; import it
// for that:
//
//	import _ "github.com/marrasen/gunim-game-audio/plugfx"
package plugfx

import (
	"fmt"
	"math"

	"github.com/marrasen/gunim/audio/plug"
)

// glideMS is how long a mix or a width takes to glide where it is set,
// so a knob turned or automated moves smoothly.
const glideMS = 20

// percent writes a value in percent to show.
func percent(v float64) string { return fmt.Sprintf("%.0f%%", v) }

// millis writes a time in milliseconds to show.
func millis(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2f s", v/1000)
	}
	if v < 10 {
		return fmt.Sprintf("%.1f ms", v)
	}
	return fmt.Sprintf("%.0f ms", v)
}

// seconds writes a time in seconds to show.
func seconds(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.0f ms", v*1000)
	}
	return fmt.Sprintf("%.1f s", v)
}

// hertz writes a frequency to show.
func hertz(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.3g kHz", v/1000)
	}
	if v < 10 {
		return fmt.Sprintf("%.2f Hz", v)
	}
	return fmt.Sprintf("%.0f Hz", v)
}

// decibels writes a level in decibels to show.
func decibels(v float64) string {
	if math.Abs(v) < 0.05 {
		return "0 dB"
	}
	return fmt.Sprintf("%+.1f dB", v)
}

// onOff writes a switch to show.
func onOff(v float64) string {
	if v > 0.5 {
		return "On"
	}
	return "Off"
}

// sides returns a block's channels as left and right: the one channel
// twice where there is one, and nil where there is none.
func sides(chans [][]float32) (l, r []float32) {
	switch len(chans) {
	case 0:
		return nil, nil
	case 1:
		return chans[0], chans[0]
	default:
		return chans[0], chans[1]
	}
}

// segments is a processor that runs a block in segments, each from one
// change of a parameter to the next, so each change takes effect from
// its frame on.
type segments interface {
	take(c plug.Change)
	segment(b *plug.Block, from, to int)
}

// runSegments runs b through s, segment by segment.
func runSegments(b *plug.Block, s segments) {
	ci := 0
	for at := 0; at < b.Frames; {
		for ci < len(b.Changes) && b.Changes[ci].Frame <= at {
			s.take(b.Changes[ci])
			ci++
		}
		next := b.Frames
		if ci < len(b.Changes) {
			next = min(next, b.Changes[ci].Frame)
		}
		s.segment(b, at, next)
		at = next
	}
	for ; ci < len(b.Changes); ci++ {
		s.take(b.Changes[ci])
	}
}

// blend mixes an effect's sound into the sound it was fed: the dry
// sound times 1-mix and the wet times mix, the wet's sides scaled by
// width. The mix and the width glide where they are set.
type blend struct {
	mix, width, mixTo, widthTo float32
	glide                      float32
}

// prepare readies the blend for sound at rate frames a second.
func (m *blend) prepare(rate float64) {
	m.glide = float32(1 - math.Exp(-1/(glideMS/1000*rate)))
}

// jump puts the mix and the width where they are set at once.
func (m *blend) jump() { m.mix, m.width = m.mixTo, m.widthTo }

// run writes out's frames from index from up to index to, of the dry
// sound in and the wet in wl and wr. out may be in.
func (m *blend) run(in, out [][]float32, wl, wr []float32, from, to int) {
	inL, inR := sides(in)
	outL, outR := sides(out)
	if outL == nil {
		return
	}
	stereo := len(out) > 1
	for i := from; i < to; i++ {
		m.mix += (m.mixTo - m.mix) * m.glide
		m.width += (m.widthTo - m.width) * m.glide
		var dl, dr float32
		if inL != nil {
			dl, dr = inL[i], inR[i]
		}
		// Each side keeps (1+width)/2 of its own and takes (1-width)/2 of
		// the other's: as it was at 1, and mono at 0.
		near, far := (1+m.width)*0.5, (1-m.width)*0.5
		l := dl*(1-m.mix) + (wl[i]*near+wr[i]*far)*m.mix
		r := dr*(1-m.mix) + (wr[i]*near+wl[i]*far)*m.mix
		if stereo {
			outL[i], outR[i] = l, r
		} else {
			outL[i] = (l + r) * 0.5
		}
		for _, ch := range out[min(len(out), 2):] {
			ch[i] = 0
		}
	}
}

// feed copies the sound in's frames from index from up to index to
// into wl and wr, or silence where there is none.
func feed(in [][]float32, wl, wr []float32, from, to int) {
	inL, inR := sides(in)
	if inL == nil {
		clear(wl[from:to])
		clear(wr[from:to])
		return
	}
	copy(wl[from:to], inL[from:to])
	copy(wr[from:to], inR[from:to])
}
