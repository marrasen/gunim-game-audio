// Package music is a library of songs for gunim's band player, package
// github.com/marrasen/gunim/audio/band, to give a program music to try
// its sound with, as a game's:
//
//	mix := audio.NewMixer()
//	if _, err := speaker.Open(mix, speaker.Options{Name: "My game"}); err != nil {
//		return err
//	}
//	song, err := music.Song(music.GreekThemes)
//	if err != nil {
//		return err
//	}
//	mix.Play(song.Play(seed), audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
//
// [Songs] lists them all. A song may offer features beyond playing, as
// interfaces its player implements; package band says which there are.
//
// [Calls] holds the companions' calls, short sounds a game's characters
// make, made in code by package calls, a new take each time:
//
//	lib, err := music.Calls()
//	...
//	voices := calls.NewPlayer(mix, lib)
//	voices.Warm()
//	voices.Play("uggla", calls.Hello, audio.Options{})
//
// The songs are © 2026 Marcus Johansson, under the Creative Commons
// Attribution 4.0 licence in LICENSE-music: use them as you like, and
// credit him.
package music

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"slices"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"

	"github.com/marrasen/gunim-game-audio/calls"
	"github.com/marrasen/gunim-game-audio/synth"
)

// The songs, by name.
const (
	// GreekThemes is ten synths made in Reason at 136 BPM, which gunim's
	// candy sudoku plays: nine come and go in 16-bar phrases, and a
	// brass solo plays now and then.
	GreekThemes = "greek-themes"
	// ARoundSong is eight parts made in Reason at 110 BPM, in four
	// tiers: a pad and bass always, then light percussion, then three
	// melodies, and then drums and a solo that comes round. It is a
	// [band.Tiers].
	ARoundSong = "a-round-song"
	// KeypadRound is A round song made in code, by package synth, at
	// 118 BPM in C major: eight tracks in four tiers, from a pad and
	// bass to full drums, a sparkle and a vocal chop, with a keypad
	// whose digits play the C major pentatonic over it.
	KeypadRound = "keypad-round"
	// BossEntrance is a cartoon villain's entrance, made in code, at
	// 146 BPM in D minor: pizzicato, a tuba, brass stabs and timpani in
	// four tiers that rise as a boss's health falls, and a two-bar victory
	// sting.
	BossEntrance = "boss-entrance"
	// MascotDance is a bright dance groove, made in code, at 124 BPM in
	// E major, for a mascot to dance to: its tracks come and go by
	// themselves, and a topline it writes itself changes as it goes.
	MascotDance = "mascot-dance"
	// BubbleBounce is a bouncy chip tune in the manner of Bubble Bobble,
	// made in code for a Commodore 64's sound, at 150 BPM in F major, in
	// four tiers: a hopping bass and arpeggiated chords; a counter melody
	// and drums; the lead; and its echo and a fuller beat.
	BubbleBounce = "bubble-bounce"
	// SisterDreams is a bittersweet chip tune in the manner of the Giana
	// Sisters' intro, made in code at 132 BPM in D minor: an arpeggio
	// through a sweeping SID filter and a squelching bass always, and a
	// lead, its echo, a pad, drums and wind coming and going.
	SisterDreams = "sister-dreams"
	// GraveyardGallop is a dark, driving chip tune in the manner of
	// Ghosts'n Goblins, made in code at 148 BPM in E minor, in four tiers:
	// a galloping bass and eerie chord stabs; a march; the lead; and its
	// harmony, toms and ringing bells. A stage-clear sting resumes it.
	GraveyardGallop = "graveyard-gallop"
	// PocketKingdom is a sunny chip tune in the manner of Super Mario
	// Land, made in code for the Game Boy's sound, at 144 BPM in G major,
	// in four tiers: a calypso bass on the wave channel and offbeat stabs;
	// drums and a counter melody; the lead; and its echo.
	PocketKingdom = "pocket-kingdom"
	// MeadowHop is a shuffling, jazzy chip tune in the manner of Super
	// Mario Bros. 3, made in code for the NES's sound, at 140 BPM in F
	// major, its eighths swung in triplets, in four tiers: a walking
	// triangle bass and two pulses comping each chord's third and
	// seventh; a shuffle beat; the lead's hook and claps; a counter line
	// and fills.
	MeadowHop = "meadow-hop"
	// HerosField is a heroic march in the manner of The Legend of Zelda,
	// for the NES's sound, at 130 BPM in B flat major, in four tiers:
	// triangle bass and triplet arpeggios; a march; the lead; its harmony
	// and drums. A treasure fanfare sting resumes it.
	HerosField = "heros-field"
	// PalaceRun is a driving chip tune in the manner of Zelda II's
	// palaces, for the NES's sound, at 160 BPM in A minor, in four tiers:
	// a pumping octave bass and racing arpeggios; drums; the lead; and its
	// harmony and a metallic clank.
	PalaceRun = "palace-run"
	// UnderworldAscent is a quirky chip tune in the manner of Kid Icarus,
	// for the NES's sound, at 150 BPM in G minor: a bouncing triangle bass
	// and chirps always, and a lead, its harmony, chords and drums coming
	// and going.
	UnderworldAscent = "underworld-ascent"
	// StarDrift is a calm song of space, made in code, at 76 BPM in D
	// Lydian, full of wonder: slow pads, a sub and bells ringing
	// always, and twinkles, arpeggios, a choir, a floating lead and a
	// soft heartbeat coming and going.
	StarDrift = "star-drift"
	// OrbitRound is a round song in space, made in code, at 116 BPM in E
	// minor, in four tiers: a pad, a bass and a driving sequencer; claps
	// and bells; a theremin's lead; drums and zaps. The digit keys play
	// the E minor pentatonic over it.
	OrbitRound = "orbit-round"
	// AlienEntrance is an alien boss's entrance, made in code, at 128
	// BPM in C minor, in four tiers that rise as the boss's health
	// falls: a bass and radar blips; saucer stabs, a march and claps; a
	// theremin's theme and a choir; drums, string runs and zaps. A
	// victory sting of 2 bars ends it.
	AlienEntrance = "alien-entrance"
	// CandyClouds is a calm, sweet song of candy land, made in code, at
	// 84 BPM in F major, for a game's room and its map: a soft pad, a
	// round bass, a music box and a soft snap always, and a marimba, a
	// celesta's tune, sugar sparkles and a hum coming and going.
	CandyClouds = "candy-clouds"
	// CompassRose is a calm song of travel, made in code, at 92 BPM in D
	// major, for a game's room and its map: a fingerpicked guitar, an
	// upright bass, strings and a hand drum always, and a wooden flute's
	// tune, a glockenspiel and the sea's swell coming and going.
	CompassRose = "compass-rose"
	// SummerMeadow is a calm song of a summer meadow, made in code, at
	// 88 BPM in G major, for a game's room and its map: a plucked harp,
	// a soft pad, a round bass and a woody tick always, and an
	// ocarina's tune, birdsong and a bumblebee's hum coming and going.
	SummerMeadow = "summer-meadow"
	// TinkerLab is a curious, bouncy song of an inventor's workshop,
	// made in code, at 96 BPM in A major, shuffled in triplets, for a
	// game's room and its map: plucked strings, a plucked bass, a soft
	// pad, a light groove and a clock's tick-tock always, and a
	// marimba's tune, bubbly blips and a vibraphone coming and going.
	TinkerLab = "tinker-lab"
	// TinkerRound is a round song of the same workshop, made in code,
	// at 112 BPM in A major, shuffled, in four tiers: plucked strings, a
	// plucked bass and a pad; claps and a clock; a marimba's tune;
	// drums and bubbly blips. The digit keys play the A major
	// pentatonic over it.
	TinkerRound = "tinker-round"
)

