// Command play plays a song of the library through the speakers
// until interrupted.
//
//	go run github.com/marrasen/gunim-music/cmd/play@latest
//
// -song picks the song, by name, and -list lists them. -seed picks how
// the band plays it; each seed plays it its own way.
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
	"github.com/marrasen/gunim/audio/speaker"

	music "github.com/marrasen/gunim-music"
)

func main() {
	name := flag.String("song", music.GreekThemes, "the song to play, by name")
	list := flag.Bool("list", false, "list the songs, and quit")
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "how the band plays the song")
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
	mix.Play(song.Play(*seed), audio.Options{FadeIn: time.Second})
	log.Printf("playing %s, seed %d; Ctrl+C stops", song.Info().Title, *seed)
	<-ctx.Done()
}
