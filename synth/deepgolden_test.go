package synth

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"testing"
)

// drumGolden is where the hashes of every drum's sound are kept, as the
// drums sounded before the deep settings: a drum that leaves them must
// sound bit for bit the same. Write it afresh with
// DRUM_GOLDEN_WRITE=1 go test ./synth -run TestEveryDrumSoundsAsItDidBeforeTheDeepSettings,
// but only from code that sounds as it should.
const drumGolden = "testdata/drum_golden.json"

// goldenHits are the drums the golden hashes cover: every drum the
// default kit names, and every drum of General MIDI's standard,
// electronic and TR-808 kits, each plain and with Tune, Decay and Tone
// moved, at two velocities.
func goldenHits(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	add := func(kit string, p *Patch) {
		c, err := p.compile(kit)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(c.kit))
		for n := range c.kit {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			for vi, vel := range []float32{1, 0.55} {
				for ki, mv := range []func(*Drum){nil, func(d *Drum) { d.Tune += 2; d.Decay = 1.5; d.Tone = 0.3 }} {
					d := c.kit[n]
					if mv != nil {
						mv(&d.Drum)
					}
					hs := newHits(1)
					hs.play(d, vel, 0, 40, rate/2, 0)
					l, r := make([]float32, rate), make([]float32, rate)
					hs.render(l, r, true)
					h := fnv.New64a()
					for i := range l {
						var b [8]byte
						lb, rb := math.Float32bits(l[i]), math.Float32bits(r[i])
						for j := range 4 {
							b[j], b[4+j] = byte(lb>>(8*j)), byte(rb>>(8*j))
						}
						h.Write(b[:])
					}
					out[fmt.Sprintf("%s/%s/v%d/m%d", kit, n, vi, ki)] = strconv.FormatUint(h.Sum64(), 16)
				}
			}
		}
	}
	add("kit", &Patch{Kind: "drums"})
	for _, k := range []int{0, 24, 25} {
		add("gm"+strconv.Itoa(k), GMKit(k))
	}
	return out
}

func TestEveryDrumSoundsAsItDidBeforeTheDeepSettings(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the golden hashes are amd64's; other machines may fuse multiplies and adds")
	}
	got := goldenHits(t)
	if os.Getenv("DRUM_GOLDEN_WRITE") == "1" {
		b, _ := json.MarshalIndent(got, "", "\t")
		if err := os.WriteFile(drumGolden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(drumGolden)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Errorf("%d drum sounds, %d golden", len(got), len(want))
	}
	bad := 0
	for k, w := range want {
		if got[k] != w {
			bad++
			if bad <= 20 {
				t.Errorf("%s sounds different from before", k)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d drum sounds differ", bad, len(want))
	}
}
