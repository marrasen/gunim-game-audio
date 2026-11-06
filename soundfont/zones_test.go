package soundfont

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/marrasen/gunim-game-audio/internal/sftest"
)

func TestAPresetListsItsZones(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	zs := slices.Collect(f.Zones(f.Preset(0, 0)))
	if len(zs) != 1 {
		t.Fatalf("%d zones", len(zs))
	}
	z := zs[0]
	if z.KeyLo != 0 || z.KeyHi != 127 || z.VelLo != 0 || z.VelHi != 127 || z.Root != 60 || z.Loop != 1 {
		t.Errorf("the zone is %+v", z)
	}
	if rs := f.Regions(0, 0, z.Root, 127); z.Sample != rs[0].Sample || z.Gain != rs[0].Gain {
		t.Errorf("the zone plays %+v, the note %+v", z.Region, rs[0])
	}
	if z.HalfGain >= z.Gain {
		t.Errorf("a soft note plays at %v, a hard one at %v", z.HalfGain, z.Gain)
	}
	if zs := slices.Collect(f.Zones(f.Preset(128, 0))); len(zs) == 0 {
		t.Errorf("the kit has no zones")
	}
}

func TestZonesStopsWhenAsked(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for range f.Zones(f.Preset(0, 0)) {
		n++
		break
	}
	if n != 1 {
		t.Errorf("%d zones", n)
	}
}

func TestDecodeReadsASampleAndKeepsNothing(t *testing.T) {
	f, err := Read(bytes.NewReader(sftest.Build()))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Regions(0, 0, 60, 100)[0].Sample
	got, err := s.Decode(1000)
	if err != nil || len(got) != 100 || got[0] != 12000 {
		t.Fatalf("read %d frames: %v", len(got), err)
	}
	if s.done.Load() {
		t.Error("the frames were kept")
	}
	if _, err := s.Decode(99); !errors.Is(err, ErrTooLong) {
		t.Errorf("past the limit: %v", err)
	}
	s.End = 1 << 30
	if _, err := s.Decode(1 << 31); err == nil {
		t.Error("a sample past the bank read")
	}
}

func TestAClosedBankLetsItsFileGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sf2")
	if err := os.WriteFile(path, sftest.Build(), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Error(err)
	}
}

func TestADamagedFileIsLetGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.sf2")
	if err := os.WriteFile(path, []byte("not a bank at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("a bad file opened")
	}
	if err := os.Remove(path); err != nil {
		t.Error(err)
	}
}
