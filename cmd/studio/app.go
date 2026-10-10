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
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/driver"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
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
	song  *synth.Song
	p     *synth.Player
	voice *audio.Voice
	// tryer is the same song with all its parts off, playing on a voice
	// of its own that never pauses: the kit's pads, the keyboard and the
	// keypad play through it while the song is paused.
	tryer      *synth.Player
	tryerVoice *audio.Voice
	volume     float32
	gen        int
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
	loads     chan loaded
	// focus is what the editors show, and preview and drum the sounds
	// rendered of it, with the patches they were rendered from, as JSON.
	focus     Focus
	preview   Preview
	previewOf string
	drum      Preview
	drumOf    string
	// levels are the mix's, as heard, and heardFrom the frame to read on
	// from; scope holds the watched track's sound.
	levels    *audioui.Levels
	heardFrom int64
	heardBuf  []float32
	lastTick  time.Time
	scope     []float32
	// patternSel is where the pattern editor's choice goes after a change.
	patternTrack  string
	patternSel    string
	patternSelGen int
	// editor is the editor the window is asked to show.
	editor, editorTrack string
	editorGen           int
}

// newStudio returns the studio, playing through mix, with the song
// named songName open, or the first.
func newStudio(c gunim.Client, mix *audio.Mixer, songName string) (*studio, error) {
	s := &studio{c: c, mix: mix, volume: 0.8, drafts: map[string]string{}, errs: map[string]string{},
		freqs: audioui.LogFreqs(spectrumPoints), seed: uint64(time.Now().UnixNano()), saved: make(chan string, 1), loads: make(chan loaded, 1),
		levels: audioui.NewLevels(), scope: make([]float32, 1024)}
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
	if s.tryerVoice != nil {
		s.tryerVoice.Stop(250 * time.Millisecond)
	}
	s.seed++
	s.p = synth.NewPlayer(s.song.Clone(), s.seed)
	if tier > 0 {
		s.p.SetTier(tier)
	}
	s.voice = s.mix.Play(s.p, audio.Options{Volume: s.volume, FadeIn: 150 * time.Millisecond})
	s.tryer = synth.NewPlayer(s.song.Clone(), s.seed)
	s.silence(s.song)
	s.tryerVoice = s.mix.Play(s.tryer, audio.Options{Volume: s.volume})
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
	_ = s.tryer.SetSong(next.Clone())
	s.silence(next)
	s.song = next
	return nil
}

// silence turns every part of song off in the tryer, a part an edit
// adds as well, so it plays only what is tried.
func (s *studio) silence(song *synth.Song) {
	for _, t := range song.Tracks {
		_ = s.tryer.SetPart(t.Name, band.PartOff)
	}
}