// files holds the songs: a folder each, named for the song, with its
// song.json and its parts, made by encode.sh.
//
//go:embed songs
var files embed.FS

// voices holds the companions' calls: library.json, and a recipe for
// each companion, named for its ID.
//
//go:embed voices/*.json
var voices embed.FS

// Calls returns the companions' calls, as the library holds them.
func Calls() (*calls.Library, error) {
	sub, err := fs.Sub(voices, "voices")
	if err != nil {
		return nil, err
	}
	return calls.Load(sub)
}

// ErrNoSong is returned for a name the library holds no song of.
var ErrNoSong = errors.New("music: no such song")

// Songs returns the songs' names, in order.
func Songs() []string {
	entries, err := fs.ReadDir(files, "songs")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Category returns the category of song name, as its song.json gives
// it: the group it shows in, in a list of songs, such as Boss fights or
// Calm rooms. A song of no category gives "".
func Category(name string) (string, error) {
	b, err := files.ReadFile(path.Join("songs", name, "song.json"))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNoSong, name)
	}
	var s spec
	if err = json.Unmarshal(b, &s); err != nil {
		return "", fmt.Errorf("music: %s: %w", name, err)
	}
	return s.Category, nil
}

// spec is a song.json: the song's kind, which says what plays it, and
// its settings. Kind "wander" is a [band.Wander], and "tiers" a
// [band.Tiers], whose Parts give each part its tier, and Start the
// tier it starts at. Kind "synth" is a [*synth.Song], made in code,
// which the whole song.json describes.
type spec struct {
	Kind          string
	Title, Artist string
	Category      string
	BPM           float64
	BeatsPerBar   int
	PhraseBars    int
	Parts         map[string]partSpec
	Start         int
}

