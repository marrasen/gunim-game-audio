package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/driver"

	"github.com/marrasen/gunim-game-audio/midi"
	"github.com/marrasen/gunim-game-audio/synth"
)

// app is the application half: the playlist, the song playing and how
// it plays.
type app struct {
	c     gunim.Client
	mixer *audio.Mixer
	// list is the playlist, files its files read, nil until read, and
	// cur the one playing, -1 for none.
	list  []Item
	files []*midi.File
	cur   int
	p     *synth.MIDIPlayer
	voice *audio.Voice
	score *Score
	gen   int
	// ended says the song playing has ended and what follows has been
	// seen to.
	ended     bool
	style     string
	muted     [16]bool
	solo      int
	speed     float64
	transpose int
	volume    float32
	repeat    Repeat
	msg       string
	msgGen    int
	// mix is how the channels are mixed, kept from song to song and saved
	// to mixFile, where set, a moment after it changes; an measures the
	// sound heard.
	mix      synth.GMMix
	mixFile  string
	mixDirty time.Time
	an       *audio.Analyzer
	spec     []float32
	// chosen carries the files the open dialog chose to the serve loop.
	chosen chan []string
}

func newApp(c gunim.Client, mix *audio.Mixer) *app {
	return &app{c: c, mixer: mix, cur: -1, solo: -1, speed: 1, volume: 0.9, style: "gm", repeat: RepeatAll,
		chosen: make(chan []string, 1), mix: synth.DefaultGMMix(), an: audio.NewAnalyzer(mix, 32),
		spec: make([]float32, len(specFreqs))}
}

// midiFiles are the files the open dialog shows.
var midiFiles = []driver.FileFilter{{Name: "MIDI files", Patterns: []string{"*.mid", "*.midi", "*.kar", "*.smf"}}}

// isMIDI reports whether path is named as a MIDI file.
func isMIDI(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mid", ".midi", ".kar", ".smf", ".rmi":
		return true
	}
	return false
}

// add puts the MIDI files among paths, and those in folders among them,
// on the playlist, and plays the first of them at once. It reads each
// file now, so one that is no MIDI file says so at once.
func (a *app) add(paths []string) {
	var found []string
	for _, p := range paths {
		// Some file managers hand over a URI in place of a path.
		if u, err := url.Parse(p); err == nil && u.Scheme == "file" {
			p = u.Path
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			_ = filepath.WalkDir(p, func(q string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && isMIDI(q) {
					found = append(found, q)
				}
				return nil
			})
			continue
		}
		found = append(found, p)
	}
	first := len(a.list)
	for _, p := range found {
		f, err := readMIDI(p)
		if err != nil {
			a.say(fmt.Sprintf("%s is not a MIDI file I can play: %v", filepath.Base(p), err))
			continue
		}
		a.list = append(a.list, Item{Name: displayName(p, f), Path: p, Length: f.Length()})
		a.files = append(a.files, f)
	}
	if len(found) == 0 && len(paths) > 0 {
		a.say("No MIDI files there")
	}
	if added := len(a.list) - first; added > 0 {
		a.start(first)
		if added > 1 {
			a.say(fmt.Sprintf("Playing %s, and %d more after it", a.list[first].Name, added-1))
		}
	}
}

// addDemo puts the demo song on the playlist and plays it.
func (a *app) addDemo() {
	f, err := midi.Read(bytes.NewReader(demoSong()))
	if err != nil {
		a.say(err.Error())
		return
	}
	a.list = append(a.list, Item{Name: "Neon Overture (demo)", Length: f.Length()})
	a.files = append(a.files, f)
	a.start(len(a.list) - 1)
}

// readMIDI reads the MIDI file at path.
func readMIDI(path string) (*midi.File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := midi.Read(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	n := 0
	for _, t := range f.Tracks {
		n += len(t)
	}
	if n == 0 {
		return nil, errors.New("it holds no notes")
	}
	return f, nil
}

// displayName names a file: its own name inside, where it is a real
// one, or else its file's name.
func displayName(path string, f *midi.File) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if f == nil {
		return base
	}
	n := strings.TrimSpace(f.Name())
	switch strings.ToLower(n) {
	case "", "untitled", "track 1", "track1", "tempo track", "conductor":
		return base
	}
	if len(n) > 60 {
		return base
	}
	return n
}

