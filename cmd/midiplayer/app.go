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

	"github.com/marrasen/gunim-game-audio/internal/fontcache"
	"github.com/marrasen/gunim-game-audio/midi"
	"github.com/marrasen/gunim-game-audio/soundfont"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim-game-audio/tracker"
)

// app is the application half: the playlist, the song playing and how
// it plays.
type app struct {
	c     gunim.Client
	mixer *audio.Mixer
	// list is the playlist, files its files read, and cur the one
	// playing, -1 for none.
	list  []Item
	files []songFile
	cur   int
	p     songPlayer
	voice *audio.Voice
	score *Score
	gen   int
	// ended says the song playing has ended and what follows has been
	// seen to.
	ended     bool
	style     string
	muted     [maxCh]bool
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
	// font is the SoundFont bank the soundfont style plays, and fontName
	// its name; fonts carries a bank read, or fetched, to the serve loop,
	// and fontSaved is the file that remembers which bank was last
	// played. fetching is how far a bank's download has come, from 0 to
	// 1, or -1 for none.
	font      *soundfont.Font
	fontName  string
	fonts     chan fontLoaded
	fontSaved string
	fetching  float64
	progress  chan float64
	// fx is how much the window animates, kept in prefs.
	fx    float32
	prefs string
}

// fontLoaded is a SoundFont bank read from path, or why it could not be.
type fontLoaded struct {
	f    *soundfont.Font
	path string
	err  error
}

func newApp(c gunim.Client, mix *audio.Mixer) *app {
	return &app{c: c, mixer: mix, cur: -1, solo: -1, speed: 1, volume: 0.9, style: "gm", repeat: RepeatAll,
		chosen: make(chan []string, 1), mix: synth.DefaultGMMix(), an: audio.NewAnalyzer(mix, 32),
		fonts: make(chan fontLoaded, 1), progress: make(chan float64, 8), fetching: -1, fx: 0.5,
		spec: make([]float32, len(specFreqs))}
}

// songFile is a song of the playlist, read: a MIDI file, or a
// tracker's module.
type songFile struct {
	midi *midi.File
	mod  *tracker.Module
}

// length returns how long the song plays, in seconds.
func (f songFile) length() float64 {
	if f.mod != nil {
		return tracker.Scan(f.mod).Length
	}
	return f.midi.Length()
}

// songPlayer is what plays a song: a MIDI file's player, or a module's.
type songPlayer interface {
	Read(dst []float32) (int, error)
	Length() float64
	Seek(secs float64)
	SetSpeed(speed float64)
	SetMute(ch int, mute bool)
	SetMix(m synth.GMMix)
	TakeMeters() synth.GMMeters
	TimeAt(out int64) float64
	Ended() bool
}

// midiFiles are the files the open dialog shows.
var midiFiles = []driver.FileFilter{
	{Name: "Songs", Patterns: []string{"*.mid", "*.midi", "*.kar", "*.smf", "*.mod", "*.xm"}},
	{Name: "MIDI files", Patterns: []string{"*.mid", "*.midi", "*.kar", "*.smf"}},
	{Name: "Tracker modules", Patterns: []string{"*.mod", "*.xm"}},
}

// isMIDI reports whether path is named as a song the player plays: a
// MIDI file, or a MOD or an XM, which the Amiga named mod.name.
func isMIDI(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mid", ".midi", ".kar", ".smf", ".rmi", ".mod", ".xm":
		return true
	}
	return strings.HasPrefix(strings.ToLower(filepath.Base(path)), "mod.")
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
		if isBank(p) {
			a.loadFont(p)
			continue
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
		f, name, err := readSong(p)
		if err != nil {
			a.say(fmt.Sprintf("%s is not a song I can play: %v", filepath.Base(p), err))
			continue
		}
		a.list = append(a.list, Item{Name: name, Path: p, Length: f.length(), Tracker: f.mod != nil})
		a.files = append(a.files, f)
	}
	if len(found) == 0 && len(paths) > 0 && a.fetching < 0 && !anyBank(paths) {
		a.say("No songs there")
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
	a.files = append(a.files, songFile{midi: f})
	a.start(len(a.list) - 1)
}

