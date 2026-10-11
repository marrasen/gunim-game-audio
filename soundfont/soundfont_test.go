package soundfont

import (
	"bytes"
	"testing"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
)

func TestABankReadsItsPresetsAndRegions(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Test Bank" || len(f.Presets) != 1 || f.Presets[0].Name != "Square" {
		t.Fatalf("read %q, presets %+v", f.Name, f.Presets)
	}
	rs := f.Regions(0, 0, 72, 100)
	if len(rs) != 1 {
		t.Fatalf("key 72 plays %d regions, want 1", len(rs))
	}
	r := rs[0]
	if r.Root != 60 || r.Loop != 1 || r.LoopStart != 0 || r.LoopEnd != 100 || r.Pan != 1 {
		t.Errorf("the region: root %d, loop %d from %d to %d, pan %v", r.Root, r.Loop, r.LoopStart, r.LoopEnd, r.Pan)
	}
	if rel := r.Vol.Release; rel < 0.09 || rel > 0.11 {
		t.Errorf("the release is %v s, want about 0.1", rel)
	}
	if n := len(r.Sample.Frames()); n != 100 || r.Sample.Frames()[0] != 12000 {
		t.Errorf("the sample has %d frames", n)
	}
	// A bank it lacks falls back to bank 0; a drum kit it lacks, to none.
	if f.Preset(8, 0) == nil || f.Preset(128, 0) != nil {
		t.Error("the fallbacks are wrong")
	}
}

func TestNotABankIsRefused(t *testing.T) {
	if _, err := Read(bytes.NewReader([]byte("RIFF\x04\x00\x00\x00WAVE"))); err == nil {
		t.Error("a WAV file read as a bank")
	}
}
