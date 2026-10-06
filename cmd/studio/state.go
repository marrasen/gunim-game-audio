package main

import (
	"time"

	"github.com/marrasen/gunim"

	"github.com/marrasen/gunim-music/synth"
)

// The vocabulary the two halves share: the state the window shows, and
// the intents it sends back.
type (
	// Studio is what the window shows.
	Studio struct {
		// Songs are the songs' titles, and Song the one open.
		Songs []string
		Song  int
		// Gen counts the songs opened, and the edits made other than by
		// typing, so the window knows when to set its fields' text.
		Gen   int
		Title string
		// About says who made the song, its key and its tempo.
		About   string
		BPM     float64
		Key     string
		Playing bool
		Volume  float32
		// Clock says where the song is heard: at frame Frame at time At,
		// moving Rate frames a second, 0 while paused.
		Clock Clock
		// BarFrames is how long a bar lasts, in frames; Beats how many
		// beats a bar has; Bar the bar heard, from 0, which started at
		// BarFrame, and PhraseBars a phrase's length.
		BarFrames  float64
		Beats      int
		Bar        int
		BarFrame   int64
		PhraseBars int
		// Tiers is how many tiers the song has, 0 where it wanders, and
		// Tier the one set.
		Tiers, Tier int
		// Chords are the progression, and Chord the one heard; Spans lay
		// the chords out about the frame heard; ChordText is the
		// progression as its field shows it, and ChordErr what is wrong
		// with what was typed there.
		Chords    []string
		Chord     int
		Spans     []ChordSpan
		ChordText string
		ChordErr  string
		// Stings are the song's stings, and Sting the one playing;
		// Stopped says a sting ended the song.
		Stings  []string
		Sting   string
		Stopped bool
		// Keypad says the digit keys play notes.
		Keypad bool
		Tracks []TrackRow
		// Notes are the notes about the frame heard, of the tracks by
		// their index, -1 for the keypad's.
		Notes []NoteDot
		// Spectrum is how loud the sound is at each of the spectrum's
		// frequencies, in decibels.
		Spectrum []float32
		// Status says what was last done, or went wrong.
		Status string
		// Doc is the song open, as edited, for the editors to read. It is
		// never changed once sent: an edit makes another.
		Doc *synth.Song
		// Preview is a note of the patch the patch editor shows, and
		// DrumPreview a hit of the drum the kit editor shows.
		Preview, DrumPreview Preview
		// Scope is the last sound the track watched made, mono, and
		// ScopeTrack its name.
		Scope      []float32
		ScopeTrack string
		// Editor is the editor to show, by its tab's title, on
		// EditorTrack, each time EditorGen counts another.
		Editor      string
		EditorTrack string
		EditorGen   int
		// PatternSel is the step the pattern editor chooses after a change
		// to PatternTrack's pattern, PatternSelGen counting the changes.
		PatternTrack  string
		PatternSel    string
		PatternSelGen int
		// Master is the mix's level, and LUFS its short-term loudness;
		// Reduction is how far the compressor turns it down, in decibels.
		Master    Meter
		LUFS      float64
		Reduction float32
	}
	// Preview is a sound rendered for an editor to draw: Wave is its
	// whole as the lows and highs of a column each, Cycle a stretch of it
	// held steady, a few cycles long.
	Preview struct {
		Patch, Drum string
		Wave, Cycle []float32
	}
	// Meter is a pair of channels' levels, from 0 to 1 at full scale.
	Meter struct {
		Peak, RMS [2]float32
	}
	// Clock says where a song is heard.
	Clock struct {
		Frame int64
		At    time.Time
		Rate  float64
	}
	// ChordSpan is a chord heard from Frame for Len frames.
	ChordSpan struct {
		Frame, Len int64
		Name       string
		Index      int
	}
	// TrackRow is a track, and what it plays.
	TrackRow struct {
		// Gen is the studio's Gen, for the card to know when to set its
		// field and sliders.
		Gen         int
		Name, Color string
		Tier        int
		Core, Drums bool
		Playing     bool
		// Doing says what it plays: resting, intro, playing, outro, and
		// whether it is turned on or off by hand.
		Doing string
		Level float32
		// Pattern is its pattern, as typed, and PatternErr what keeps it
		// from playing, if anything.
		Pattern, PatternErr string
		// Gain is its level in decibels; Filter its filter's cutoff, in
		// octaves from 20 Hz, for a synth's patch, or its lowpass for
		// drums; Reverb and Delay its sends.
		Gain, Filter, Reverb, Delay float32
		// Meter is how loud it is after its fader, and Patch the patch
		// it plays.
		Meter Meter
		Patch string
		Mute  bool
		Solo  bool
	}
	// NoteDot is a note heard, or to be heard. DrumName names the drum
	// of a drum's note, as bd.
	NoteDot struct {
		Frame, Len int64
		Track      int
		Pitch      int
		Vel        float32
		Drum       bool
		DrumName   string
	}

	// SetValue sets the value at Path in the song, as path.go names it,
	// to Num, or to Str where IsStr says.
	SetValue struct {
		Path  string
		Num   float64
		Str   string
		IsStr bool
	}
	// ClearValue empties the value at Path, as a patch's Chip, which
	// turns it off.
	ClearValue struct{ Path string }
	// SetInts sets the list of whole numbers at Path, as a wave table.
	SetInts struct {
		Path   string
		Values []int
	}
	// AddItem appends an item, as an oscillator, to the list at Path.
	AddItem struct{ Path string }
	// RemoveItem takes item Index out of the list at Path.
	RemoveItem struct {
		Path  string
		Index int
	}
	// PatchNew makes a patch of Kind, synth, pluck or drums, and
	// PatchCopy a copy of the patch named Name.
	PatchNew  struct{ Kind string }
	PatchCopy struct{ Name string }
	// PatchSave saves the patch named Name to a file, and PatchLoad
	// loads one from a file in its place, or as a new patch where Name
	// is empty.
	PatchSave struct{ Name string }
	PatchLoad struct{ Name string }
	// TrackPatch sets the patch Track plays.
	TrackPatch struct{ Track, Patch string }
	// Audition plays a note of Patch, or its Drum, at once.
	Audition struct {
		Patch, Drum string
		Pitch       int
	}
	// ArpSet turns Patch's chip arpeggio on or off, and says whether it
	// plays chords; ArpSteps sets its steps, as 0 4 7.
	ArpSet struct {
		Patch     string
		On, Chord bool
	}
	ArpSteps struct{ Patch, Steps string }
	// OpenEditor shows the editor named Editor, by its tab's title, on
	// Track.
	OpenEditor struct{ Editor, Track string }
	// Focus says what the editors show: the patch and drum to render for
	// them, and the track whose sound the scope shows.
	Focus struct{ Patch, Kit, Drum, Track string }

	// SongChosen travels when a song is picked.
	SongChosen struct{ Song int }
	// PlayToggled travels when Play or Pause is pressed.
	PlayToggled struct{}
	// Restarted travels when Restart is pressed.
	Restarted struct{}
	// TierChosen travels when a tier is picked, from 1.
	TierChosen struct{ Tier int }
	// PartClicked travels when a track's orb or card is clicked.
	PartClicked struct{ Name string }
	// TierFollowed hands every part back to the tier.
	TierFollowed struct{}
	// StingPlayed travels when a sting's button is pressed.
	StingPlayed struct{ Name string }
	// KeyPlayed travels when a digit key is pressed.
	KeyPlayed struct{ Digit int }
	// PatternEdited travels as a track's pattern is typed.
	PatternEdited struct{ Track, Text string }
	// KnobSet travels as a track's slider moves: Knob is gain, filter,
	// reverb or delay.
	KnobSet struct {
		Track, Knob string
		Value       float32
	}
	// ChordsEdited travels as the progression is typed.
	ChordsEdited struct{ Text string }
	// ChordsGenerated asks for a progression written in Style.
	ChordsGenerated struct{ Style string }
	// BPMSet travels as the tempo is changed.
	BPMSet struct{ BPM float64 }
	// Transposed moves the song By semitones.
	Transposed struct{ By int }
	// VolumeSet travels as the volume slider moves.
	VolumeSet struct{ Volume float32 }
	// Saved asks to save the song as a song.json.
	Saved struct{}
)

