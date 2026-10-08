package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"path/filepath"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/calls"
)

// lab is what the window works on: the library, the companion open, its
// calls' takes, and what plays.
type lab struct {
	c   gunim.Client
	mix *audio.Mixer
	lib *calls.Library
	// dir is where the library saves to, and embedded says it was read
	// from the program, there being no folder yet.
	dir      string
	embedded bool
	comp     *calls.Companion
	sel      int
	takes    []*calls.Take
	seeds    []uint64
	// voice plays a call, call is which, and queue is the calls still
	// to play after it.
	voice *audio.Voice
	call  int
	// looping says the call playing is a loop, played over and over.
	looping bool
	queue   []int
	gap     time.Time
	phone   bool
	// songs are the songs' names, song the one under the calls, 0 for
	// none, and music its voice.
	songs    []string
	song     int
	music    *audio.Voice
	musicVol float32
	dirty    map[string]bool
	status   string
	gen      int
}

// newLab opens the library in dir, or the program's own where dir does
// not exist yet, saving to dir.
func newLab(c gunim.Client, mix *audio.Mixer, dir string) (*lab, error) {
	l := &lab{c: c, mix: mix, dir: dir, musicVol: 0.35, dirty: map[string]bool{}, call: -1}
	lib, err := calls.LoadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		lib, err = music.Calls()
		l.embedded = true
	}
	if err != nil {
		return nil, err
	}
	l.lib = lib
	l.songs = music.Songs()
	l.open(lib.Companions[0].ID)
	abs, _ := filepath.Abs(dir)
	if l.embedded {
		l.status = "Read the calls built into the program. Save writes them to " + abs + "."
	} else {
		l.status = "Read the calls from " + abs + "."
	}
	return l, nil
}

// open opens companion id, its calls as set, the first made chosen.
func (l *lab) open(id string) {
	c := l.lib.Companion(id)
	if c == nil {
		return
	}
	l.comp = c
	l.seeds = make([]uint64, len(c.Kinds()))
	l.takes = make([]*calls.Take, len(c.Kinds()))
	l.sel = 0
	for i, kind := range l.kinds() {
		if _, ok := c.Calls[kind]; ok {
			l.sel = i
			break
		}
	}
	l.remakeAll()
	l.gen++
}

// kinds are the calls the companion open makes: a boss's or a
// companion's.
func (l *lab) kinds() []string {
	if l.comp == nil {
		return calls.Kinds
	}
	return l.comp.Kinds()
}

// has says whether the character open makes a call i.
func (l *lab) has(i int) bool { return i >= 0 && i < len(l.takes) }

// remake makes call i's take afresh, from its seed.
func (l *lab) remake(i int) {
	l.takes[i] = nil
	call, ok := l.comp.Calls[l.kinds()[i]]
	if !ok {
		return
	}
	l.takes[i] = l.lib.Make(call, l.seeds[i])
}

func (l *lab) remakeAll() {
	for i := range l.kinds() {
		l.remake(i)
	}
}

// play plays call i's take, stopping any call playing.
func (l *lab) play(i int) {
	if !l.has(i) {
		return
	}
	t := l.takes[i]
	if t == nil || len(t.Samples) == 0 {
		return
	}
	// A loop plays over and over, as the game plays it, till played
	// again, or another call plays; in a run of all the calls, once.
	loops := t.Stats.Loop && len(l.queue) == 0
	if l.voice != nil {
		wasLoop := l.looping && l.call == i
		l.voice.Stop(10 * time.Millisecond)
		l.voice = nil
		if wasLoop && loops {
			return
		}
	}
	o := audio.Options{Loop: loops}
	if l.phone {
		o.Insert = calls.NewPhone()
	}
	l.voice = l.mix.Play(t.Clip().Source(), o)
	l.call, l.looping = i, loops
}

// playing says whether a call plays, and how far through it is.
func (l *lab) playing() (int, float32) {
	if l.voice == nil {
		return -1, -1
	}
	select {
	case <-l.voice.Done():
		l.voice = nil
		return -1, -1
	default:
	}
	n := l.voice.Len()
	if n <= 0 {
		return l.call, 0
	}
	at := float32(l.voice.Position()) / float32(n)
	if l.looping {
		return l.call, at - float32(int(at))
	}
	return l.call, min(at, 1)
}