// padPlayer returns the player a pad, a key or the keypad plays through:
// the song's while it plays, so they sound with it, in time; the
// tryer's while it is paused, which plays on.
func (s *studio) padPlayer() *synth.Player {
	if s.voice.Paused() {
		return s.tryer
	}
	return s.p
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
		s.padPlayer().Key(v.Digit)
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
		s.tryerVoice.SetVolume(v.Volume, anim.Spring{Response: 0.15, Damping: 1})
	case Saved:
		go s.save(ctx, s.song.Clone())
	case SetValue:
		var x any = v.Num
		if v.IsStr {
			x = v.Str
		}
		var perr error
		err := s.edit(func(song *synth.Song) { perr = setPath(song, v.Path, x) })
		if perr != nil {
			err = perr
		}
		if err != nil {
			s.status = plain(err)
		}
	case ToggleValue:
		var perr error
		err := s.edit(func(song *synth.Song) {
			cur, set := getPath(song, v.Path)
			switch {
			case v.Seed == "":
				on, _ := cur.(bool)
				perr = setPath(song, v.Path, boolNum(!on))
			case set:
				perr = setPath(song, v.Path, nil)
			default:
				perr = setPath(song, v.Seed, v.Num)
			}
		})
		if perr != nil {
			err = perr
		}
		if err != nil {
			s.status = plain(err)
		}
	case ChorusKind:
		err := s.edit(func(song *synth.Song) {
			for _, t := range song.Tracks {
				if t.Name == v.Track {
					t.ChorusType = v.Type
					if t.Chorus == 0 {
						t.Chorus = 0.5
					}
				}
			}
		})
		if err != nil {
			s.status = plain(err)
		}
	case ClearValue:
		if err := s.edit(func(song *synth.Song) { _ = setPath(song, v.Path, nil) }); err != nil {
			s.status = plain(err)
		}
	case SetInts:
		var perr error
		err := s.edit(func(song *synth.Song) { perr = setPath(song, v.Path, slices.Clone(v.Values)) })
		if perr != nil {
			err = perr
		}
		if err != nil {
			s.status = plain(err)
		}
	case AddItem:
		if err := s.edit(func(song *synth.Song) { _ = addItem(song, v.Path) }); err != nil {
			s.status = plain(err)
		}
	case RemoveItem:
		if err := s.edit(func(song *synth.Song) { _ = removeItem(song, v.Path, v.Index) }); err != nil {
			s.status = plain(err)
		}
	case PatchNew:
		name := s.freeName(v.Kind)
		p := newPatch(v.Kind)
		if err := s.edit(func(song *synth.Song) { song.Patches[name] = p }); err != nil {
			s.status = plain(err)
			break
		}
		s.status = "Made the patch " + name
		s.gen++
	case PatchCopy:
		src, ok := s.song.Patches[v.Name]
		if !ok {
			break
		}
		name := s.freeName(v.Name)
		cp := s.song.Clone().Patches[v.Name]
		_ = src
		if err := s.edit(func(song *synth.Song) { song.Patches[name] = cp }); err == nil {
			s.status = "Copied " + v.Name + " to " + name
			s.gen++
		}
	case PatchSave:
		if p, ok := s.song.Patches[v.Name]; ok {
			go s.savePatch(ctx, v.Name, s.song.Clone().Patches[v.Name])
			_ = p
		}
	case PatchLoad:
		go s.loadPatch(ctx, v.Name)
	case TrackPatch:
		if err := s.edit(func(song *synth.Song) {
			if t := track(song, v.Track); t != nil {
				t.Pattern = adaptPattern(t.Pattern, song.Patches[t.Patch], song.Patches[v.Patch])
				t.Patch = v.Patch
			}
		}); err != nil {
			s.status = plain(err)
		} else {
			s.gen++
		}
	case Audition:
		if v.Drum != "" {
			s.padPlayer().AuditionDrum(v.Patch, v.Drum, 0.95)
		} else {
			s.padPlayer().Audition(v.Patch, v.Pitch, 0.9, 0.45)
		}
	case PatternOp:
		t := track(s.song, v.Track)
		if t == nil {
			break
		}
		src := t.Pattern
		if d, ok := s.drafts[v.Track]; ok {
			src = d
		}
		out, at, err := applyOp(src, v)
		if err != nil {
			s.status = plain(err)
			break
		}
		s.handle(ctx, PatternEdited{Track: v.Track, Text: out})
		s.patternTrack, s.patternSel = v.Track, at
		s.patternSelGen++
		s.gen++
	case ArpSet:
		if err := s.edit(func(song *synth.Song) {
			p := song.Patches[v.Patch]
			if p == nil {
				return
			}
			switch {
			case p.Arpeggio == nil:
				p.Arpeggio = &synth.Arpeggio{Chord: true, Hz: 50}
			case !v.Chord:
				p.Arpeggio = nil
			default:
				p.Arpeggio.Chord = !p.Arpeggio.Chord
				if !p.Arpeggio.Chord && len(p.Arpeggio.Steps) == 0 {
					p.Arpeggio.Steps = []int{0, 4, 7}
				}
			}
		}); err != nil {
			s.status = plain(err)
		}
		s.gen++
	case ArpSteps:
		var steps []int
		for _, f := range strings.Fields(v.Steps) {
			n, err := strconv.Atoi(f)
			if err != nil {
				s.status = "A step is a number of semitones, as 0 4 7"
				return
			}
			steps = append(steps, n)
		}
		if err := s.edit(func(song *synth.Song) {
			if p := song.Patches[v.Patch]; p != nil && p.Arpeggio != nil {
				p.Arpeggio.Steps = steps
			}
		}); err != nil {
			s.status = plain(err)
		}
	case OpenEditor:
		s.editor, s.editorTrack = v.Editor, v.Track
		s.editorGen++
	case Focus:
		s.focus = v
		s.p.WatchTrack(v.Track)
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
		st.Songs, st.Categories = append(st.Songs, ss.Title), append(st.Categories, ss.Category)
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
		st.Notes = append(st.Notes, NoteDot{Frame: n.Frame, Len: n.Len, Track: i, Pitch: n.Pitch, Vel: n.Vel, Drum: n.Drum != "", DrumName: n.Drum})
	}
	// A chord written by numeral shows its name too, as V · B.
	for i, c := range st.Chords {
		if i < len(look.Spelled) && look.Spelled[i] != c {
			st.Chords[i] = c + " · " + look.Spelled[i]
		}
	}
	st.Spans = spans(look, song, heard)
	st.Doc = song
	st.Editor, st.EditorTrack, st.EditorGen = s.editor, s.editorTrack, s.editorGen
	st.PatternTrack, st.PatternSel, st.PatternSelGen = s.patternTrack, s.patternSel, s.patternSelGen
	for i := range st.Tracks {
		lt := look.Tracks[i]
		st.Tracks[i].Meter = Meter{Peak: lt.Peak, RMS: lt.RMS}
		st.Tracks[i].Mute, st.Tracks[i].Solo = lt.Mute, lt.Solo
		if tr := track(song, lt.Name); tr != nil {
			st.Tracks[i].Patch = tr.Patch
		}
	}
	s.previews()
	st.Preview, st.DrumPreview = s.preview, s.drum
	st.ScopeTrack = s.focus.Track
	if st.ScopeTrack != "" {
		st.Scope = slices.Clone(s.p.Scope(s.scope))
	}
	s.meter()
	for ch := range 2 {
		st.Master.Peak[ch] = float32(dbLinear(s.levels.Peak[ch]))
		st.Master.RMS[ch] = float32(dbLinear(s.levels.RMS[ch]))
	}
	st.LUFS, _ = s.levels.ShortTerm()
	st.Reduction = s.p.Reduction()
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
		case l := <-s.loads:
			s.adopt(l)
		case ev, ok := <-s.c.Intents():
			if !ok {
				return s.c.Err()
			}
			s.handle(ctx, ev.Intent)
		}
		_ = s.c.Update("studio", s.state())
	}
}

