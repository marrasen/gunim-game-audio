// Command play plays the song through the speakers until interrupted.
//
//	go run github.com/marrasen/gunim-music/cmd/play@latest
//
// -seed picks how the band plays it; each seed plays it its own way.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"

	music "github.com/marrasen/gunim-music"
)

func main() {
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "how the band plays the song")
	flag.Parse()
	mix := audio.NewMixer()
	if _, err := speaker.Open(mix, speaker.Options{Name: "gunim-music"}); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	mix.Play(music.New(*seed), audio.Options{FadeIn: time.Second})
	log.Printf("playing, seed %d; Ctrl+C stops", *seed)
	<-ctx.Done()
}
