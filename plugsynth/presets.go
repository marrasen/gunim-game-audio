package plugsynth

import (
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
)

// The synth offers its sounds to the host's browser as presets.
var _ plug.PresetLister = (*plugin)(nil)

// Presets implements [plug.PresetLister]: every sound, in order.
func (p *plugin) Presets() []plug.Preset {
	out := make([]plug.Preset, 0, len(all()))
	for _, s := range all() {
		out = append(out, asPreset(s))
	}
	return out
}

// PresetOf implements [plug.PresetLister]: the sound the state names,
// by its bank, group and name, even where its patch was edited since;
// the default sound for an empty one, as SetState plays.
func (p *plugin) PresetOf(state []byte) (plug.Preset, bool) {
	st := defaultState()
	if len(state) > 0 {
		var err error
		if st, err = DecodeState(state); err != nil {
			return plug.Preset{}, false
		}
	}
	s, ok := Find(st.Bank, st.Group, st.Name)
	if !ok {
		return plug.Preset{}, false
	}
	return asPreset(s), true
}

// asPreset is s as a preset.
func asPreset(s Sound) plug.Preset {
	return plug.Preset{
		Bank: s.Bank, Group: s.Group, Name: s.Name, About: s.About,
		Pitch: previewPitch(s),
		Drums: s.patch.Kind == "drums",
		State: func() ([]byte, error) { return s.State(), nil },
	}
}

// previewPitch is the note a preview of s plays: a kick of a drum kit,
// a low A of a bass, and middle C of the rest.
func previewPitch(s Sound) float64 {
	switch {
	case s.patch.Kind == "drums":
		return 36
	case strings.Contains(strings.ToLower(s.Group), "bass"):
		return 45
	}
	return 60
}

// skip plays the sound by from the one playing, in its group, round
// from the last to the first, as one edit for the host, and plays a
// note of it. A sound of its own, in no group, skips to nothing.
func (v *panel) skip(by int, u *gunim.UI) {
	cur := v.p.sound()
	var list []Sound
	at := -1
	for _, s := range all() {
		if s.Bank != cur.Bank || s.Group != cur.Group {
			continue
		}
		if s.Name == cur.Name {
			at = len(list)
		}
		list = append(list, s)
	}
	if len(list) == 0 {
		return
	}
	s := list[((at+by)%len(list)+len(list))%len(list)]
	v.h.BeginEdit("Pick "+s.Name, u)
	v.p.pick(s)
	v.h.Changed(u)
	v.h.EndEdit(u)
	v.FollowState(u)
	v.audition(s, u)
}

// audition plays a short note of s, where the host lets it.
func (v *panel) audition(s Sound, u *gunim.UI) {
	if a, ok := v.h.(plug.Auditioner); ok {
		a.Audition(previewPitch(s), 0.8, 700*time.Millisecond, u)
	}
}
