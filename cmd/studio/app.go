package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/driver"

	music "github.com/marrasen/gunim-music"
	"github.com/marrasen/gunim-music/synth"
)

// spectrumPoints is how many frequencies the spectrum shows.
const spectrumPoints = 96

// studio is the application half: the songs, the one open as edited,
// and what plays it.
type studio struct {
	c     gunim.Client
	mix   *audio.Mixer
	an    *audio.Analyzer
	freqs []float32
	names []string
	songs []*synth.Song
	i     int
	// song is the song open, as edited: each edit makes a new one, as
	// the player keeps the one it was handed.
	song   *synth.Song
	p      *synth.Player
	voice  *audio.Voice
	volume float32
	gen    int
	// drafts are patterns typed that do not play, by track, and errs
	// why not.
	drafts    map[string]string
	errs      map[string]string
	chordText string
	chordErr  string
	status    string
	notes     []synth.Note
	seed      uint64
	saved     chan string
}

// newStudio returns the studio, playing through mix, with the song
// named songName open, or the first.
func newStudio(c gunim.Client, mix *audio.Mixer, songName string) (*studio, error) {
	s := &studio{c: c, mix: mix, volume: 0.8, drafts: map[string]string{}, errs: map[string]string{},
		freqs: audioui.LogFreqs(spectrumPoints), seed: uint64(time.Now().UnixNano()), saved: make(chan string, 1)}
	s.an = audio.NewAnalyzer(s.mix, 16)
	for _, name := range music.Songs() {
		song, err := music.Song(name)
		if err != nil {
			return nil, err
		}
		if ss, ok := song.(*synth.Song); ok {
			s.names, s.songs = append(s.names, name), append(s.songs, ss)
			if name == songName {
				s.i = len(s.songs) - 1
			}
		}
	}
	if len(s.songs) == 0 {
		return nil, errors.New("studio: the library holds no songs made in code")
	}
	s.open(s.i)
	return s, nil
}

// open opens song i afresh, as the library has it.
func (s *studio) open(i int) {
	s.i = i
	s.song = s.songs[i].Clone()
	clear(s.drafts)
	clear(s.errs)
	s.chordErr = ""
	s.start()
}

// start plays the song open from its start.
func (s *studio) start() {
	tier := 0
	if s.p != nil {
		tier = s.p.Tier()
	}
	if s.voice != nil {
		s.voice.Stop(250 * time.Millisecond)
	}
	s.seed++
	s.p = synth.NewPlayer(s.song.Clone(), s.seed)
	if tier > 0 {
		s.p.SetTier(tier)
	}
	s.voice = s.mix.Play(s.p, audio.Options{Volume: s.volume, FadeIn: 150 * time.Millisecond})
	s.chordText = strings.Join(s.song.Chords, " ")
	s.gen++
}

// edit changes the song open with fn, and hands it to the player: from
// the next block on, a patch or a level is heard as edited, and a
// pattern from the next bar. It returns what keeps the edit from playing.
func (s *studio) edit(fn func(*synth.Song)) error {
	next := s.song.Clone()
	fn(next)
	if err := s.p.SetSong(next.Clone()); err != nil {
		return err
	}
	s.song = next
	return nil
}

