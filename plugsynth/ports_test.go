package plugsynth

import (
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
)

func TestADrumKitHasAPortForEachPartAndOtherSoundsNone(t *testing.T) {
	kits, others := 0, 0
	for _, s := range Sounds() {
		p := Descriptor.PortsIn(s.State())
		drums := s.Patch().Kind == "drums"
		for i := range synth.DrumParts {
			if p.HasAuxOut(i) != drums {
				t.Errorf("%s has its %s output %v, want %v", s.Name, synth.DrumPartNames[i], p.HasAuxOut(i), drums)
			}
		}
		if drums {
			kits++
		} else {
			others++
		}
	}
	if kits == 0 || others == 0 {
		t.Fatalf("the sounds hold %d drum kits and %d other sounds, want both", kits, others)
	}
	for name, state := range map[string][]byte{"no state": nil, "an unreadable state": []byte("{")} {
		if p := Descriptor.PortsIn(state); p.HasAuxOut(KickOut) {
			t.Errorf("%s has a kick output, want the default sound's outputs: none", name)
		}
	}
}