// dbLinear is a level in decibels as a gain, 1 at full scale.
func dbLinear(db float32) float64 { return math.Pow(10, float64(db)/20) }

// meter takes the mix heard since the last tick into the master's
// levels.
func (s *studio) meter() {
	now := time.Now()
	dt := now.Sub(s.lastTick)
	if s.lastTick.IsZero() || dt > time.Second {
		dt = time.Second / 30
	}
	s.lastTick = now
	var frames []float32
	frames, s.heardFrom = s.an.Heard(s.heardBuf[:0], s.heardFrom)
	s.heardBuf = frames
	if len(frames) == 0 {
		s.levels.Quiet(dt)
		return
	}
	s.levels.Take(frames, audio.SampleRate, dt)
}

// previews renders the patch and the drum the editors show, where they
// have changed since they were last rendered.
func (s *studio) previews() {
	if p, ok := s.song.Patches[s.focus.Patch]; ok && p.Kind != "drums" {
		b, _ := json.Marshal(p)
		if key := s.focus.Patch + string(b); key != s.previewOf {
			s.previewOf = key
			s.preview = Preview{Patch: s.focus.Patch}
			if wave, err := synth.PreviewPatch(p, 60, 0.9, 1.6); err == nil {
				s.preview.Wave = overview(wave, 360)
				// A few cycles of C4, once the attack is past.
				at := min(len(wave)-1, int(0.3*audio.SampleRate))
				s.preview.Cycle = resample(wave[at:min(len(wave), at+3*184)], 256)
			}
		}
	}
	if p, ok := s.song.Patches[s.focus.Kit]; ok && p.Kind == "drums" && s.focus.Drum != "" {
		b, _ := json.Marshal(p)
		if key := s.focus.Kit + "/" + s.focus.Drum + string(b); key != s.drumOf {
			s.drumOf = key
			s.drum = Preview{Patch: s.focus.Kit, Drum: s.focus.Drum}
			if wave, err := synth.PreviewDrum(p, s.focus.Drum, 1.2); err == nil {
				s.drum.Wave = overview(wave, 360)
				s.drum.Cycle = resample(wave[:min(len(wave), 2400)], 256)
			}
		}
	}
}

// overview returns x as the low and the high of each of n columns.
func overview(x []float32, n int) []float32 {
	out := make([]float32, 2*n)
	for c := range n {
		lo, hi := float32(0), float32(0)
		for _, v := range x[c*len(x)/n : (c+1)*len(x)/n] {
			lo, hi = min(lo, v), max(hi, v)
		}
		out[2*c], out[2*c+1] = lo, hi
	}
	return out
}

// resample returns x as n points, read between its samples.
func resample(x []float32, n int) []float32 {
	out := make([]float32, n)
	if len(x) < 2 {
		return out
	}
	for i := range out {
		pos := float64(i) / float64(n-1) * float64(len(x)-1)
		j := int(pos)
		f := float32(pos - float64(j))
		if j+1 < len(x) {
			out[i] = x[j] + (x[j+1]-x[j])*f
		} else {
			out[i] = x[j]
		}
	}
	return out
}

