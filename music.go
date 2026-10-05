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

	"github.com/marrasen/gunim/audio/band"
)

// The songs, by name.
const (
	// GreekThemes is ten synths made in Reason at 136 BPM, which gunim's
	// candy sudoku plays: nine come and go in 16-bar phrases, and a
	// brass solo plays now and then.
	GreekThemes = "greek-themes"
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
// its settings. Kind "wander" is a [band.Wander].
type spec struct {
	Kind          string
	Title, Artist string
	BPM           float64
	BeatsPerBar   int
	PhraseBars    int
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
	// Each kind of song is a case here; Kind "wander" is the first.
	if s.Kind != "wander" {
		return nil, fmt.Errorf("music: %s is of kind %q, which this version plays none of", name, s.Kind)
	}
	parts, err := band.Load(files, dir)
	if err != nil {
		return nil, err
	}
	return &band.Wander{Title: s.Title, Artist: s.Artist, BPM: s.BPM, BeatsPerBar: s.BeatsPerBar,
		PhraseBars: s.PhraseBars, Parts: parts}, nil
}
