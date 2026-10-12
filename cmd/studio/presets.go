package main

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/widget"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

// fromSongs is the library's last category: every patch of the songs
// made in code, under the song's title.
const fromSongs = "From the songs"

// preset is a patch to try: the library's, or a song's.
type preset struct {
	row   PresetRow
	patch *synth.Patch
}

// loadPresets lists the library's presets, and then each song's patches.
func loadPresets(songs []*synth.Song) []preset {
	var out []preset
	lib, err := music.Presets()
	if err == nil {
		for _, p := range lib {
			out = append(out, preset{row: PresetRow{ID: p.Category + "/" + p.Name, Name: p.Name, About: p.About,
				Path: []string{p.Category}, Drums: p.Patch.Kind == "drums"}, patch: p.Patch})
		}
	}
	for _, s := range songs {
		for _, name := range sortedNames(s.Patches) {
			p := s.Patches[name]
			out = append(out, preset{row: PresetRow{ID: fromSongs + "/" + s.Title + "/" + name, Name: name,
				About: "The " + name + " of " + s.Title + ".", Path: []string{fromSongs, s.Title}, Drums: p.Kind == "drums"},
				patch: clonePatch(p)})
		}
	}
	return out
}

func clonePatch(p *synth.Patch) *synth.Patch {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	c := new(synth.Patch)
	if err := json.Unmarshal(b, c); err != nil {
		panic(err)
	}
	return c
}

// presetIndex returns the index of the preset id, or -1.
func (s *studio) presetIndex(id string) int {
	return slices.IndexFunc(s.presets, func(p preset) bool { return p.row.ID == id })
}

// tryPreset puts preset id in place of the patch name, as loud as the
// patch was, keeping the patch as it was to revert to.
func (s *studio) tryPreset(name, id string) {
	i := s.presetIndex(id)
	old, ok := s.song.Patches[name]
	if i < 0 || !ok {
		return
	}
	pr := s.presets[i]
	if (old.Kind == "drums") != pr.row.Drums {
		s.status = "A kit takes a kit's place, and a synth a synth's"
		return
	}
	orig, trying := s.backup[name]
	if !trying {
		orig = clonePatch(old)
	}
	p := clonePatch(pr.patch)
	// As loud as the patch it replaces was, so the mix keeps its balance.
	pitch := s.pitchOf(name)
	if want, got := levelOf(orig, pitch), levelOf(p, pitch); want > 0 && got > 0 {
		gain := p.Gain
		if gain == 0 {
			gain = 1
		}
		p.Gain = math.Round(min(max(gain*want/got, 0.05), 4)*1000) / 1000
	}
	if err := s.edit(func(song *synth.Song) { song.Patches[name] = p }); err != nil {
		s.status = plain(err)
		return
	}
	s.backup[name] = orig
	s.trying[name] = id
	s.status = "Trying " + pr.row.Name + " as " + name + ": Keep it, or Revert to the patch as it was"
	s.gen++
}

// stepPreset tries the preset by places after the one tried on name, in
// its category, round from the end to the start; or, where by is 0, one
// of them at random. Where none is tried, it starts the category of
// name's kind.
func (s *studio) stepPreset(name string, by int) {
	old, ok := s.song.Patches[name]
	if !ok {
		return
	}
	drums := old.Kind == "drums"
	cur := s.presetIndex(s.trying[name])
	var path []string
	if cur >= 0 {
		path = s.presets[cur].row.Path
	}
	var group []int
	for i, p := range s.presets {
		if p.row.Drums != drums {
			continue
		}
		if path == nil {
			path = p.row.Path
		}
		if slices.Equal(p.row.Path, path) {
			group = append(group, i)
		}
	}
	if len(group) == 0 {
		return
	}
	at := slices.Index(group, cur)
	switch {
	case by == 0:
		next := at
		for len(group) > 1 && next == at {
			next = rand.IntN(len(group))
		}
		at = next
	case at < 0:
		at = 0
		if by < 0 {
			at = len(group) - 1
		}
	default:
		at = ((at+by)%len(group) + len(group)) % len(group)
	}
	s.tryPreset(name, s.presets[group[at]].row.ID)
}

