package plugsynth

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

func TestThePresetsAreTheSounds(t *testing.T) {
	p, _ := newSynth(t)
	ps, ss := p.Presets(), Sounds()
	if len(ps) != len(ss) {
		t.Fatalf("the synth lists %d presets for %d sounds", len(ps), len(ss))
	}
	for i, s := range ss {
		if !ps[i].Same(plug.Preset{Bank: s.Bank, Group: s.Group, Name: s.Name}) || ps[i].About != s.About {
			t.Fatalf("preset %d is %s/%s/%s, not %s/%s/%s", i, ps[i].Bank, ps[i].Group, ps[i].Name, s.Bank, s.Group, s.Name)
		}
	}
	// A preset's state plays it.
	for _, c := range []struct {
		name  string
		pitch float64
		drums bool
	}{{"Juno pad", 60, false}, {"Moog bass", 45, false}, {"TR-808 kit", 36, true}} {
		s, _ := Named(c.name)
		pr, ok := p.PresetOf(s.State())
		if !ok || !pr.Same(plug.Preset{Bank: s.Bank, Group: s.Group, Name: s.Name}) {
			t.Fatalf("the state of %s is preset %s, %v", c.name, pr.Name, ok)
		}
		if pr.Pitch != c.pitch || pr.Drums != c.drums {
			t.Errorf("%s previews at %g, drums %v", c.name, pr.Pitch, pr.Drums)
		}
		b, err := pr.State()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.SetState(b); err != nil || p.sound().Name != c.name {
			t.Errorf("preset %s's state plays %s: %v", c.name, p.sound().Name, err)
		}
	}
}

func TestPresetOfNamesTheSoundByItsName(t *testing.T) {
	p, _ := newSynth(t)
	if pr, ok := p.PresetOf(nil); !ok || pr.Name != DefaultSound {
		t.Errorf("an empty state is preset %q, %v, not %s", pr.Name, ok, DefaultSound)
	}
	// A patch edited since keeps its sound's name.
	s, _ := Named("Moog bass")
	pt := s.Patch()
	pt.Amp.Release *= 2
	edited := State{Bank: s.Bank, Group: s.Group, Name: s.Name, Patch: pt}.Encode()
	if pr, ok := p.PresetOf(edited); !ok || pr.Name != "Moog bass" {
		t.Errorf("the Moog bass, edited, is preset %q, %v", pr.Name, ok)
	}
	// A sound of its own, and a state that does not read, are none.
	own := State{Name: "Everything", Patch: s.Patch()}.Encode()
	for _, b := range [][]byte{own, []byte("{"), []byte(`{"Name":"Juno pad"}`)} {
		if pr, ok := p.PresetOf(b); ok {
			t.Errorf("the state %.40q is preset %s", b, pr.Name)
		}
	}
}