// readSong reads the song at path, a MIDI file or a module, by its name
// or else by what it holds, and returns it and its name.
func readSong(path string) (songFile, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return songFile{}, "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	tryMod := ext == ".mod" || ext == ".xm" || strings.HasPrefix(strings.ToLower(filepath.Base(path)), "mod.")
	readMod := func() (songFile, string, error) {
		m, err := tracker.Read(bytes.NewReader(b))
		if err != nil {
			return songFile{}, "", err
		}
		name := strings.TrimSpace(m.Title)
		if name == "" || strings.EqualFold(name, "untitled") {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		return songFile{mod: m}, name, nil
	}
	if tryMod {
		if f, name, err := readMod(); err == nil {
			return f, name, nil
		}
	}
	f, err := readMIDI(path)
	if err == nil {
		return songFile{midi: f}, displayName(path, f), nil
	}
	if !tryMod {
		if sf, name, err2 := readMod(); err2 == nil {
			return sf, name, nil
		}
	}
	return songFile{}, "", err
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
	if f.mod != nil {
		tp := tracker.NewPlayer(f.mod)
		a.p = tp
		a.score = newTrackerScore(f.mod, tp.Timeline(), a.list[i].Name)
	} else {
		mp := synth.NewMIDIPlayer(f.midi)
		mp.SetSoundFont(a.font)
		_ = mp.SetStyle(a.style)
		mp.SetTranspose(a.transpose)
		a.p = mp
		a.score = newScore(f.midi, a.list[i].Name)
	}
	a.p.SetSpeed(a.speed)
	a.p.SetMix(a.mix)
	a.applyMutes()
	a.gen++
	a.voice = a.mixer.Play(a.p, audio.Options{Volume: a.volume})
}

// applyMutes sets what each channel plays: muted, or alone.
func (a *app) applyMutes() {
	if a.p == nil {
		return
	}
	for ch := range maxCh {
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
	s.Font, s.Fetching, s.FX = a.fontName, a.fetching, a.fx
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
	case FontOpenAsked:
		go func() {
			paths, err := a.c.ChooseFiles(ctx, driver.ChooseOptions{Title: "Choose a SoundFont bank",
				Filters: []driver.FileFilter{{Name: "SoundFont banks", Patterns: []string{"*.sf2", "*.sf3"}}}})
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
			a.voice.Resume(0)
		default:
			a.voice.Pause(0)
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
			if mp := a.midi(); mp != nil {
				_ = mp.SetStyle(v.Style)
			}
		}
	case ChannelClicked:
		ch := min(max(v.Channel, 0), maxCh-1)
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
		if mp := a.midi(); mp != nil {
			mp.SetTranspose(a.transpose)
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
	case FetchAsked:
		a.fetch()
	case FXSet:
		a.fx = min(max(v.FX, 0), 1)
		a.mixDirty = time.Now()
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
		case fl := <-a.fonts:
			a.adoptFont(fl)
		case x := <-a.progress:
			a.fetching = x
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

// prefsFile is what prefs hold.
type prefsFile struct{ FX float32 }

// loadPrefs reads the window's preferences from file, and saves them
// there from now on.
func (a *app) loadPrefs(file string) {
	a.prefs = file
	var p prefsFile
	if b, err := os.ReadFile(file); err == nil && json.Unmarshal(b, &p) == nil {
		a.fx = min(max(p.FX, 0), 1)
	}
}

// saveMix saves the mix to its file, and the preferences to theirs.
func (a *app) saveMix() {
	if a.prefs != "" {
		if b, err := json.Marshal(prefsFile{FX: a.fx}); err == nil {
			_ = os.MkdirAll(filepath.Dir(a.prefs), 0o755)
			_ = os.WriteFile(a.prefs, b, 0o644)
		}
	}
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

// midi returns the MIDI file's player, where one plays.
func (a *app) midi() *synth.MIDIPlayer {
	mp, _ := a.p.(*synth.MIDIPlayer)
	return mp
}

// isBank reports whether path is named as a SoundFont bank.
func isBank(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sf2", ".sf3":
		return true
	}
	return false
}

func anyBank(paths []string) bool { return slices.ContainsFunc(paths, isBank) }

// loadFont reads the bank at path, away from the serve loop, as a bank
// can be large, and hands it to the loop.
func (a *app) loadFont(path string) {
	a.say("Loading the sound font " + filepath.Base(path) + "…")
	go func() {
		f, err := soundfont.Open(path)
		a.fonts <- fontLoaded{f: f, path: path, err: err}
	}()
}

// adoptFont plays the bank read from now on, in the soundfont style, and
// remembers it for the next time.
func (a *app) adoptFont(fl fontLoaded) {
	if fl.err != nil {
		a.say(fmt.Sprintf("%s is not a sound font I can play: %v", filepath.Base(fl.path), fl.err))
		return
	}
	a.font = fl.f
	a.fontName = strings.TrimSpace(fl.f.Name)
	if a.fontName == "" {
		a.fontName = strings.TrimSuffix(filepath.Base(fl.path), filepath.Ext(fl.path))
	}
	a.style = "soundfont"
	if mp := a.midi(); mp != nil {
		mp.SetSoundFont(a.font)
		_ = mp.SetStyle(a.style)
	}
	if a.fontSaved != "" {
		_ = os.MkdirAll(filepath.Dir(a.fontSaved), 0o755)
		_ = os.WriteFile(a.fontSaved, []byte(fl.path), 0o644)
	}
	a.say("Playing on the sound font " + a.fontName)
}

// rememberFont reads the bank last played, as saved in file, and saves
// the next there.
func (a *app) rememberFont(file string) {
	a.fontSaved = file
	if b, err := os.ReadFile(file); err == nil {
		if path := strings.TrimSpace(string(b)); path != "" {
			if _, err := os.Stat(path); err == nil {
				a.loadFont(path)
			}
		}
	}
}

// fetch downloads the first of soundfont.Banks, MuseScore General, and
// its licence, to the user's cache, and plays on it.
func (a *app) fetch() {
	if a.fetching >= 0 {
		return
	}
	bank := soundfont.Banks[0]
	if path, ok := fontcache.Fetched(bank); ok {
		a.loadFont(path)
		return
	}
	a.fetching = 0
	a.say(fmt.Sprintf("Fetching %s (%s licence, %d MB)…", bank.Name, bank.License, bank.Size>>20))
	go func() {
		path, err := fontcache.Download(bank, func(x float64) {
			select {
			case a.progress <- x:
			default:
			}
		})
		a.progress <- -1
		if err != nil {
			a.fonts <- fontLoaded{path: path, err: fmt.Errorf("could not fetch it: %w", err)}
			return
		}
		f, err := soundfont.Open(path)
		a.fonts <- fontLoaded{f: f, path: path, err: err}
	}()
}