// keepPreset makes the preset tried on name the patch, to revert no more.
func (s *studio) keepPreset(name string) {
	if id, ok := s.trying[name]; ok {
		s.status = "Kept " + s.presets[max(s.presetIndex(id), 0)].row.Name + " as " + name
	}
	delete(s.backup, name)
	delete(s.trying, name)
}

// revertPreset puts back the patch name as it was before presets were
// tried on it.
func (s *studio) revertPreset(name string) {
	orig, ok := s.backup[name]
	if !ok {
		return
	}
	if err := s.edit(func(song *synth.Song) { song.Patches[name] = clonePatch(orig) }); err != nil {
		s.status = plain(err)
		return
	}
	delete(s.backup, name)
	delete(s.trying, name)
	s.status = "Reverted " + name
	s.gen++
}

// pitchOf returns a note the patch name plays about, to hear it at: the
// C of the octave of the first track that plays it.
func (s *studio) pitchOf(name string) int {
	for _, t := range s.song.Tracks {
		if t.Patch == name {
			oct := t.Octave
			if oct == 0 {
				oct = 4
			}
			return 12 * (oct + 1)
		}
	}
	return 60
}

// levelOf returns how loud p sounds, at pitch, or a kit's kick, snare
// and hat together.
func levelOf(p *synth.Patch, pitch int) float64 {
	var x []float32
	if p.Kind == "drums" {
		for _, d := range []string{"bd", "sn", "hh"} {
			h, err := synth.PreviewDrum(p, d, 0.4)
			if err == nil {
				x = append(x, h...)
			}
		}
	} else {
		var err error
		if x, err = synth.PreviewPatch(p, pitch, 0.4, 0.6); err != nil {
			return 0
		}
	}
	var sum float64
	for _, v := range x {
		sum += float64(v * v)
	}
	return math.Sqrt(sum / float64(max(len(x), 1)))
}

// libraryBar steps through the patch library, which its tree, docked
// beside the editor, opens: a step back and on through the category of
// the preset tried, a pick at random, Keep and Revert for the preset
// tried, and what it sounds like.
type libraryBar struct {
	*widget.Flex
	prev, next   *widget.IconButton
	dice         *widget.IconButton
	keep, revert *widget.Button
	about        *widget.Label
}

func newLibraryBar() *libraryBar {
	lb := &libraryBar{}
	lb.prev = widget.NewIconButton(icon.ChevronLeft, "Try the preset before this one")
	lb.next = widget.NewIconButton(icon.ChevronRight, "Try the next preset")
	lb.dice = widget.NewIconButton(icon.Dices, "Try a preset of this category at random")
	lb.keep = widget.NewButton("Keep")
	lb.keep.Icon, lb.keep.Tooltip = icon.Check, "Keep the preset tried as the patch"
	lb.revert = widget.NewButton("Revert")
	lb.revert.Icon, lb.revert.Tooltip = icon.Undo2, "Put back the patch as it was before you tried presets"
	lb.about = small("")
	lb.about.MaxLines = 2
	lb.Flex = widget.Row(small("LIBRARY"), lb.prev, lb.next, lb.dice, lb.keep, lb.revert, lb.about).Grow(lb.about, 1)
	lb.Flex.Cross = widget.CrossCenter
	return lb
}

// update shows the library for the patch name, from st.
func (lb *libraryBar) update(st Studio, name string) {
	id, trying := st.Trying[name]
	lb.about.Text = "Pick a sound in the library to try in place of " + name + ", as loud as it is; Revert puts it back."
	if trying {
		for _, p := range st.Presets {
			if p.ID == id {
				lb.about.Text = p.About
			}
		}
	}
	lb.prev.OnClick = widget.Sends(PresetStep{Patch: name, By: -1})
	lb.next.OnClick = widget.Sends(PresetStep{Patch: name, By: 1})
	lb.dice.OnClick = widget.Sends(PresetStep{Patch: name})
	lb.keep.OnClick = widget.Sends(PresetKeep{Patch: name})
	lb.revert.OnClick = widget.Sends(PresetRevert{Patch: name})
	lb.keep.Disabled, lb.revert.Disabled = !trying, !trying
}
