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
// electronic and TR-808 kits. Each drum's settings are hashed, and that
// plays nothing. The first drum of each type plays, once plain at full
// velocity and once softer with its Tune, Decay and Tone moved, so every
// type's sound is heard.
func goldenHits(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	typed := map[int]bool{}
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
			d := c.kit[n]
			b, err := json.Marshal(d.Drum)
			if err != nil {
				t.Fatal(err)
			}
			h := fnv.New64a()
			_, _ = fmt.Fprintf(h, "%s %d %d %v %d %d %v", b, d.mute, d.kind, d.gain, d.key, d.prog, d.fine)
			out[fmt.Sprintf("set/%s/%s", kit, n)] = strconv.FormatUint(h.Sum64(), 16)
			if typed[d.kind] {
				continue
			}
			typed[d.kind] = true
			for _, vk := range [][2]int{{0, 0}, {1, 1}} {
				d := c.kit[n]
				if vk[1] == 1 {
					d.Tune += 2
					d.Decay = 1.5
					d.Tone = 0.3
				}
				hs := newHits(1)
				hs.play(d, []float32{1, 0.55}[vk[0]], 0, 40, rate/2, 0)
				l, r := make([]float32, rate), make([]float32, rate)
				hs.render(l, r, true)
				out[fmt.Sprintf("%s/%s/v%d/m%d", kit, n, vk[0], vk[1])] = hashLR(l, r)
			}
		}
	}
	add("kit", &Patch{Kind: "drums"})
	// A modelled drum struck again as it rings, and open hats choked by
	// closed ones, a hit taken from the pool for each.
	c, err := (&Patch{Kind: "drums"}).compile("kit")
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range [][]string{{"mbd", "mbd"}, {"mmt", "mmt", "mmt"}, {"msn", "msn"}, {"oh", "hh", "oh"},
		{"moh", "mhh"}, {"tr8oh", "tr8hh"}, {"tr9oh", "tr9hh", "tr9oh"}, {"bd", "sn", "bd", "tr8bd", "sdssn"}} {
		hs := newHits(2)
		l, r := make([]float32, rate), make([]float32, rate)
		step := rate / 10
		for i, n := range seq {
			hs.play(c.kit[n], 0.6+0.2*float32(i%3), 0, 40, rate/2, 0)
			hs.render(l[i*step:(i+1)*step], r[i*step:(i+1)*step], true)
		}
		hs.render(l[len(seq)*step:], r[len(seq)*step:], true)
		out["seq/"+fmt.Sprint(seq)] = hashLR(l, r)
	}
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
		t.Errorf("%d drum hashes, %d golden", len(got), len(want))
	}
	bad := 0
	for k, g := range got {
		if w, ok := want[k]; !ok {
			t.Errorf("%s has no golden sound", k)
		} else if g != w {
			bad++
			if bad <= 20 {
				t.Errorf("%s differs from before", k)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d drum hashes differ", bad, len(got))
	}
}

// hashLR hashes a stereo sound, bit for bit.
func hashLR(l, r []float32) string {
	h := fnv.New64a()
	for i := range l {
		var b [8]byte
		lb, rb := math.Float32bits(l[i]), math.Float32bits(r[i])
		for j := range 4 {
			b[j], b[4+j] = byte(lb>>(8*j)), byte(rb>>(8*j))
		}
		h.Write(b[:])
	}
	return strconv.FormatUint(h.Sum64(), 16)
}