// tick plays the next call queued once the one playing has ended, a
// breath after it, so each is heard apart.
func (l *lab) tick() {
	if len(l.queue) == 0 {
		return
	}
	if i, _ := l.playing(); i >= 0 {
		l.gap = time.Now().Add(300 * time.Millisecond)
		return
	}
	if time.Now().Before(l.gap) {
		return
	}
	next := l.queue[0]
	l.queue = l.queue[1:]
	l.play(next)
}

// current returns the call the editor shows, or nil where it is not
// made yet.
func (l *lab) current() *calls.Call { return l.comp.Calls[l.kinds()[l.sel]] }

// changed marks the companion changed, and remakes call i.
func (l *lab) changed(i int) {
	l.dirty[l.comp.ID] = true
	l.remake(i)
}

// handle obeys an intent.
func (l *lab) handle(in gunim.Intent) {
	switch v := in.(type) {
	case CompanionChosen:
		l.queue = nil
		l.open(v.ID)
	case CallChosen:
		if l.has(v.Call) {
			l.sel = v.Call
			l.gen++
		}
	case PlayCall:
		l.queue = nil
		l.play(v.Call)
	case NewTake:
		// The window may ask for a call of the character open before;
		// a boss's seventh, say, after a companion was picked.
		if !l.has(v.Call) {
			return
		}
		l.queue = nil
		l.seeds[v.Call] = rand.Uint64() | 1
		l.remake(v.Call)
		l.play(v.Call)
	case AsSet:
		if !l.has(v.Call) {
			return
		}
		l.queue = nil
		l.seeds[v.Call] = 0
		l.remake(v.Call)
		l.play(v.Call)
	case PlayAll:
		l.queue = nil
		for i := range l.kinds() {
			if l.takes[i] != nil {
				l.queue = append(l.queue, i)
			}
		}
		if l.voice != nil {
			l.voice.Stop(10 * time.Millisecond)
			l.voice = nil
		}
		l.tick()
	case ParamSet:
		l.set(v)
	case LayerAdded:
		call := l.current()
		if call == nil {
			call = &calls.Call{Vary: 0.5, Room: 0.15}
			l.comp.Calls[l.kinds()[l.sel]] = call
		}
		call.Layers = append(call.Layers, &calls.Layer{Model: v.Model, Params: map[string]float64{}})
		l.changed(l.sel)
		l.gen++
	case LayerRemoved:
		call := l.current()
		if call == nil || v.Layer < 0 || v.Layer >= len(call.Layers) {
			return
		}
		call.Layers = append(call.Layers[:v.Layer], call.Layers[v.Layer+1:]...)
		if len(call.Layers) == 0 {
			delete(l.comp.Calls, l.kinds()[l.sel])
		}
		l.changed(l.sel)
		l.gen++
	case ModelChosen:
		call := l.current()
		if call == nil || v.Layer < 0 || v.Layer >= len(call.Layers) || call.Layers[v.Layer].Model == v.Model {
			return
		}
		call.Layers[v.Layer] = &calls.Layer{Model: v.Model, At: call.Layers[v.Layer].At, Gain: call.Layers[v.Layer].Gain, Params: map[string]float64{}}
		l.changed(l.sel)
		l.gen++
		l.play(l.sel)
	case NotesSet:
		if call := l.current(); call != nil && call.Notes != v.Text {
			call.Notes = v.Text
			l.dirty[l.comp.ID] = true
		}
	case Saved:
		if err := l.lib.Save(l.dir, l.comp); err != nil {
			l.status = "Could not save: " + err.Error()
			return
		}
		l.dirty[l.comp.ID] = false
		l.embedded = false
		abs, _ := filepath.Abs(filepath.Join(l.dir, l.comp.ID+".json"))
		l.status = fmt.Sprintf("Saved %s at %s.", abs, time.Now().Format("15:04"))
	case Reverted:
		lib, err := calls.LoadDir(l.dir)
		if err != nil {
			l.status = "Could not read the saved calls: " + err.Error()
			return
		}
		fresh := lib.Companion(l.comp.ID)
		for i, c := range l.lib.Companions {
			if c.ID == fresh.ID {
				l.lib.Companions[i] = fresh
			}
		}
		l.lib.Master = lib.Master
		l.dirty[fresh.ID] = false
		sel := l.sel
		l.open(fresh.ID)
		l.sel = sel
		l.status = "Read " + fresh.Name + " back from its file."
	case PhoneSet:
		l.phone = v.On
	case SongChosen:
		l.playSong(v.Song)
	case MusicVolumeSet:
		l.musicVol = v.Volume
		if l.music != nil {
			l.music.SetVolume(v.Volume, anim.Spring{Response: 0.2, Damping: 1})
		}
	}
}

