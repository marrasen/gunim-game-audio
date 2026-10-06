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
	"path"
	"slices"

	"github.com/marrasen/gunim/audio/band"

	"github.com/marrasen/gunim-music/synth"
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
	// four tiers that rise as a boss's health falls, and a victory
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
)

// files holds the songs: a folder each, named for the song, with its
// song.json and its parts, made by encode.sh.
//
//go:embed songs
var files embed.FS

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

// spec is a song.json: the song's kind, which says what plays it, and
// its settings. Kind "wander" is a [band.Wander], and "tiers" a
// [band.Tiers], whose Parts give each part its tier, and Start the
// tier it starts at. Kind "synth" is a [*synth.Song], made in code,
// which the whole song.json describes.
type spec struct {
	Kind          string
	Title, Artist string
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
