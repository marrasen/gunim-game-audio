package soundfont

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
)

func TestABankReadsItsPresetsAndRegions(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Test Bank" || len(f.Presets) != 2 || f.Presets[0].Name != "Square" || f.Presets[1].Name != "Kit" {
		t.Fatalf("read %q, presets %+v", f.Name, f.Presets)
	}
	if n, m := len(f.Regions(128, 0, 38, 100)), len(f.Regions(128, 0, 49, 100)); n != 1 || m != 0 {
		t.Fatalf("the kit plays %d regions on key 38 and %d on 49, want 1 and 0", n, m)
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
	// A bank it lacks falls back to bank 0; a drum kit it lacks, to kit 0.
	if f.Preset(8, 0) == nil || f.Preset(128, 25) != f.Preset(128, 0) {
		t.Error("the fallbacks are wrong")
	}
}

func TestNotABankIsRefused(t *testing.T) {
	if _, err := Read(bytes.NewReader([]byte("RIFF\x04\x00\x00\x00WAVE"))); err == nil {
		t.Error("a WAV file read as a bank")
	}
}

func TestABankOpenedFromItsFilePlaysAsOneRead(t *testing.T) {
	b := sftest.Build()
	path := filepath.Join(t.TempDir(), "test.sf2")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	got, want := f.Regions(0, 0, 72, 100)[0].Sample.Frames(), r.Regions(0, 0, 72, 100)[0].Sample.Frames()
	if len(got) != 100 || !slices.Equal(got, want) {
		t.Errorf("the file's sample is %v, want %v", got, want)
	}
	// The file may go once the bank is open: its samples stay.
	if err := os.Remove(path); err == nil {
		if n := len(f.Regions(128, 0, 38, 100)[0].Sample.Frames()); n == 0 {
			t.Error("the kit's sample went with the file")
		}
	}
}