func (a *app) say(msg string) {
	a.msg = msg
	a.msgGen++
}

// start plays song i of the playlist from its start.
func (a *app) start(i int) {
	if i < 0 || i >= len(a.list) {
		return
	}
	if a.voice != nil {
		a.voice.Stop(150 * time.Millisecond)
	}
	f := a.files[i]
	a.cur, a.ended = i, false
	a.p = synth.NewMIDIPlayer(f)
	_ = a.p.SetStyle(a.style)
	a.p.SetSpeed(a.speed)
	a.p.SetTranspose(a.transpose)
	a.p.SetMix(a.mix)
	a.applyMutes()
	a.score = newScore(f, a.list[i].Name)
	a.gen++
	a.voice = a.mixer.Play(a.p, audio.Options{Volume: a.volume})
}

// applyMutes sets what each channel plays: muted, or alone.
func (a *app) applyMutes() {
	if a.p == nil {
		return
	}
	for ch := range 16 {
		a.p.SetMute(ch, a.muted[ch] || a.solo >= 0 && a.solo != ch)
	}
}

// heard returns where the song is heard, in seconds.
func (a *app) heard() float64 {
	if a.p == nil || a.voice == nil {
		return 0
	}
	return a.p.TimeAt(audio.Frames(a.voice.Position()))
}

func (a *app) playing() bool { return a.voice != nil && !a.voice.Paused() && !a.ended }

// state returns what the window shows.
func (a *app) state() Player {
	s := Player{Playlist: slices.Clone(a.list), Current: a.cur, Score: a.score, ScoreGen: a.gen, Playing: a.playing(),
		Style: a.style, Muted: a.muted, Solo: a.solo, Speed: a.speed, Transpose: a.transpose, Volume: a.volume,
		Repeat: a.repeat, Message: a.msg, MessageGen: a.msgGen, Mix: a.mix}
	if a.p != nil {
		s.Meters = a.p.TakeMeters()
	}
	a.an.Spectrum(specFreqs, a.spec, nil)
	s.Spectrum = slices.Clone(a.spec)
	s.Clock = Clock{Time: a.heard(), At: time.Now()}
	if s.Playing {
		s.Clock.Rate = a.speed
	}
	if a.score != nil {
		s.Clock.Time = min(s.Clock.Time, a.score.Length)
	}
	return s
}

// handle carries out an intent.
func (a *app) handle(ctx context.Context, in gunim.Intent) {
	switch v := in.(type) {
	case FilesDropped:
		a.add(v.Paths)
	case OpenAsked:
		go func() {
			paths, err := a.c.ChooseFiles(ctx, driver.ChooseOptions{Title: "Open MIDI files", Multiple: true, Filters: midiFiles})
			if err == nil && len(paths) > 0 {
				a.chosen <- paths
			}
		}()
	case DemoAsked:
		a.addDemo()
	case PlayToggled:
		switch {
		case a.voice == nil || a.ended:
			if len(a.list) > 0 {
				a.start(max(a.cur, 0))
			}
		case a.voice.Paused():
			a.voice.Resume()
		default:
			a.voice.Pause()
		}
	case Skipped:
		a.skip(v.By)
	case Sought:
		if a.p != nil {
			if a.ended {
				// A song ended is played again from where it is sought.
				a.start(a.cur)
			}
			a.p.Seek(min(max(v.Time, 0), a.p.Length()))
		}
	case StyleChosen:
		if slices.Contains(synth.GMStyles, v.Style) {
			a.style = v.Style
			if a.p != nil {
				_ = a.p.SetStyle(v.Style)
			}
		}
	case ChannelClicked:
		ch := v.Channel & 15
		if v.Solo {
			if a.solo == ch {
				a.solo = -1
			} else {
				a.solo = ch
			}
		} else {
			a.muted[ch] = !a.muted[ch]
		}
		a.applyMutes()
	case SpeedSet:
		a.speed = min(max(v.Speed, 0.25), 3)
		if a.p != nil {
			a.p.SetSpeed(a.speed)
		}
	case TransposeSet:
		a.transpose = min(max(v.Semis, -24), 24)
		if a.p != nil {
			a.p.SetTranspose(a.transpose)
		}
	case VolumeSet:
		a.volume = min(max(v.Volume, 0), 1)
		if a.voice != nil {
			a.voice.SetVolume(a.volume, anim.Spring{Response: 0.15, Damping: 1})
		}
	case RepeatToggled:
		a.repeat = (a.repeat + 1) % 3
	case Picked:
		a.start(v.Index)
	case Removed:
		a.remove(v.Index)
	case MixSet:
		a.mix = v.Mix
		if a.p != nil {
			a.p.SetMix(a.mix)
		}
		a.mixDirty = time.Now()
	}
}