// track returns the song's track named name.
func track(song *synth.Song, name string) *synth.Track {
	for _, t := range song.Tracks {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// The filter slider runs from 0 at 20 Hz to 1 at 20 kHz, ten octaves.
func cutoffAt(x float32) float64 { return 20 * math.Exp2(float64(x)*10) }

func filterAt(hz float64) float32 {
	if hz <= 0 {
		return 1
	}
	return float32(min(max(math.Log2(hz/20)/10, 0), 1))
}

// filterOf returns where a track's filter slider stands.
func filterOf(song *synth.Song, t *synth.Track) float32 {
	p := song.Patches[t.Patch]
	if p != nil && p.Kind != "drums" && p.Filter.Type != "" && p.Filter.Type != "none" {
		return filterAt(p.Filter.Cutoff)
	}
	return filterAt(t.LPF)
}

// setFilter sets a track's filter: its patch's cutoff, for a synth
// patch with a filter, or its lowpass.
func setFilter(song *synth.Song, t *synth.Track, x float32) {
	p := song.Patches[t.Patch]
	if p != nil && p.Kind != "drums" && p.Filter.Type != "" && p.Filter.Type != "none" {
		p.Filter.Cutoff = cutoffAt(x)
		return
	}
	if x >= 0.999 {
		t.LPF = 0
	} else {
		t.LPF = cutoffAt(x)
	}
}

// handle carries out an intent.
func (s *studio) handle(ctx context.Context, v gunim.Intent) {
	switch v := v.(type) {
	case SongChosen:
		if v.Song != s.i && v.Song >= 0 && v.Song < len(s.songs) {
			s.open(v.Song)
			s.status = "Opened " + s.song.Title
		}
	case PlayToggled:
		if s.voice.Paused() {
			s.voice.Resume()
		} else {
			s.voice.Pause()
		}
	case Restarted:
		s.start()
	case TierChosen:
		s.p.SetTier(v.Tier)
	case TierFollowed:
		for _, t := range s.song.Tracks {
			_ = s.p.SetPart(t.Name, band.PartAuto)
		}
	case PartClicked:
		s.toggle(v.Name)
	case StingPlayed:
		if err := s.p.Sting(v.Name); err != nil {
			s.status = err.Error()
		} else {
			s.status = "Playing the " + v.Name + " sting; Restart plays the song again"
		}
	case KeyPlayed:
		s.p.Key(v.Digit)
	case PatternEdited:
		err := s.edit(func(song *synth.Song) {
			if t := track(song, v.Track); t != nil {
				t.Pattern = v.Text
			}
		})
		if err != nil {
			s.drafts[v.Track], s.errs[v.Track] = v.Text, plain(err)
		} else {
			delete(s.drafts, v.Track)
			delete(s.errs, v.Track)
		}
	case KnobSet:
		_ = s.edit(func(song *synth.Song) {
			t := track(song, v.Track)
			if t == nil {
				return
			}
			switch v.Knob {
			case "gain":
				t.Gain = float64(v.Value)
			case "filter":
				setFilter(song, t, v.Value)
			case "reverb":
				t.Reverb = float64(v.Value)
			case "delay":
				t.Delay = float64(v.Value)
			}
		})
	case ChordsEdited:
		s.chordText = v.Text
		chords := strings.Fields(strings.NewReplacer(",", " ", "|", " ", "-", " ").Replace(v.Text))
		err := s.edit(func(song *synth.Song) { song.Chords = chords })
		s.chordErr = ""
		if len(chords) == 0 {
			s.chordErr = "Type a chord or more, as Am F C G or vi IV I V"
		} else if err != nil {
			s.chordErr = plain(err)
		}
	case ChordsGenerated:
		n := max(len(s.song.Chords), 4)
		s.seed++
		chords, err := synth.GenerateChords(v.Style, n, s.seed, 0.3)
		if err == nil {
			err = s.edit(func(song *synth.Song) { song.Chords = chords })
		}
		if err != nil {
			s.status = plain(err)
			break
		}
		s.chordText, s.chordErr = strings.Join(chords, " "), ""
		s.status = fmt.Sprintf("Wrote a %s progression: %s", v.Style, s.chordText)
		s.gen++
	case BPMSet:
		if v.BPM >= 40 && v.BPM <= 240 {
			_ = s.edit(func(song *synth.Song) { song.BPM = v.BPM })
		}
	case Transposed:
		if err := s.edit(func(song *synth.Song) { song.Transpose(v.By) }); err == nil {
			s.chordText = strings.Join(s.song.Chords, " ")
			s.status = "Now in " + s.song.KeyName()
			s.gen++
		}
	case VolumeSet:
		s.volume = v.Volume
		s.voice.SetVolume(v.Volume, anim.Spring{Response: 0.15, Damping: 1})
	case Saved:
		go s.save(ctx, s.song.Clone())
	}
}

// toggle starts the part named name where it rests or is leaving, and
// stops it where it plays, from the next phrase on.
func (s *studio) toggle(name string) {
	for _, t := range s.p.Look(s.heard()).Tracks {
		if t.Name != name {
			continue
		}
		on := t.Playing && t.Piece != band.Outro
		switch t.Control {
		case band.PartOn:
			on = true
		case band.PartOff:
			on = false
		case band.PartAuto:
		}
		c := band.PartOn
		if on {
			c = band.PartOff
		}
		_ = s.p.SetPart(name, c)
	}
}

// save asks where to save song, and saves it there as a song.json.
func (s *studio) save(ctx context.Context, song *synth.Song) {
	path, err := s.c.SaveFile(ctx, driver.SaveOptions{Title: "Save the song", Name: "song.json"})
	if err != nil || path == "" {
		s.saved <- ""
		return
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "\t")
	err = enc.Encode(struct {
		Kind string
		*synth.Song
	}{"synth", song})
	if err == nil {
		err = os.WriteFile(path, b.Bytes(), 0o644)
	}
	if err != nil {
		s.saved <- "Could not save: " + err.Error()
		return
	}
	s.saved <- "Saved " + filepath.Base(path)
}

// plain trims the package's name from an error, for a person to read.
func plain(err error) string {
	m := err.Error()
	for _, p := range []string{"synth: ", "synth: "} {
		m = strings.ReplaceAll(m, p, "")
	}
	if m != "" {
		m = strings.ToUpper(m[:1]) + m[1:]
	}
	return m
}

// heard returns the frame of the song being heard.
func (s *studio) heard() int64 { return audio.Frames(s.voice.Position()) }

// state returns what the window shows.
func (s *studio) state() Studio {
	heard := s.heard()
	look := s.p.Look(heard)
	song := s.song
	st := Studio{Song: s.i, Gen: s.gen, Title: song.Title, BPM: song.BPM, Key: song.KeyName(), Volume: s.volume,
		Playing: !s.voice.Paused(), BarFrames: look.BarFrames, Beats: look.BeatsPerBar, Bar: look.Bar,
		BarFrame: look.BarFrame, PhraseBars: look.PhraseBars, Tiers: look.Tiers, Tier: s.p.Tier(),
		Chords: slices.Clone(look.Chords), Chord: look.ChordIndex, ChordText: s.chordText, ChordErr: s.chordErr, Stings: look.Stings, Sting: look.Sting,
		Stopped: look.Stopped, Keypad: look.Keypad, Status: s.status}
	if look.Wander {
		st.Tiers = 0
	}
	for _, ss := range s.songs {
		st.Songs = append(st.Songs, ss.Title)
	}
	st.About = fmt.Sprintf("By %s · %s · %g BPM", song.Artist, song.KeyName(), song.BPM)
	st.Clock = Clock{Frame: heard, At: time.Now()}
	if st.Playing && !look.Stopped {
		st.Clock.Rate = audio.SampleRate
	}
	index := map[string]int{}
	for i, t := range look.Tracks {
		index[t.Name] = i
		row := TrackRow{Gen: s.gen, Name: t.Name, Color: t.Color, Tier: t.Tier, Core: t.Core, Drums: t.Drums, Playing: t.Playing,
			Level: t.Level, Doing: doing(t)}
		if tr := track(song, t.Name); tr != nil {
			row.Pattern = tr.Pattern
			if d, ok := s.drafts[t.Name]; ok {
				row.Pattern = d
			}
			row.PatternErr = s.errs[t.Name]
			row.Gain, row.Reverb, row.Delay = float32(tr.Gain), float32(tr.Reverb), float32(tr.Delay)
			row.Filter = filterOf(song, tr)
		}
		st.Tracks = append(st.Tracks, row)
	}
	bar := int64(look.BarFrames)
	s.notes = s.p.Notes(s.notes[:0], heard-3*bar)
	for _, n := range s.notes {
		if n.Frame > heard+2*bar {
			continue
		}
		i, ok := index[n.Track]
		if !ok {
			i = -1
		}
		st.Notes = append(st.Notes, NoteDot{Frame: n.Frame, Len: n.Len, Track: i, Pitch: n.Pitch, Vel: n.Vel, Drum: n.Drum != ""})
	}
	// A chord written by numeral shows its name too, as V · B.
	for i, c := range st.Chords {
		if i < len(look.Spelled) && look.Spelled[i] != c {
			st.Chords[i] = c + " · " + look.Spelled[i]
		}
	}
	st.Spans = spans(look, song, heard)
	st.Spectrum = make([]float32, len(s.freqs))
	s.an.Spectrum(s.freqs, st.Spectrum, nil)
	return st
}

// doing says what a track plays, for its card.
func doing(t synth.TrackLook) string {
	d := "resting"
	if t.Playing {
		switch t.Piece {
		case band.Intro:
			d = "coming in"
		case band.Outro:
			d = "leaving"
		default:
			d = "playing"
		}
	}
	switch t.Control {
	case band.PartOn:
		d += " · on"
	case band.PartOff:
		d += " · off"
	case band.PartAuto:
	}
	return d
}

// spans lays out the chords about the frame heard: from two bars before
// it to two after, each by its place in the progression from the one
// heard.
func spans(look synth.Look, song *synth.Song, heard int64) []ChordSpan {
	n := len(look.Chords)
	if n == 0 || look.BarFrames <= 0 {
		return nil
	}
	cb := song.ChordBars
	if cb <= 0 || look.Sting != "" {
		cb = 1
	}
	pos := float64(look.Bar) + float64(heard-look.BarFrame)/look.BarFrames
	now := int(math.Floor(pos/cb + 1e-9))
	var out []ChordSpan
	for step := int(math.Floor((pos-2)/cb)) - 1; float64(step)*cb <= pos+2; step++ {
		at := float64(step)*cb - float64(look.Bar)
		idx := ((look.ChordIndex+step-now)%n + n) % n
		name := look.Chords[idx]
		if idx < len(look.Spelled) && look.Spelled[idx] != name {
			name += " · " + look.Spelled[idx]
		}
		out = append(out, ChordSpan{Frame: look.BarFrame + int64(at*look.BarFrames), Len: int64(cb * look.BarFrames),
			Name: name, Index: idx})
	}
	return out
}

// serve runs the application half until ctx ends or the window closes.
func (s *studio) serve(ctx context.Context) error {
	if err := s.c.Mount(gunim.Root, "studio", "studio", s.state()); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second / 30)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		case msg := <-s.saved:
			if msg != "" {
				s.status = msg
			}
		case ev, ok := <-s.c.Intents():
			if !ok {
				return s.c.Err()
			}
			s.handle(ctx, ev.Intent)
		}
		_ = s.c.Update("studio", s.state())
	}
}
