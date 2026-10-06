package calls

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
)

func library(t testing.TB) *Library {
	t.Helper()
	lib, err := LoadDir("../voices")
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestEveryCallMeetsTheBriefInEveryTake(t *testing.T) {
	lib := library(t)
	// The game's ten, as its brief names them; the library may hold
	// more, as Whizpah, who must meet the brief too.
	for _, id := range []string{"groda", "uggla", "enhorningskatt", "kpop-tjej", "kpop-kille",
		"robo-ninja", "trollkarlen", "drakungen", "fotbollsstjarnan", "raven"} {
		if lib.Companion(id) == nil {
			t.Errorf("the library holds no %s", id)
		}
	}
	// The game's bosses, a voice for its early, middle and late tables.
	for _, id := range []string{"boss-liten", "boss-mellan", "boss-stor"} {
		if c := lib.Companion(id); c == nil || c.Role != Boss {
			t.Errorf("the library holds no boss %s", id)
		}
	}
	for _, c := range lib.Companions {
		t.Run(c.ID, func(t *testing.T) {
			t.Parallel()
			for _, kind := range c.Kinds() {
				if _, ok := c.Calls[kind]; !ok {
					t.Errorf("%s makes no %s", c.ID, kind)
					continue
				}
				for seed := range uint64(20) {
					tk, err := lib.Take(c.ID, kind, seed)
					if err != nil {
						t.Fatal(err)
					}
					if p := tk.Stats.Problems(kind, lib.Master); len(p) > 0 {
						t.Errorf("%s's %s, take %d: %v", c.ID, kind, seed, p)
					}
					if v := tk.Samples[0]; v != 0 {
						t.Errorf("%s's %s, take %d, starts at %v, not from silence", c.ID, kind, seed, v)
					}
				}
			}
		})
	}
}

func TestATakeIsTheSameFromItsSeedAndAnotherSeedStrays(t *testing.T) {
	lib := library(t)
	a, _ := lib.Take("uggla", Hello, 7)
	b, _ := lib.Take("uggla", Hello, 7)
	c, _ := lib.Take("uggla", Hello, 8)
	if !slices.Equal(a.Samples, b.Samples) {
		t.Error("take 7 differs from itself")
	}
	if slices.Equal(a.Samples, c.Samples) {
		t.Error("takes 7 and 8 are the same")
	}
	// A call that does not vary makes each take as set.
	call := *lib.Companion("uggla").Calls[Hello]
	call.Vary = 0
	set, other := lib.Make(&call, 0), lib.Make(&call, 9)
	if set.Stats.Length != other.Stats.Length {
		t.Errorf("a call of Vary 0 lasts %v as set but %v in take 9", set.Stats.Length, other.Stats.Length)
	}
}

func TestALibrarySavedReadsBackTheSame(t *testing.T) {
	lib := library(t)
	dir := t.TempDir()
	for _, c := range lib.Companions {
		if err := lib.Save(dir, c); err != nil {
			t.Fatal(err)
		}
	}
	again, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lib.Companions {
		for kind := range c.Calls {
			a, _ := lib.Take(c.ID, kind, 3)
			b, _ := again.Take(c.ID, kind, 3)
			if !slices.Equal(a.Samples, b.Samples) {
				t.Errorf("%s's %s sounds different once saved", c.ID, kind)
			}
		}
	}
	// Each parameter is written out, so the file shows them all.
	b, _ := os.ReadFile(filepath.Join(dir, "uggla.json"))
	if !bytes.Contains(b, []byte(`"vibrato"`)) {
		t.Error("uggla.json leaves its defaults out")
	}
}

func TestAPlayerPlaysANewTakeEachTime(t *testing.T) {
	lib := library(t)
	mix := audio.NewMixer()
	p := NewPlayer(mix, lib)
	p.Warm("groda")
	var lens []time.Duration
	for range 4 {
		v, err := p.Play("groda", Cheer, audio.Options{})
		if err != nil {
			t.Fatal(err)
		}
		lens = append(lens, v.Len())
	}
	if mix.Playing() != 4 {
		t.Errorf("%d calls play, not 4", mix.Playing())
	}
	slices.Sort(lens)
	if lens[0] == lens[3] {
		t.Errorf("four takes all last %v", lens[0])
	}
	if _, err := p.Play("nobody", Hello, audio.Options{}); err == nil {
		t.Error("a companion the library does not hold played")
	}
}

func TestAWAVFileIsMonoAt48kHzIn24Bits(t *testing.T) {
	var b bytes.Buffer
	if err := WriteWAV(&b, []float32{0, 0.5, -1}); err != nil {
		t.Fatal(err)
	}
	h := b.Bytes()
	if string(h[0:4]) != "RIFF" || binary.LittleEndian.Uint16(h[22:]) != 1 ||
		binary.LittleEndian.Uint32(h[24:]) != 48000 || binary.LittleEndian.Uint16(h[34:]) != 24 || len(h) != 44+9 {
		t.Fatalf("the header is %x", h[:44])
	}
	clip, err := audio.Load(bytes.NewReader(h))
	if err != nil {
		t.Fatal(err)
	}
	if s := clip.Samples(); len(s) != 6 || s[2] < 0.49 || s[2] > 0.51 || s[3] != s[2] {
		t.Errorf("gunim reads the file back as %v", s)
	}
}

func BenchmarkACheer(b *testing.B) {
	lib := library(b)
	for i := range b.N {
		_, _ = lib.Take("groda", Cheer, uint64(i))
	}
}
func TestTheTruePeakIsAsGunimsMeterFindsIt(t *testing.T) {
	lib := library(t)
	for _, seed := range []uint64{0, 1, 2} {
		for _, id := range []string{"groda", "uggla"} {
			for _, k := range Kinds {
				tk, _ := lib.Take(id, k, seed)
				var m audio.TruePeakMeter
				st := make([]float32, 2*len(tk.Samples))
				for i, v := range tk.Samples {
					st[2*i], st[2*i+1] = v, v
				}
				m.Write(st)
				theirs, ours := toDB(m.Peak()), truePeak(tk.Samples)
				if math.Abs(theirs-ours) > 0.05 {
					t.Errorf("%s %s: gunim %.3f, ours %.3f", id, k, theirs, ours)
				}
			}
		}
	}
}

func TestABossMakesItsOwnCallsAndACompanionItsOwn(t *testing.T) {
	boss, comp := &Companion{Role: Boss}, &Companion{}
	if !slices.Equal(boss.Kinds(), BossKinds) || !slices.Equal(comp.Kinds(), Kinds) {
		t.Fatalf("a boss makes %v and a companion %v", boss.Kinds(), comp.Kinds())
	}
	for _, kind := range BossKinds {
		if lo, hi := Lengths(kind); lo <= 0 || hi <= lo {
			t.Errorf("a boss's %s may last %v–%v s", kind, lo, hi)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "library.json"), []byte(`{"Order":["x"]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "x.json"), []byte(`{"Name":"X","Calls":{"roar":{"Layers":[]}}}`), 0o644)
	if _, err := LoadDir(dir); err == nil {
		t.Error("a companion roaring was read without complaint")
	}
	os.WriteFile(filepath.Join(dir, "x.json"), []byte(`{"Name":"X","Role":"boss","Calls":{"roar":{"Layers":[]}}}`), 0o644)
	if _, err := LoadDir(dir); err != nil {
		t.Errorf("a boss roaring was refused: %v", err)
	}
}
