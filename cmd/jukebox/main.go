// Command jukebox is a window to try the library's songs in: pick a
// song, play and pause it, set its tier where it has tiers, and watch
// what each part plays, bar by bar. In a song whose parts start and
// stop one by one, a click on a part starts it, or stops it with its
// outro, at the next phrase.
//
//	go run github.com/marrasen/gunim-music/cmd/jukebox@latest
//
// The keys 1 to 9 set the tier too.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
	"github.com/marrasen/gunim/audio/speaker"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"

	music "github.com/marrasen/gunim-music"
)

// The vocabulary the two halves share.
type (
	// Jukebox is the state the window shows.
	Jukebox struct {
		// Songs are the songs' titles, and Song the one chosen.
		Songs []string
		Song  int
		// About says who made the song, and its tempo.
		About   string
		Playing bool
		// Tiers is how many tiers the song has, 0 for none, and Tier
		// the one set.
		Tiers, Tier int
		// Where says where the song is: its bar and phrase.
		Where string
		Parts []PartRow
		// Triggers says the song's parts start and stop one by one.
		Triggers bool
	}
	// PartRow is a part, and what it plays.
	PartRow struct {
		Name, Doing string
		Tier        int
		Playing     bool
	}
	// PartClicked travels when a part is clicked.
	PartClicked struct{ Name string }
	// TierFollowed travels when Follow the tier is pressed.
	TierFollowed struct{}
	// SongChosen travels when a song is picked.
	SongChosen struct{ Song int }
	// PlayToggled travels when Play or Pause is pressed.
	PlayToggled struct{}
	// Restarted travels when Restart is pressed.
	Restarted struct{}
	// TierChosen travels when a tier is picked, from 1.
	TierChosen struct{ Tier int }
	// VolumeSet travels as the volume slider moves.
	VolumeSet struct{ Volume float32 }
)

func init() {
	gunim.RegisterType[Jukebox]("jukebox.state")
	gunim.RegisterType[SongChosen]("jukebox.song")
	gunim.RegisterType[PlayToggled]("jukebox.play")
	gunim.RegisterType[Restarted]("jukebox.restart")
	gunim.RegisterType[TierChosen]("jukebox.tier")
	gunim.RegisterType[VolumeSet]("jukebox.volume")
	gunim.RegisterType[PartClicked]("jukebox.part")
	gunim.RegisterType[TierFollowed]("jukebox.follow")
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim-music jukebox",
			Size:  geom.Sz(460, 600),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		w.RegisterTheme(widget.Light())
		gunim.RegisterView(w, "jukebox", buildView, (*keys).update)
		return serve(ctx, w.Client())
	})
}

// view is the window's view, with handles on what updates change.
type view struct {
	*widget.Pad
	songs  *widget.Dropdown
	about  *widget.Label
	play   *widget.Button
	tiers  *widget.Segmented
	tierUI *widget.Label
	where  *widget.Label
	hint   *widget.Label
	follow *widget.Button
	parts  *widget.List
	shown  int
}

// keys is the view's root: it hears the keys 1 to 9, for the tiers.
type keys struct {
	*view
}

func buildView(s Jukebox) *keys {
	title := widget.NewLabel("Jukebox")
	title.Size = widget.HeadingSize
	v := &view{shown: -1}
	v.songs = widget.NewDropdown(s.Songs...)
	v.songs.Label = "Song"
	v.songs.OnChange = func(i int) gunim.Intent { return SongChosen{Song: i} }
	v.about = widget.NewLabel(s.About)
	v.about.Color = widget.MenuHint
	v.play = widget.NewButton("Pause")
	v.play.Icon, v.play.Kind, v.play.On = icon.Pause, widget.ButtonPrimary, PlayToggled{}
	restart := widget.NewButton("Restart")
	restart.Icon, restart.On = icon.RotateCcw, Restarted{}
	volume := widget.NewSlider(0, 1)
	volume.Set(1)
	volume.OnChange = func(x float32) gunim.Intent { return VolumeSet{Volume: x} }
	transport := widget.Row(v.play, restart, widget.NewLabel("Volume"), volume).Grow(volume, 1)
	transport.Cross = widget.CrossCenter
	v.tierUI = widget.NewLabel("Tier")
	v.tiers = widget.NewSegmented("1")
	v.tiers.OnChange = func(i int) gunim.Intent { return TierChosen{Tier: i + 1} }
	v.where = widget.NewLabel("")
	v.where.Color = widget.MenuHint
	v.parts = widget.NewList()
	v.parts.OnClick = func(k widget.Key) gunim.Intent { return PartClicked{Name: string(k)} }
	v.hint = widget.NewLabel("")
	v.hint.Color = widget.MenuHint
	v.follow = widget.NewButton("Follow the tier")
	v.follow.Icon, v.follow.On = icon.Layers, TierFollowed{}
	partsHead := widget.Row(v.hint, v.follow).Grow(v.hint, 1)
	partsHead.Cross = widget.CrossCenter
	list := widget.NewScroll(v.parts)
	col := widget.Column(title, v.songs, v.about, transport, v.tierUI, v.tiers, v.where, partsHead, list).Grow(list, 1)
	col.Cross = widget.CrossStretch
	v.Pad = widget.NewPad(col)
	v.Padding = widget.CardPadding
	return &keys{v}
}