// freeName returns a name for a new patch, from base, that no patch of
// the song has.
func (s *studio) freeName(base string) string {
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if _, ok := s.song.Patches[name]; !ok {
			return name
		}
	}
}

// newPatch returns a patch of kind to start from: a synth of two saws
// through a filter, a plucked string, or a kit.
func newPatch(kind string) *synth.Patch {
	switch kind {
	case "drums":
		return &synth.Patch{Kind: "drums"}
	case "pluck":
		return &synth.Patch{Kind: "pluck", Pluck: synth.Pluck{Decay: 0.8, Bright: 0.6, Body: 0.3},
			Filter: synth.Filter{Type: "lp", Cutoff: 3000, Res: 0.1}, Amp: synth.Env{Attack: 0.001, Decay: 1, Sustain: 1, Release: 0.2},
			Poly: 8}
	}
	return &synth.Patch{
		Osc:       []synth.Osc{{Wave: "saw", Level: 0.7}, {Wave: "saw", Detune: 9, Level: 0.5}},
		Filter:    synth.Filter{Type: "lp", Cutoff: 1400, Res: 0.2, Env: 1.5},
		Amp:       synth.Env{Attack: 0.01, Decay: 0.3, Sustain: 0.7, Release: 0.25},
		FilterEnv: synth.Env{Attack: 0.005, Decay: 0.3, Sustain: 0.3, Release: 0.2},
		Poly:      8, Gain: 0.5,
	}
}

// adaptPattern keeps a track's pattern where its new patch plays it as
// the old one did, and gives it one to start from where the new patch
// plays drums and the old notes, or the other way round.
func adaptPattern(pattern string, from, to *synth.Patch) string {
	drums := func(p *synth.Patch) bool { return p != nil && p.Kind == "drums" }
	switch {
	case drums(from) == drums(to):
		return pattern
	case drums(to):
		return "bd ~ sn ~"
	}
	return "c0 c1 c2 c1"
}

// patchFiles are the files a patch saves as.
var patchFiles = []driver.FileFilter{{Name: "Patches", Patterns: []string{"*.patch.json", "*.json"}}}

// savePatch asks where to save p, named name, and saves it there.
func (s *studio) savePatch(ctx context.Context, name string, p *synth.Patch) {
	path, err := s.c.SaveFile(ctx, driver.SaveOptions{Title: "Save the patch " + name, Name: name + ".patch.json", Filters: patchFiles})
	if err != nil || path == "" {
		s.saved <- ""
		return
	}
	b, err := json.MarshalIndent(p, "", "\t")
	if err == nil {
		err = os.WriteFile(path, append(b, '\n'), 0o644)
	}
	if err != nil {
		s.saved <- "Could not save: " + err.Error()
		return
	}
	s.saved <- "Saved " + filepath.Base(path)
}

// loaded is a patch read from a file, handed back to the serve loop.
type loaded struct {
	name, file string
	p          *synth.Patch
}

// loadPatch asks for a patch's file, and hands it back to put in place
// of the patch named name, or as a new one where name is empty.
func (s *studio) loadPatch(ctx context.Context, name string) {
	paths, err := s.c.ChooseFiles(ctx, driver.ChooseOptions{Title: "Load a patch", Filters: patchFiles})
	if err != nil || len(paths) == 0 {
		return
	}
	b, err := os.ReadFile(paths[0])
	p := new(synth.Patch)
	if err == nil {
		err = json.Unmarshal(b, p)
	}
	if err != nil {
		s.saved <- "Could not load: " + err.Error()
		return
	}
	s.loads <- loaded{name: name, file: paths[0], p: p}
}

// adopt puts a patch loaded in the song.
func (s *studio) adopt(l loaded) {
	name := l.name
	if name == "" {
		base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(l.file), ".json"), ".patch")
		name = base
		if _, ok := s.song.Patches[name]; ok {
			name = s.freeName(base)
		}
	}
	if old, ok := s.song.Patches[name]; ok && (old.Kind == "drums") != (l.p.Kind == "drums") {
		s.status = "A kit loads in place of a kit, and a synth in place of a synth"
		return
	}
	if err := s.edit(func(song *synth.Song) { song.Patches[name] = l.p }); err != nil {
		s.status = plain(err)
		return
	}
	s.status = "Loaded " + filepath.Base(l.file) + " as " + name
	s.gen++
}