// partSpec is a part's settings in a song.json.
type partSpec struct {
	// Tier is the tier the part plays from, and Loops how many times it
	// loops before it leaves and comes in again; see [band.Part].
	Tier, Loops int
}

// Song returns the song name. Its pieces read and decode as they play.
func Song(name string) (band.Song, error) {
	dir := path.Join("songs", name)
	b, err := files.ReadFile(path.Join(dir, "song.json"))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNoSong, name)
	}
	var s spec
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("music: %s: %w", name, err)
	}
	if s.Kind == "synth" {
		// A song made in code is all in its song.json.
		song, perr := synth.Parse(b)
		if perr != nil {
			return nil, fmt.Errorf("music: %s: %w", name, perr)
		}
		return song, nil
	}
	parts, err := band.Load(files, dir)
	if err != nil {
		return nil, err
	}
	// Each kind of song is a case here.
	switch s.Kind {
	case "wander":
		return &band.Wander{Title: s.Title, Artist: s.Artist, BPM: s.BPM, BeatsPerBar: s.BeatsPerBar,
			PhraseBars: s.PhraseBars, Parts: parts}, nil
	case "tiers":
		for i := range parts {
			p, ok := s.Parts[parts[i].Name]
			if !ok || p.Tier < 1 {
				return nil, fmt.Errorf("music: %s: song.json gives part %s no tier", name, parts[i].Name)
			}
			parts[i].Tier, parts[i].Loops = p.Tier, p.Loops
		}
		// The parts go in order of tier, as a tool lists them.
		slices.SortStableFunc(parts, func(a, b band.Part) int { return a.Tier - b.Tier })
		if len(s.Parts) != len(parts) {
			return nil, fmt.Errorf("music: %s: song.json names %d parts, and there are %d", name, len(s.Parts), len(parts))
		}
		return &band.Tiers{Title: s.Title, Artist: s.Artist, BPM: s.BPM, BeatsPerBar: s.BeatsPerBar,
			PhraseBars: s.PhraseBars, Parts: parts, Start: s.Start}, nil
	}
	return nil, fmt.Errorf("music: %s is of kind %q, which this version plays none of", name, s.Kind)
}

// Beat is where a song is in its bar: its bar, the beat in it, and how
// far through the beat; see [synth.Beat].
type Beat = synth.Beat

// BeatAt returns where song s, played by p, is at frame at, as heard:
// pass the frame the voice playing it is at, so a character dances to
// the beat heard. It works for every song of the library: a song made
// in code asks its player, which follows its tempo as it changes; a
// recorded one keeps its tempo from its first frame.
func BeatAt(s band.Song, p band.Player, at int64) Beat {
	if sp, ok := p.(*synth.Player); ok {
		return sp.Beat(at)
	}
	bpm, beats := s.Info().BPM, 4
	switch s := s.(type) {
	case *band.Wander:
		beats = s.BeatsPerBar
	case *band.Tiers:
		beats = s.BeatsPerBar
	}
	if bpm <= 0 || beats <= 0 {
		return Beat{}
	}
	bar := float64(audio.SampleRate) * 60 * float64(beats) / bpm
	n := int(math.Floor(float64(at) / bar))
	return synth.BeatOf(n, int64(math.Round(float64(n)*bar)), bar, beats, at)
}
