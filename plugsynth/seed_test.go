package plugsynth

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// inTurn gives each NoteOn of events its place among them, as a host playing the music does.
func inTurn(events []plug.Event) []plug.Event {
	out := append([]plug.Event(nil), events...)
	turn := int64(0)
	for i := range out {
		if out[i].Kind == plug.NoteOn {
			turn++
			out[i].Turn, out[i].Group = turn, 3
		}
	}
	return out
}

// The sounds that draw on their random numbers as they play, and as a
// note starts: the phases of an analogue pad and a bass, a plucked
// string's burst, and a kit's noise.
var seededSounds = []string{"Juno pad", "Moog bass", "Harp pluck", "Supersaw pad", "TR-808"}

// A note of the music sounds the same whichever notes rang out before it, so taking one out leaves the rest as it was.
func TestANoteOwesNothingToTheNotesBefore(t *testing.T) {
	const at, end = 4 * 48000, 6 * 48000
	s := plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2}
	for _, name := range seededSounds {
		t.Run(name, func(t *testing.T) {
			last := note(3, 45, at, at+12000)
			with := inTurn(append(append(note(1, 45, 0, 4000), note(2, 50, 6000, 10000)...), last...))
			without := inTurn(last)
			a := plugtest.Play(t, synthOf(t, name).Processor(), s, end, params(), with...)[0]
			b := plugtest.Play(t, synthOf(t, name).Processor(), s, end, params(), without...)[0]
			heard := false
			for i := at; i < end; i++ {
				heard = heard || b[i] != 0
				if a[i] != b[i] {
					t.Fatalf("with notes before it, the note sounds otherwise from frame %d", i)
				}
			}
			if !heard {
				t.Fatal("no sound")
			}
		})
	}
}

// Notes alike at other places in the song aren't copies of each other: each starts at phases of its own.
func TestNotesAlikeStartApart(t *testing.T) {
	s := plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2}
	const a, b, n = 48000, 3 * 48000, 12000
	out := plugtest.Play(t, synthOf(t, "Juno pad").Processor(), s, 5*48000, params(),
		inTurn(append(note(1, 45, a, a+n), note(2, 45, b, b+n)...))...)[0]
	for i := range n {
		if out[a+i] != out[b+i] {
			return
		}
	}
	t.Error("two notes alike, at two places, sound as copies")
}

// Lanes playing the same note at the same frame don't start in phase, which would sound louder than two lanes do.
func TestLanesApartStartApart(t *testing.T) {
	if noteSeed(48000, 45, 1, 1) == noteSeed(48000, 45, 2, 1) {
		t.Error("two lanes take one seed")
	}
	if noteSeed(48000, 45, 1, 1) == noteSeed(48001, 45, 1, 1) || noteSeed(48000, 45, 1, 1) == noteSeed(48000, 47, 1, 1) {
		t.Error("two notes take one seed")
	}
	if noteSeed(48000, 45, 1, 0) != 0 {
		t.Error("a note played live takes a seed")
	}
	if noteSeed(0, 0, 0, 1) == 0 {
		t.Error("a note of the music takes no seed")
	}
}