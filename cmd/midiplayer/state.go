package main

import (
	"time"

	"github.com/marrasen/gunim"
)

// The vocabulary the two halves share.
type (
	// Player is the state the window shows.
	Player struct {
		// Playlist is the files dropped or opened, and Current the one
		// playing, -1 for none.
		Playlist []Item
		Current  int
		// Score is the song playing, made ready to draw, and ScoreGen
		// counts the songs loaded, so the window knows a new one.
		Score    *Score
		ScoreGen int
		// Clock says where the song is heard.
		Clock   Clock
		Playing bool
		// Style is the style the instruments play in, one of
		// synth.GMStyles.
		Style string
		// Muted are the channels muted, and Solo the one played alone,
		// -1 for none.
		Muted [16]bool
		Solo  int
		// Speed is how fast the song plays, 1 as written, and Transpose
		// how many semitones it is moved.
		Speed     float64
		Transpose int
		Volume    float32
		// Repeat is what plays after a song ends: the next in the list,
		// the list again from its start, or the song again.
		Repeat Repeat
		// Message says what went wrong with a file, or what happened;
		// MessageGen counts the messages, so the same one twice shows
		// twice.
		Message    string
		MessageGen int
	}
	// Item is a file of the playlist.
	Item struct {
		Name, Path string
		// Length is how long it plays, in seconds, once read.
		Length float64
		// Err says why it could not be read.
		Err string
	}
	// Clock says where a song is heard: Time seconds into it at At,
	// moving Rate seconds a second, 0 while paused.
	Clock struct {
		Time float64
		At   time.Time
		Rate float64
	}
	// Repeat is what plays after a song ends.
	Repeat int

	// FilesDropped travels when files are dropped on the window.
	FilesDropped struct{ Paths []string }
	// OpenAsked travels when Open is pressed.
	OpenAsked struct{}
	// DemoAsked travels when the demo is asked for.
	DemoAsked struct{}
	// PlayToggled travels when play or pause is pressed.
	PlayToggled struct{}
	// Skipped travels when next, By 1, or previous, By -1, is pressed.
	Skipped struct{ By int }
	// Sought travels when the timeline is clicked or dragged, or a key
	// moves through the song.
	Sought struct{ Time float64 }
	// StyleChosen travels when a style's cartridge is picked.
	StyleChosen struct{ Style string }
	// ChannelClicked travels when a channel's station is clicked: Solo
	// with Shift held.
	ChannelClicked struct {
		Channel int
		Solo    bool
	}
	// SpeedSet travels as the speed changes.
	SpeedSet struct{ Speed float64 }
	// TransposeSet travels as the transpose changes.
	TransposeSet struct{ Semis int }
	// VolumeSet travels as the volume slider moves.
	VolumeSet struct{ Volume float32 }
	// RepeatToggled travels when the repeat button is pressed.
	RepeatToggled struct{}
	// Picked travels when a song of the playlist is picked.
	Picked struct{ Index int }
	// Removed travels when a song is taken off the playlist.
	Removed struct{ Index int }
)

// The ways to repeat.
const (
	RepeatOff Repeat = iota
	RepeatAll
	RepeatOne
)

func init() {
	gunim.RegisterType[Player]("midiplayer.state")
	gunim.RegisterType[FilesDropped]("midiplayer.drop")
	gunim.RegisterType[OpenAsked]("midiplayer.open")
	gunim.RegisterType[DemoAsked]("midiplayer.demo")
	gunim.RegisterType[PlayToggled]("midiplayer.play")
	gunim.RegisterType[Skipped]("midiplayer.skip")
	gunim.RegisterType[Sought]("midiplayer.seek")
	gunim.RegisterType[StyleChosen]("midiplayer.style")
	gunim.RegisterType[ChannelClicked]("midiplayer.channel")
	gunim.RegisterType[SpeedSet]("midiplayer.speed")
	gunim.RegisterType[TransposeSet]("midiplayer.transpose")
	gunim.RegisterType[VolumeSet]("midiplayer.volume")
	gunim.RegisterType[RepeatToggled]("midiplayer.repeat")
	gunim.RegisterType[Picked]("midiplayer.pick")
	gunim.RegisterType[Removed]("midiplayer.remove")
}

// heard returns the time heard at now, from c.
func (c Clock) heard(now time.Time) float64 {
	if c.At.IsZero() {
		return c.Time
	}
	return c.Time + now.Sub(c.At).Seconds()*c.Rate
}