func init() {
	gunim.RegisterType[Studio]("studio.state")
	gunim.RegisterType[SongChosen]("studio.song")
	gunim.RegisterType[PlayToggled]("studio.play")
	gunim.RegisterType[Restarted]("studio.restart")
	gunim.RegisterType[TierChosen]("studio.tier")
	gunim.RegisterType[PartClicked]("studio.part")
	gunim.RegisterType[TierFollowed]("studio.follow")
	gunim.RegisterType[StingPlayed]("studio.sting")
	gunim.RegisterType[KeyPlayed]("studio.key")
	gunim.RegisterType[PatternEdited]("studio.pattern")
	gunim.RegisterType[KnobSet]("studio.knob")
	gunim.RegisterType[ChordsEdited]("studio.chords")
	gunim.RegisterType[ChordsGenerated]("studio.generate")
	gunim.RegisterType[BPMSet]("studio.bpm")
	gunim.RegisterType[Transposed]("studio.transpose")
	gunim.RegisterType[VolumeSet]("studio.volume")
	gunim.RegisterType[Saved]("studio.save")
	gunim.RegisterType[SetValue]("studio.set")
	gunim.RegisterType[AddItem]("studio.add")
	gunim.RegisterType[ClearValue]("studio.clear")
	gunim.RegisterType[SetInts]("studio.ints")
	gunim.RegisterType[RemoveItem]("studio.remove")
	gunim.RegisterType[PatchNew]("studio.patch.new")
	gunim.RegisterType[PatchCopy]("studio.patch.copy")
	gunim.RegisterType[PatchSave]("studio.patch.save")
	gunim.RegisterType[PatchLoad]("studio.patch.load")
	gunim.RegisterType[TrackPatch]("studio.track.patch")
	gunim.RegisterType[Audition]("studio.audition")
	gunim.RegisterType[Focus]("studio.focus")
	gunim.RegisterType[OpenEditor]("studio.open")
	gunim.RegisterType[ArpSet]("studio.arp")
	gunim.RegisterType[ArpSteps]("studio.arp.steps")
}

// styles are the progression styles Generate offers, in order.
var styles = []string{"pop", "kpop", "epic", "villain"}
