package main

import "github.com/marrasen/gunim"

// The vocabulary the two halves share: the state the window shows, and
// the intents it sends back.
type (
	// Lab is what the window shows.
	Lab struct {
		Companions []CompanionRow
		Companion  int
		// Name is the companion's, and About its style and character.
		Name, About string
		Calls       []CallView
		// Selected is the call the editor shows, by its index in Calls.
		Selected int
		Editor   Editor
		// HighPass, Loudness and Ceiling finish every call.
		HighPass, Loudness, Ceiling float64
		// Phone plays the calls as a phone's speaker does.
		Phone bool
		// Songs are the songs to play under the calls, "None" first, and
		// Song the one playing.
		Songs       []string
		Song        int
		MusicVolume float32
		// Dirty says there are changes not saved; Status says what was
		// done last, or what went wrong.
		Dirty  bool
		Status string
		// Gen counts the times the editor's call changed other than by
		// typing, so the window knows when to set its notes' text.
		Gen int
	}
	// CompanionRow is a companion in the list.
	CompanionRow struct {
		ID, Name, Doing string
		Made, Open      bool
	}
	// CallView is a call: its take, drawn and measured.
	CallView struct {
		Kind string
		Made bool
		// Wave is the take's lows and highs, column by column, and
		// Spectrum its level in bands from 100 Hz to 16 kHz, in dB.
		Wave     []float32
		Spectrum []float32
		// Stats says how it measures, and Problems where it falls short.
		Stats    string
		Problems []string
		Take     string
		// Playhead is how far through it plays, from 0 to 1, or -1.
		Playhead float32
	}
	// Editor is the selected call's settings.
	Editor struct {
		Kind       string
		Layers     []LayerView
		Vary, Room float64
		Cut        float64
		Presence   float64
		Notes      string
		Models     []string
	}
	// LayerView is a layer of a call: its model and its numbers.
	LayerView struct {
		Model, About string
		Params       []ParamView
	}
	// ParamView is a number of a layer, with its range.
	ParamView struct {
		Name, Label, Unit, About string
		Lo, Hi, Def, Value, Step float64
		Log                      bool
		Choices                  []string
		// Odd keeps a stepped number odd, or 0; Zero names 0.
		Odd  bool
		Zero string
	}

	// CompanionChosen travels when a companion is picked.
	CompanionChosen struct{ ID string }
	// CallChosen travels when a call is picked for the editor.
	CallChosen struct{ Call int }
	// PlayCall travels to play a call's take again; NewTake makes a new
	// take and plays it; AsSet plays the call as set, take 0.
	PlayCall struct{ Call int }
	NewTake  struct{ Call int }
	AsSet    struct{ Call int }
	// PlayAll plays the companion's calls, one after another.
	PlayAll struct{}
	// ParamSet sets a number: of layer Layer, or with Layer -1 of the
	// call, Vary, Room, Cut or Presence, or with Layer -2 of the master. Done says the
	// knob was let go, and the call plays.
	ParamSet struct {
		Layer int
		Name  string
		Value float64
		Done  bool
	}
	// LayerAdded adds a layer of a model; LayerRemoved takes one away;
	// ModelChosen changes a layer's model.
	LayerAdded   struct{ Model string }
	LayerRemoved struct{ Layer int }
	ModelChosen  struct {
		Layer int
		Model string
	}
	// NotesSet travels as the notes are typed.
	NotesSet struct{ Text string }
	// Saved saves the companion; Reverted reads it back from its file.
	Saved    struct{}
	Reverted struct{}
	// PhoneSet turns the phone's speaker on or off.
	PhoneSet struct{ On bool }
	// SongChosen picks the song under the calls, 0 for none.
	SongChosen struct{ Song int }
	// MusicVolumeSet sets the song's volume.
	MusicVolumeSet struct{ Volume float32 }
)

func init() {
	gunim.RegisterType[Lab]("calls.state")
	gunim.RegisterType[CompanionChosen]("calls.companion")
	gunim.RegisterType[CallChosen]("calls.call")
	gunim.RegisterType[PlayCall]("calls.play")
	gunim.RegisterType[NewTake]("calls.take")
	gunim.RegisterType[AsSet]("calls.asset")
	gunim.RegisterType[PlayAll]("calls.all")
	gunim.RegisterType[ParamSet]("calls.param")
	gunim.RegisterType[LayerAdded]("calls.addlayer")
	gunim.RegisterType[LayerRemoved]("calls.removelayer")
	gunim.RegisterType[ModelChosen]("calls.model")
	gunim.RegisterType[NotesSet]("calls.notes")
	gunim.RegisterType[Saved]("calls.save")
	gunim.RegisterType[Reverted]("calls.revert")
	gunim.RegisterType[PhoneSet]("calls.phone")
	gunim.RegisterType[SongChosen]("calls.song")
	gunim.RegisterType[MusicVolumeSet]("calls.musicvolume")
}
