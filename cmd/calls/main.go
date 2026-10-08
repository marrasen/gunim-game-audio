// Command calls is a window to hear the companions' calls in, and set
// them by ear: each companion's hello, cheer and oops, drawn and
// measured against the game's brief, with a knob for each number of the
// models that make them. A call plays as a knob is let go; New take
// plays another take, as the game makes one each time; Phone speaker
// plays them as a phone's speaker does; and a song can play under them.
// Notes for Claude say what should change, and save with the call.
//
//	go run ./cmd/calls
//
// It reads and saves the recipes in -dir, voices by default. -render
// writes every call to a WAV file in a folder instead, and says how
// each measures. -companion and -call open a call. -shot writes the window to a PNG after -after, and
// quits.
package main

import (
	"context"
	"errors"
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
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/calls"
)

func main() {
	dir := flag.String("dir", "voices", "the folder of the calls' recipes")
	out := flag.String("render", "", "write every call to a WAV file in this folder, and quit")
	seed := flag.Uint64("seed", 0, "with -render, the take to write: 0 the call as set")
	companion := flag.String("companion", "", "the companion to open, by ID")
	call := flag.String("call", "", "the call to open: hello, cheer or oops")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	flag.Parse()
	if *out != "" {
		lib, err := calls.LoadDir(*dir)
		if err != nil {
			log.Fatal(err)
		}
		if err := render(lib, *out, *seed); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(*dir, *companion, *call, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(dir, companion, call, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "Companion calls",
			Size:  geom.Sz(1360, 940),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		gunim.RegisterView(w, "lab", buildView, (*root).update)
		c := w.Client()
		mix := audio.NewMixer()
		if _, serr := speaker.Open(mix, speaker.Options{Name: "Companion calls"}); serr != nil {
			return serr
		}
		l, err := newLab(c, mix, dir)
		if err != nil {
			return err
		}
		if companion != "" {
			l.open(companion)
		}
		if i := slices.Index(l.kinds(), call); i >= 0 {
			l.sel = i
		}
		if shot != "" {
			go func() {
				time.Sleep(after)
				if err := writeShot(ctx, c, shot); err != nil {
					log.Print(err)
				}
				c.Close()
			}()
		}
		return l.serve(ctx)
	})
	switch {
	case errors.Is(err, driver.ErrNoDriver):
		log.Print("gunim has no driver for this operating system yet")
		return nil
	case errors.Is(err, context.Canceled):
		return nil
	}
	return err
}

func writeShot(ctx context.Context, c gunim.Client, path string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}
