// Package tracker reads and plays tracker modules: the MOD files of
// ProTracker and the Amiga, and the XM files of FastTracker 2. A module
// carries its own sampled instruments, and patterns of notes and
// effects that play them, row by row, channel by channel.
//
//	m, err := tracker.Read(file)
//	p := tracker.NewPlayer(m)
//	mix.Play(p, audio.Options{})
//
// A [Player] plays through a [synth.Bus], so a module has the same
// mixing desk as a General MIDI file: each channel's strip, the
// effects, and the master's equaliser, compressor and limiter.
package tracker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// A Module is a tracker module, read.
type Module struct {
	// Title is the module's name, Format MOD or XM, and Tracker the
	// program that made it, as far as the file says.
	Title, Format, Tracker string
	// Channels is how many channels its patterns have.
	Channels int
	// Orders are the patterns in the order they play, and Restart the
	// order the song goes back to once it ends.
	Orders  []int
	Restart int
	// Patterns are its patterns.
	Patterns []Pattern
	// Instruments are its instruments, from 1 as the patterns number
	// them: Instruments[0] is instrument 1.
	Instruments []Instrument
	// Speed is how many ticks a row lasts to begin with, and BPM how
	// fast the ticks go: a tick lasts 2.5/BPM seconds.
	Speed, BPM int
	// Linear says the module slides its pitches in a linear table, as
	// an XM may, rather than by the Amiga's periods.
	Linear bool
	// Pan is each channel's place between the speakers to begin with,
	// from 0, left, to 255, right.
	Pan []int
	// VBlank says the module is timed by the Amiga's vertical blank, 50
	// ticks a second, as the oldest MODs are: every Fxx then sets the
	// speed, and the BPM stays at 125.
	VBlank bool
	// clock is what an Amiga period divides into to give a sample's
	// rate, in FastTracker's periods, four of the Amiga's.
	clock float64
	// OctaveShift is how many octaves a tracker of the format shows its
	// notes off from FastTracker's: -2 for ProTracker, whose middle C is
	// C-2.
	OctaveShift int
}

// A Pattern is rows of cells, a cell for each channel in each row.
type Pattern struct {
	Rows  int
	Cells []Cell
}

// Cell returns the cell of channel ch in row.
func (p *Pattern) Cell(row, ch, channels int) Cell { return p.Cells[row*channels+ch] }

// A Cell is a channel's part of a row: a note, an instrument, a volume
// and an effect, each of which may be missing.
type Cell struct {
	// Note is 1 to 96, C-0 to B-7, 97 to let go of the note, or 0 for
	// none.
	Note uint8
	// Instrument is 1 or more, or 0 for none.
	Instrument uint8
	// Volume is an XM's volume column, as the file has it, or 0 for
	// none.
	Volume uint8
	// Effect and Param are the effect, 0 to 35, as FastTracker numbers
	// them, and its parameter. Effect 0 with Param 0 is none.
	Effect, Param uint8
}

// KeyOff is the note that lets go of the note playing.
const KeyOff = 97

// An Instrument is samples and how they play: which sample each note
// plays, and envelopes of volume and panning.
type Instrument struct {
	Name    string
	Samples []Sample
	// Keymap is the sample each note plays, 0 to 95.
	Keymap [96]uint8
	// Vol and Pan are its envelopes.
	Vol, Pan Envelope
	// Fadeout is how fast its notes fade once let go: so much of 65536
	// a tick.
	Fadeout int
	// VibType, VibSweep, VibDepth and VibRate are its automatic vibrato.
	VibType, VibSweep, VibDepth, VibRate int
}

// An Envelope moves a note's volume or panning as it plays: through
// Points, of tick and value, 0 to 64; holding at Sustain while the note
// is held, and going round from LoopEnd to LoopStart, where On and the
// flags say.
type Envelope struct {
	Points                        [][2]int
	On, Sustain, Loop             bool
	SustainAt, LoopStart, LoopEnd int
}

// A Sample is a sound of an instrument.
type Sample struct {
	Name string
	// Data is its sound, from -1 to 1.
	Data []float32
	// LoopStart and LoopLen are its loop, in frames, and Loop the kind:
	// 0 none, 1 forward, 2 back and forth.
	LoopStart, LoopLen, Loop int
	// Volume is its level, 0 to 64; Finetune moves its pitch by
	// 128ths of a semitone; Pan is its place, 0 to 255; and RelNote moves
	// its notes by semitones.
	Volume, Finetune, Pan, RelNote int
	// Bits says the file held it in 16 bits rather than 8.
	Bits int
}

// Read reads a module: an XM, or a MOD, of 4 to 32 channels, or of 15
// samples, as the Soundtracker made.
func Read(r io.Reader) (*Module, error) {
	b, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(b, []byte("Extended Module:")):
		return readXM(b)
	case len(b) >= 1084:
		if m, err := readMOD(b); err == nil {
			return m, nil
		} else if !errors.Is(err, errNotMOD) {
			return nil, err
		}
	}
	if m, err := readMOD15(b); err == nil {
		return m, nil
	}
	return nil, errors.New("tracker: not a MOD or an XM file")
}

var errNotMOD = errors.New("tracker: no MOD signature")

// reader reads a file's fields, little-endian, and runs out quietly:
// past the end, every field is zero and ok is false.
type reader struct {
	b  []byte
	at int
	ok bool
}

func (r *reader) take(n int) []byte {
	if n < 0 || r.at+n > len(r.b) {
		r.ok = false
		r.at = len(r.b)
		return make([]byte, max(n, 0))
	}
	s := r.b[r.at : r.at+n]
	r.at += n
	return s
}

func (r *reader) u8() int  { return int(r.take(1)[0]) }
func (r *reader) u16() int { s := r.take(2); return int(s[0]) | int(s[1])<<8 }
func (r *reader) u32() int {
	s := r.take(4)
	return int(s[0]) | int(s[1])<<8 | int(s[2])<<16 | int(s[3])<<24
}
func (r *reader) str(n int) string { return cleanName(r.take(n)) }

// cleanName returns a name as a file holds it: to its first zero, its
// unprintable bytes as spaces, and trimmed at its end.
func cleanName(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		if c < 32 || c > 126 {
			c = ' '
		}
		out = append(out, rune(c))
	}
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// NoteName names a note as a tracker shows it, as C-4 or C#4, shifted
// by shift octaves: "===" for a key off, and "---" for none.
func NoteName(note uint8, shift int) string {
	switch {
	case note == 0:
		return "---"
	case note == KeyOff:
		return "==="
	case note > 96:
		return "???"
	}
	names := [12]string{"C-", "C#", "D-", "D#", "E-", "F-", "F#", "G-", "G#", "A-", "A#", "B-"}
	n := int(note) - 1
	return fmt.Sprintf("%s%d", names[n%12], max(n/12+shift, 0))
}
