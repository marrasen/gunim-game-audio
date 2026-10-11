// Command studio is gunim music studio: a window to make game music
// in, with package synth's engine playing as you work.
//
//	go run github.com/marrasen/gunim-game-audio/cmd/studio@latest
//
// It opens the library's songs made in code. The stage shows the song in
// 3D: each track an orb on the ring of its tier, its notes flying in to
// the core as sparks, its drums rippling on the floor, and the chords
// round the core. Under it the notes flow past the line where they are
// heard, the bar to come already in sight. Each track's card takes a
// new pattern as it is typed, heard from the next bar, and sliders for
// its level, its filter and its sends. The chords take a new
// progression as it is typed, or write one in a style. The tier, the
// stings, the tempo and the key change as it plays, and the digit keys
// play the keypad over a song that has one.
//
// Tabs swap the stage for the editors: a mixing desk; a synth's patch,
// its oscillators, filter, envelopes and LFOs drawn and turned by
// knobs, played from a keyboard; a kit's drums as pads; the effects;
// a track's pattern, its structure as boxes and its notes on a grid of
// steps; and every drum track at once, as a drum machine shows them.
// Patches and kits save to files of their own and load from them.
//
// -song opens a song by name, and -editor an editor, by its tab, on the
// track -track. -shot writes the window to a PNG after -after, and
// quits; -tier sets the tier to start at.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/soundfont"
)

func main() {
	song := flag.String("song", music.KeypadRound, "the song to open, by name")
	tier := flag.Int("tier", 0, "the tier to start at")
	editor := flag.String("editor", "", "the editor to open, by its tab: Stage, Mixer, Patch, Kit, Effects, Pattern or Beat")
	on := flag.String("track", "", "with -editor, the track to open it on")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	font := flag.String("soundfont", "", "a SoundFont bank, .sf2 or .sf3, for patches to play on, as the songs' General MIDI bank")
	after := flag.Duration("after", 6*time.Second, "how long -shot waits")
	flag.Parse()
	if *font != "" {
		if err := registerFont(*font); err != nil {
			log.Fatal(err)
		}
	}
	if err := run(*song, *tier, *editor, *on, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(song string, tier int, editor, on, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim music studio",
			Size:  geom.Sz(1500, 940),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		gunim.RegisterView(w, "studio", buildView, (*root).update)
		c := w.Client()
		mix := audio.NewMixer()
		if _, serr := speaker.Open(mix, speaker.Options{Name: "gunim music studio"}); serr != nil {
			return serr
		}
		s, err := newStudio(c, mix, song)
		if err != nil {
			return err
		}
		if tier > 0 {
			s.p.SetTier(tier)
		}
		if editor != "" {
			s.handle(ctx, OpenEditor{Editor: editor, Track: on})
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
		return s.serve(ctx)
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

// fontName, fontPrograms and fontKits are the name -soundfont's bank is
// registered by, music.SoundFont, the names of its programs of bank 0,
// by number, and its drum kits.
var (
	fontName     string
	fontPrograms []string
	fontKits     []FontKit
)

// registerFont reads the bank at path and registers it as the songs'
// General MIDI bank, for patches to play on.
func registerFont(path string) error {
	f, err := soundfont.Open(path)
	if err != nil {
		return err
	}
	// Registered as the library's songs name it, so they play on it, and
	// the patches made here play on a game's bank.
	fontName = music.SoundFont
	music.UseSoundFont(f)
	fontPrograms = make([]string, 128)
	for i := range fontPrograms {
		name := "—"
		if p := f.Preset(0, i); p != nil {
			name = p.Name
		}
		fontPrograms[i] = fmt.Sprintf("%03d %s", i+1, name)
	}
	fontKits = nil
	for _, p := range f.Presets {
		if p.Bank == 128 {
			fontKits = append(fontKits, FontKit{Program: p.Program, Name: p.Name})
		}
	}
	slices.SortFunc(fontKits, func(a, b FontKit) int { return a.Program - b.Program })
	return nil
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