// set sets a number, and remakes what it changes.
func (l *lab) set(v ParamSet) {
	switch {
	case v.Layer == -2:
		m := &l.lib.Master
		switch v.Name {
		case "highpass":
			m.HighPass = v.Value
		case "loudness":
			m.Loudness = v.Value
		case "ceiling":
			m.Ceiling = v.Value
		}
		// The master is the library's, saved with each companion.
		l.dirty[l.comp.ID] = true
		l.remakeAll()
	case v.Layer == -1:
		call := l.current()
		if call == nil {
			return
		}
		switch v.Name {
		case "vary":
			call.Vary = v.Value
		case "room":
			call.Room = v.Value
		case "cut":
			call.Cut = v.Value
		case "presence":
			call.Presence = v.Value
		case "loop":
			call.Loop = v.Value
		}
		l.changed(l.sel)
	default:
		call := l.current()
		if call == nil || v.Layer >= len(call.Layers) {
			return
		}
		ly := call.Layers[v.Layer]
		switch v.Name {
		case "@at":
			ly.At = v.Value
		case "@gain":
			ly.Gain = v.Value
		case "@ring":
			ly.Ring = v.Value
		case "@ringmix":
			ly.RingMix = v.Value
		default:
			if ly.Params == nil {
				ly.Params = map[string]float64{}
			}
			ly.Params[v.Name] = v.Value
		}
		l.changed(l.sel)
	}
	if v.Done {
		l.play(l.sel)
	}
}

// playSong plays song i under the calls, or none for 0.
func (l *lab) playSong(i int) {
	if l.music != nil {
		l.music.Stop(500 * time.Millisecond)
		l.music = nil
	}
	l.song = 0
	if i <= 0 || i > len(l.songs) {
		return
	}
	s, err := music.Song(l.songs[i-1])
	if err != nil {
		l.status = err.Error()
		return
	}
	l.song = i
	l.music = l.mix.Play(s.Play(rand.Uint64()), audio.Options{Volume: l.musicVol, FadeIn: time.Second})
}

// state returns what the window shows.
func (l *lab) state() Lab {
	m := l.lib.Master
	s := Lab{
		Name: l.comp.Name, About: l.comp.Style + " · " + l.comp.Character,
		Selected: l.sel, HighPass: m.HighPass, Loudness: m.Loudness, Ceiling: m.Ceiling,
		Phone: l.phone, Song: l.song, MusicVolume: l.musicVol, Status: l.status, Gen: l.gen,
		Dirty: l.dirty[l.comp.ID],
	}
	s.Songs = append([]string{"No music"}, l.songs...)
	for i, c := range l.lib.Companions {
		row := CompanionRow{ID: c.ID, Name: c.Name, Made: len(c.Calls) > 0, Open: c == l.comp}
		what := "calls"
		if c.Role == calls.Effects {
			what = "sounds"
		}
		row.Doing = fmt.Sprintf("%d of %d %s", len(c.Calls), len(c.Kinds()), what)
		if len(c.Calls) == 0 {
			row.Doing = "not made yet"
		}
		if l.dirty[c.ID] {
			row.Doing += " · changed"
		}
		s.Companions = append(s.Companions, row)
		if c == l.comp {
			s.Companion = i
		}
	}
	at, head := l.playing()
	for i, kind := range l.kinds() {
		cv := CallView{Kind: kind, Playhead: -1}
		if t := l.takes[i]; t != nil {
			cv.Made = true
			cv.Wave = spans(t.Samples, 220)
			cv.Spectrum = calls.Spectrum(t.Samples, 64)
			st := t.Stats
			phone := fmt.Sprintf("%.1f dB quieter on a phone", st.Phone)
			if st.Phone < 0 {
				phone = fmt.Sprintf("%.1f dB louder on a phone", -st.Phone)
			}
			cv.Stats = fmt.Sprintf("%.2f s · %.1f LUFS · peak %.1f dBTP · limited %.1f dB\n%s · 1–4 kHz %.0f%% · under 300 Hz %.0f%%",
				st.Length, st.Loudness, st.Peak, st.Limited, phone, st.Presence*100, st.Lows*100)
			cv.Problems = st.Problems(kind, m)
			cv.Take = "As set"
			if t.Seed != 0 {
				cv.Take = fmt.Sprintf("Take %04d", t.Seed%10000)
			}
			if i == at {
				cv.Playhead = head
			}
		}
		s.Calls = append(s.Calls, cv)
	}
	s.Editor = l.editor()
	return s
}