// skip plays the song by songs on in the playlist, round its ends. A
// step back more than three seconds into a song starts it again.
func (a *app) skip(by int) {
	if len(a.list) == 0 {
		return
	}
	if by < 0 && a.heard() > 3 && !a.ended {
		a.p.Seek(0)
		return
	}
	a.start(((a.cur+by)%len(a.list) + len(a.list)) % len(a.list))
}

// remove takes song i off the playlist; the one playing stops.
func (a *app) remove(i int) {
	if i < 0 || i >= len(a.list) {
		return
	}
	a.list = slices.Delete(a.list, i, i+1)
	a.files = slices.Delete(a.files, i, i+1)
	switch {
	case i == a.cur:
		if a.voice != nil {
			a.voice.Stop(200 * time.Millisecond)
		}
		a.voice, a.p, a.score, a.cur = nil, nil, nil, -1
		a.gen++
		if len(a.list) > 0 {
			a.start(min(i, len(a.list)-1))
		}
	case i < a.cur:
		a.cur--
	}
}

// follow sees to a song that has ended: the next plays, or the same
// again, as Repeat says.
func (a *app) follow() {
	if a.p == nil || a.ended || !a.p.Ended() {
		return
	}
	a.ended = true
	switch {
	case a.repeat == RepeatOne:
		a.start(a.cur)
	case a.cur+1 < len(a.list):
		a.start(a.cur + 1)
	case a.repeat == RepeatAll && len(a.list) > 0:
		a.start(0)
	}
}

// serve runs the application half until ctx ends.
func (a *app) serve(ctx context.Context, files []string) error {
	if len(files) > 0 {
		a.add(files)
	}
	if err := a.c.Mount(gunim.Root, "midiplayer", "midiplayer", a.state()); err != nil {
		return err
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		case paths := <-a.chosen:
			a.add(paths)
		case ev, ok := <-a.c.Intents():
			if !ok {
				return a.c.Err()
			}
			a.handle(ctx, ev.Intent)
		}
		a.follow()
		if !a.mixDirty.IsZero() && time.Since(a.mixDirty) > time.Second {
			a.mixDirty = time.Time{}
			a.saveMix()
		}
		_ = a.c.Update("midiplayer", a.state())
	}
}

// loadMix reads the mix saved in file, where there is one, and saves
// the mix there from now on.
func (a *app) loadMix(file string) {
	a.mixFile = file
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	m := synth.DefaultGMMix()
	if err := json.Unmarshal(b, &m); err != nil {
		a.say("The saved mix could not be read: " + err.Error())
		return
	}
	a.mix = m
}

// saveMix saves the mix to its file.
func (a *app) saveMix() {
	if a.mixFile == "" {
		return
	}
	b, err := json.MarshalIndent(a.mix, "", "\t")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(a.mixFile), 0o755)
	}
	if err == nil {
		err = os.WriteFile(a.mixFile, b, 0o644)
	}
	if err != nil {
		a.say("The mix could not be saved: " + err.Error())
	}
}