// update shows s.
func (v *view) update(s Jukebox, u *gunim.UI) {
	v.songs.Selected = s.Song
	v.about.SetText(s.About)
	if s.Playing {
		v.play.Label, v.play.Icon = "Pause", icon.Pause
	} else {
		v.play.Label, v.play.Icon = "Play", icon.Play
	}
	if s.Tiers > 0 {
		if v.shown != s.Tiers {
			labels := make([]string, s.Tiers)
			for i := range labels {
				labels[i] = strconv.Itoa(i + 1)
			}
			v.tiers.Labels = labels
			v.shown = s.Tiers
		}
		v.tierUI.SetText(fmt.Sprintf("Tier: %d of %d. It changes at the next phrase; keys 1 to %d set it too.", s.Tier, s.Tiers, s.Tiers))
		if v.tiers.Selected() != s.Tier-1 {
			v.tiers.SetSelected(s.Tier-1, u)
		}
	} else {
		v.tierUI.SetText("This song has no tiers: its parts come and go by themselves.")
		v.tiers.Labels = []string{"–"}
		v.shown = 0
	}
	v.where.SetText(s.Where)
	v.follow.Disabled = !s.Triggers
	if s.Triggers {
		v.hint.SetText("Click a part to start it, or stop it with its outro, at the next phrase.")
	} else {
		v.hint.SetText("The song chooses its parts.")
	}
	widget.Sync(v.parts, u, s.Parts, func(p PartRow) widget.Key { return widget.Key(p.Name) }, newPartRow, (*partRow).set)
}

// update shows s.
func (k *keys) update(s Jukebox, u *gunim.UI) { k.view.update(s, u) }

// Handle sets the tier for the keys 1 to 9.
func (k *keys) Handle(e input.Event, u *gunim.UI) bool {
	p, ok := e.(input.KeyPress)
	if !ok || p.Mods != 0 || p.Char < '1' || p.Char > '9' {
		return false
	}
	n := int(p.Char - '0')
	if n > k.shown {
		return false
	}
	u.Send(k, TierChosen{Tier: n})
	return true
}

// partRow is a part's row: its name and tier, and what it plays.
type partRow struct {
	*widget.Card
	name, doing *widget.Label
}

func newPartRow(p PartRow) *partRow {
	r := &partRow{name: widget.NewLabel(""), doing: widget.NewLabel("")}
	gap := widget.NewSpacer()
	row := widget.Row(r.name, gap, r.doing).Grow(gap, 1)
	row.Cross = widget.CrossCenter
	r.Card = widget.NewCard(row)
	r.set(p, nil)
	return r
}

func (r *partRow) set(p PartRow, _ *gunim.UI) {
	name := p.Name
	if p.Tier > 0 {
		name = fmt.Sprintf("%s · tier %d", p.Name, p.Tier)
	}
	r.name.SetText(name)
	r.doing.SetText(p.Doing)
	r.doing.Color = widget.MenuHint
	if p.Playing {
		r.doing.Color = widget.Accent
	}
}

// player is what plays: the songs, the one chosen and its voice.
type player struct {
	mix    *audio.Mixer
	songs  []band.Song
	names  []string
	song   int
	voice  *audio.Voice
	p      band.Player
	volume float32
}