// editor returns the selected call's settings.
func (l *lab) editor() Editor {
	e := Editor{Kind: l.kinds()[l.sel], Models: calls.ModelNames()}
	call := l.current()
	if call == nil {
		return e
	}
	e.Vary, e.Room, e.Cut, e.Presence, e.Loop, e.Notes = call.Vary, call.Room, call.Cut, call.Presence, call.Loop, call.Notes
	for _, ly := range call.Layers {
		m := calls.Models()[ly.Model]
		lv := LayerView{Model: ly.Model, About: m.About}
		for _, p := range m.Params {
			lv.Params = append(lv.Params, ParamView{Name: p.Name, Label: p.Label, Unit: p.Unit, About: p.About,
				Lo: p.Lo, Hi: p.Hi, Def: p.Def, Value: ly.Get(p.Name), Step: p.Step, Log: p.Log, Choices: p.Choices, Odd: p.Odd, Zero: p.Zero})
		}
		lv.Params = append(lv.Params,
			ParamView{Name: "@at", Label: "Starts", Unit: "s", About: "When the layer starts in the call", Lo: 0, Hi: 1, Value: ly.At},
			ParamView{Name: "@gain", Label: "Level", Unit: "dB", About: "The layer's level", Lo: -24, Hi: 12, Value: ly.Gain},
			ParamView{Name: "@ring", Label: "Ring", Unit: "Hz", About: "Ring-modulates the layer by a tone of this pitch, as a robot's voice", Lo: 0, Hi: 2000, Value: ly.Ring, Zero: "off"},
			ParamView{Name: "@ringmix", Label: "Ring mix", About: "How much of the ring modulation is heard", Lo: 0, Hi: 1, Def: 1, Value: ly.RingMix})
		e.Layers = append(e.Layers, lv)
	}
	return e
}

// spans returns x's lows and highs over n columns.
func spans(x []float32, n int) []float32 {
	out := make([]float32, 0, 2*n)
	for c := range n {
		a, b := c*len(x)/n, (c+1)*len(x)/n
		lo, hi := float32(0), float32(0)
		for _, v := range x[a:max(b, min(a+1, len(x)))] {
			lo, hi = min(lo, v), max(hi, v)
		}
		out = append(out, lo, hi)
	}
	return out
}

// serve runs the lab: it takes the window's intents, plays, and shows
// what changed, ten times a second, or 30 while a call plays.
func (l *lab) serve(ctx context.Context) error {
	if err := l.c.Mount(gunim.Root, "lab", "lab", l.state()); err != nil {
		return err
	}
	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			l.tick()
			// Show the playhead while a call plays, and now and then
			// otherwise.
			if i, _ := l.playing(); i < 0 && time.Since(last) < 250*time.Millisecond {
				continue
			}
		case ev, ok := <-l.c.Intents():
			if !ok {
				return l.c.Err()
			}
			l.handle(ev.Intent)
			// A knob turned fast sends many numbers: take those waiting
			// before showing the result.
			for more := true; more; {
				select {
				case ev, ok := <-l.c.Intents():
					if !ok {
						return l.c.Err()
					}
					l.handle(ev.Intent)
				default:
					more = false
				}
			}
		}
		last = time.Now()
		_ = l.c.Update("lab", l.state())
	}
}
