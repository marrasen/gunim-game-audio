// Package music is a song for gunim's band player, package
// github.com/marrasen/gunim/audio/band: ten synths made in Reason at 136
// BPM. Nine come and go in 16-bar phrases, each with an intro, a loop
// and an outro, and a brass solo plays now and then. It gives a program
// music to try its sound with, as a game's:
//
//	mix := audio.NewMixer()
//	if _, err := speaker.Open(mix, speaker.Options{Name: "My game"}); err != nil {
//		return err
//	}
//	mix.Play(music.New(seed), audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
//
// The song is © 2026 Marcus Johansson, under the Creative Commons
// Attribution 4.0 licence in LICENSE-music: use it as you like, and
// credit him.
package music

import (
	"embed"

	"github.com/marrasen/gunim/audio/band"
)

// BPM is the song's tempo.
const BPM = 136

// files are the synths' parts, made by encode.sh: each synth's intro,
// loop and outro, or its solo, in Ogg Vorbis at 48 kHz.
//
//go:embed song/*.ogg
var files embed.FS

// Song returns the song, for [band.New]. Each call reads the parts into
// memory again, about 5 MB.
func Song() band.Song {
	parts, err := band.Load(files, "song")
	if err != nil {
		// The parts are embedded: a failure is a broken build.
		panic(err)
	}
	return band.Song{BPM: BPM, Parts: parts}
}

// New starts a band playing the song, choosing as seed says, so each
// seed plays the song its own way.
func New(seed uint64) *band.Band {
	return band.New(Song(), band.Options{Seed: seed})
}