// start plays song i from its start, at the tier the last one had.
func (pl *player) start(i int) {
	tier := 0
	if t, ok := pl.p.(band.Tiered); ok {
		tier = t.Tier()
	}
	if pl.voice != nil {
		pl.voice.Stop(300 * time.Millisecond)
	}
	pl.song = i
	pl.p = pl.songs[i].Play(uint64(time.Now().UnixNano()))
	if t, ok := pl.p.(band.Tiered); ok && tier > 0 {
		t.SetTier(tier)
	}
	pl.voice = pl.mix.Play(pl.p, audio.Options{Volume: pl.volume, FadeIn: 300 * time.Millisecond})
}

// toggle starts the part named name where it rests or is leaving, and
// stops it where it plays, from the next phrase on.
func (pl *player) toggle(name string) {
	t, ok := pl.p.(band.Triggered)
	w, ok2 := pl.p.(band.Watcher)
	if !ok || !ok2 {
		return
	}
	for _, p := range w.Watch().Parts {
		if p.Name != name {
			continue
		}
		on := p.Playing && p.Piece != band.Outro
		switch p.Control {
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
		_ = t.SetPart(name, c)
	}
}

// partNames returns the names of the song's parts.
func (pl *player) partNames() []string {
	w, ok := pl.p.(band.Watcher)
	if !ok {
		return nil
	}
	var names []string
	for _, p := range w.Watch().Parts {
		names = append(names, p.Name)
	}
	return names
}

// state returns what the window shows.
func (pl *player) state() Jukebox {
	s := Jukebox{Song: pl.song, Playing: !pl.voice.Paused()}
	for _, song := range pl.songs {
		s.Songs = append(s.Songs, song.Info().Title)
	}
	i := pl.songs[pl.song].Info()
	s.About = fmt.Sprintf("By %s, %v BPM. Name: %s", i.Artist, i.BPM, pl.names[pl.song])
	if t, ok := pl.p.(band.Tiered); ok {
		s.Tiers, s.Tier = t.Tiers(), t.Tier()
	}
	_, s.Triggers = pl.p.(band.Triggered)
	if w, ok := pl.p.(band.Watcher); ok {
		st := w.Watch()
		s.Where = fmt.Sprintf("Bar %d: phrase %d, bar %d of %d", st.Bar+1, st.Bar/st.PhraseBars+1, st.Bar%st.PhraseBars+1, st.PhraseBars)
		for _, p := range st.Parts {
			doing := "resting"
			if p.Playing {
				doing = p.Piece.String()
			}
			switch p.Control {
			case band.PartOn:
				doing += " · on"
			case band.PartOff:
				doing += " · off"
			case band.PartAuto:
			}
			s.Parts = append(s.Parts, PartRow{Name: p.Name, Tier: p.Tier, Doing: doing, Playing: p.Playing})
		}
	}
	return s
}

func serve(ctx context.Context, c gunim.Client) error {
	pl := &player{mix: audio.NewMixer(), volume: 1}
	if _, err := speaker.Open(pl.mix, speaker.Options{Name: "gunim-music jukebox"}); err != nil {
		return err
	}
	for _, name := range music.Songs() {
		s, err := music.Song(name)
		if err != nil {
			log.Print(err)
			continue
		}
		pl.songs, pl.names = append(pl.songs, s), append(pl.names, name)
	}
	if len(pl.songs) == 0 {
		return errors.New("jukebox: the library holds no songs")
	}
	pl.start(0)
	if err := c.Mount(gunim.Root, "jukebox", "jukebox", pl.state()); err != nil {
		return err
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch v := ev.Intent.(type) {
			case SongChosen:
				if v.Song != pl.song && v.Song < len(pl.songs) {
					pl.start(v.Song)
				}
			case PlayToggled:
				if pl.voice.Paused() {
					pl.voice.Resume()
				} else {
					pl.voice.Pause()
				}
			case Restarted:
				pl.start(pl.song)
			case TierChosen:
				if t, ok := pl.p.(band.Tiered); ok {
					t.SetTier(v.Tier)
				}
			case PartClicked:
				pl.toggle(v.Name)
			case TierFollowed:
				if t, ok := pl.p.(band.Triggered); ok {
					for _, name := range pl.partNames() {
						_ = t.SetPart(name, band.PartAuto)
					}
				}
			case VolumeSet:
				pl.volume = v.Volume
				pl.voice.SetVolume(v.Volume, anim.Spring{Response: 0.2, Damping: 1})
			}
		}
		_ = c.Update("jukebox", pl.state())
	}
}
