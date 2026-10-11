// Command midiplayer plays MIDI files through the synth engine's
// General MIDI instruments, in a window that shows the music played.
//
//	go run github.com/marrasen/gunim-game-audio/cmd/midiplayer@latest [file.mid ...]
//
// Drop MIDI files, or folders of them, on the window to play them, or
// press Open. The song falls onto a piano as it plays, and each channel
// has a station of its own on the stage, its instrument playing what it
// plays: a click mutes it, and a click with Shift plays it alone. The
// cartridges at the top pick the style the instruments play in, and the
// window takes on the look of its machine: General MIDI, a Commodore
// 64, a NES, a Game Boy or an AdLib card.
//
// Space plays and pauses, the arrows move five seconds, N and P skip, 1
// to 5 pick a style, + and - change the speed, L shows the playlist and
// O opens files.
//
// -style picks the style to start in, and -demo plays the demo song.
// -shot writes a picture of the window to a PNG file after -after, and
// quits, for the documentation; -silent plays without the speakers.
package main

import (
	"context"
	"flag"
	"image/png"
	"log"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/synth"
)

func main() {
	style := flag.String("style", "gm", "the style to start in: gm, sid, nes, gb or adlib")
	demo := flag.Bool("demo", false, "play the demo song")
	shot := flag.String("shot", "", "write a picture of the window to this PNG file, and quit")
	after := flag.Duration("after", 3*time.Second, "with -shot, how long to play first")
	silent := flag.Bool("silent", false, "play without the speakers, as for -shot")
	flag.Parse()
	if !slices.Contains(synth.GMStyles, *style) {
		log.Fatalf("no style %q", *style)
	}
	if err := run(*style, *demo, *silent, *shot, *after, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

func run(style string, demo, silent bool, shot string, after time.Duration, files []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title:         "gunim MIDI player",
			Size:          geom.Sz(1360, 880),
			Root:          widget.NewSurface(),
			UnderTitleBar: true,
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		gunim.RegisterView(w, "midiplayer", buildView, (*root).update)
		mix := audio.NewMixer()
		if silent {
			go drain(ctx, mix)
		} else if _, err := speaker.Open(mix, speaker.Options{Name: "gunim MIDI player"}); err != nil {
			return err
		}
		ap := newApp(w.Client(), mix)
		ap.style = style
		if demo {
			ap.addDemo()
		}
		if shot != "" {
			ctx2, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				time.Sleep(after)
				if err := writeShot(ctx2, w.Client(), shot); err != nil {
					log.Print(err)
				}
				cancel()
			}()
			return ap.serve(ctx2, files)
		}
		return ap.serve(ctx, files)
	})
}

// drain mixes mix as the speakers would, in real time, and throws the
// sound away.
func drain(ctx context.Context, mix *audio.Mixer) {
	buf := make([]float32, 2*480)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			mix.Mix(buf)
		}
	}
}

// writeShot writes a picture of the window to file.
func writeShot(ctx context.Context, c gunim.Client, file string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
