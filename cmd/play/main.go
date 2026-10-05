// Command play plays a song of the library through the speakers
// until interrupted.
//
//	go run github.com/marrasen/gunim-music/cmd/play@latest
//
// -song picks the song, by name, and -list lists them. -seed picks how
// the band plays it; each seed plays it its own way. -tier sets the
// tier of a song that has tiers. The jukebox, cmd/jukebox, is a window
// to try the songs in.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
	"github.com/marrasen/gunim/audio/speaker"

	music "github.com/marrasen/gunim-music"
)

func main() {
	name := flag.String("song", music.GreekThemes, "the song to play, by name")
	list := flag.Bool("list", false, "list the songs, and quit")
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "how the band plays the song")
	tier := flag.Int("tier", 1, "the tier to play, for a song that has tiers")
	flag.Parse()
	if *list {
		for _, n := range music.Songs() {
			if s, err := music.Song(n); err == nil {
				i := s.Info()
				fmt.Printf("%s\t%s, by %s, %v BPM\n", n, i.Title, i.Artist, i.BPM)
			}
		}
		return
	}
	song, err := music.Song(*name)
	if err != nil {
		log.Fatalf("%v; -list lists the songs", err)
	}
	mix := audio.NewMixer()
	if _, err := speaker.Open(mix, speaker.Options{Name: "gunim-music"}); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	p := song.Play(*seed)
	if t, ok := p.(band.Tiered); ok {
		t.SetTier(*tier)
	}
	mix.Play(p, audio.Options{FadeIn: time.Second})
	log.Printf("playing %s, seed %d; Ctrl+C stops", song.Info().Title, *seed)
	<-ctx.Done()
}
