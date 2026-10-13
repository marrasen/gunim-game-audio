package plugfx

import (
	"math"
	"testing"

	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"

	"github.com/marrasen/gunim-game-audio/synth"
)

func TestProcessAllocatesNothing(t *testing.T) {
	cases := []struct {
		name string
		d    *plug.Descriptor
		fn   string
		// between runs between blocks, off the audio thread.
		between func(p plug.Plugin, i int)
	}{
		{"reverb", &ReverbDescriptor, "plugfx.(*reverbProc).Process", nil},
		{"delay", &DelayDescriptor, "plugfx.(*delayProc).Process", nil},
		{"colour", &ColourDescriptor, "plugfx.(*colourProc).Process", func(p plug.Plugin, i int) {
			// A new chain every few blocks, so blocks cross over.
			if i%3 == 0 {
				units := []Unit{{Kind: Distortion, Slot: 1}, {Kind: Phaser, Slot: 0}, {Kind: Chorus, Slot: 3}}
				_ = p.SetState(ColourState{Units: units[:1+i/3%3]}.Encode())
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.d.New(nil)
			pr := p.Processor()
			pr.Prepare(setup)
			in := noise(setup.MaxBlock, setup.MaxBlock)
			out := [][]float32{make([]float32, setup.MaxBlock), make([]float32, setup.MaxBlock)}
			// Every parameter changes in each block, and the tempo moves.
			var changes []plug.Change
			for _, prm := range c.d.Params {
				changes = append(changes, plug.Change{Frame: 0, ID: prm.ID, Value: prm.Default})
			}
			b := &plug.Block{Frames: setup.MaxBlock, In: in, Out: out}
			const blocks = 12
			for i := range blocks {
				if c.between != nil {
					c.between(p, i)
				}
				n := plugtest.AllocsUnder(t, c.fn, func() {
					for k := range changes {
						prm := c.d.Params[k]
						changes[k].Frame = k * setup.MaxBlock / len(changes)
						changes[k].Value = prm.Min + (prm.Max-prm.Min)*float64((i+k)%5)/4
					}
					b.Changes = changes
					b.Position.Tempo = 90 + float64(i%3)*20
					pr.Process(b)
					_ = pr.Tail()
				})
				if n > 0 {
					t.Fatalf("block %d: Process allocated %d times", i, n)
				}
			}
		})
	}
}

func TestTheReverbsTailCoversItsDecay(t *testing.T) {
	for _, decay := range []float64{0.3, 2.2, 8} {
		p := ReverbDescriptor.New(nil)
		pr := p.Processor()
		changes := []plug.Change{{ID: DecayID, Value: decay}, {ID: PreDelayID, Value: 50}}
		r := rig{pr: pr, d: &ReverbDescriptor, s: setup}
		// The tail as it stands once the changes are taken.
		r.run(t, impulse(setup.MaxBlock, 0), changes...)
		tail := pr.Tail()
		if want := int((decay + 0.05) * synth.Rate); tail < want {
			t.Errorf("decay %g s: the tail is %d frames, shorter than the decay's %d", decay, tail, want)
		}
		frames := tail + synth.Rate/2
		out := r.run(t, impulse(frames, 0), changes...)
		loud := peak(out[0], out[1])
		after := peak(out[0][tail:], out[1][tail:])
		if db := 20 * math.Log10(float64(after/loud)); db > -60 {
			t.Errorf("decay %g s: %.1f dB under its peak still sound after the tail of %d frames", decay, -db, tail)
		}
		// And the reverb sounds on through most of it.
		mid := tail / 3
		if peak(out[0][mid:mid+4800]) == 0 {
			t.Errorf("decay %g s: silence a third into the tail", decay)
		}
	}
}

func TestTheDelaysTailCoversItsEchoes(t *testing.T) {
	for _, c := range []struct {
		feedback float64
		pingPong float64
	}{{0, 1}, {35, 1}, {80, 0}, {80, 1}} {
		p := DelayDescriptor.New(nil)
		pr := p.Processor()
		changes := []plug.Change{{ID: FeedbackID, Value: c.feedback}, {ID: PingPongID, Value: c.pingPong},
			{ID: SyncID, Value: 0}, {ID: TimeID, Value: 50}, {ID: HighCutID, Value: 20000}, {ID: LowCutID, Value: 20}}
		r := rig{pr: pr, d: &DelayDescriptor, s: setup}
		r.run(t, impulse(setup.MaxBlock, 0), changes...)
		tail := pr.Tail()
		out := r.run(t, impulse(tail+synth.Rate, 0), changes...)
		loud := peak(out[0], out[1])
		after := peak(out[0][tail:], out[1][tail:])
		if after > loud/1000 {
			t.Errorf("feedback %g%%, ping-pong %g: %g still sounds after the tail of %d frames, of %g", c.feedback,
				c.pingPong, after, tail, loud)
		}
		if last := peak(out[0][tail-2400:tail], out[1][tail-2400:tail]); last == 0 {
			t.Errorf("feedback %g%%, ping-pong %g: the last 50 ms of the tail of %d frames are silent", c.feedback,
				c.pingPong, tail)
		}
	}
}

// firstSound returns the first frame from at on where x sounds, or -1.
func firstSound(x []float32, at int) int {
	for i := at; i < len(x); i++ {
		if math.Abs(float64(x[i])) > 1e-6 {
			return i
		}
	}
	return -1
}

func TestTheSyncedDelayFollowsTheTempo(t *testing.T) {
	// 120 beats a minute, then 90 from frame 48128, a block's start: a
	// dotted eighth is 18000 frames, then 24000.
	const change = 188 * 256
	p := DelayDescriptor.New(nil)
	pr := p.Processor()
	r := rig{pr: pr, d: &DelayDescriptor, s: setup, tempo: func(at int) float64 {
		if at < change {
			return 120
		}
		return 90
	}}
	var tails []int
	r.before = func(at int) {
		if at == change-setup.MaxBlock*4 || at == change+setup.MaxBlock*4 {
			tails = append(tails, pr.Tail())
		}
	}
	in := impulse(change+2*synth.Rate, 0)
	in[0][change+1000], in[1][change+1000] = 1, 1
	out := r.run(t, in, plug.Change{ID: FeedbackID, Value: 0})
	if got := firstSound(out[0], 0); got != 18000 {
		t.Errorf("at 120 BPM the first echo comes at frame %d, not 18000", got)
	}
	if got := firstSound(out[0], change+1); got != change+1000+24000 {
		t.Errorf("at 90 BPM the first echo comes at frame %d, not %d", got, change+1000+24000)
	}
	if len(tails) != 2 || tails[1] <= tails[0] {
		t.Errorf("the tail is %v frames before and after the tempo falls, where it grows", tails)
	}
	// A note value set mid-play times the echoes from then on.
	p2 := DelayDescriptor.New(nil)
	out = rig{pr: p2.Processor(), d: &DelayDescriptor, s: setup, tempo: func(int) float64 { return 120 }}.run(t,
		impulse(40000, 10000), plug.Change{Frame: 5000, ID: NoteID, Value: 3}, plug.Change{Frame: 5000, ID: FeelID, Value: 2})
	if got := firstSound(out[0], 0); got != 10000+16000 {
		t.Errorf("a quarter triplet at 120 BPM echoes at frame %d, not %d", got, 10000+16000)
	}
}

func TestTheDelaysTimeChangesWithoutAClick(t *testing.T) {
	// A held sine through the delay, its time changed mid-play: no frame
	// steps further from the one before than the sine and its echoes do.
	in := sine(96000, 220, 0.4)
	steady := rig{pr: DelayDescriptor.New(nil).Processor(), d: &DelayDescriptor, s: setup}.run(t, in)
	changed := rig{pr: DelayDescriptor.New(nil).Processor(), d: &DelayDescriptor, s: setup}.run(t, in,
		plug.Change{Frame: 60000, ID: NoteID, Value: 3})
	if got, want := biggestStep(changed[0][59000:70000]), biggestStep(steady[0][59000:70000]); got > want*1.5 {
		t.Errorf("the time changed steps the sound by %g a frame, where it steps %g held", got, want)
	}
}

// biggestStep returns the most x moves from one frame to the next.
func biggestStep(x []float32) float32 {
	var most float32
	for i := 1; i < len(x); i++ {
		most = max(most, float32(math.Abs(float64(x[i]-x[i-1]))))
	}
	return most
}

func TestTheEffectsRunAtAnyRate(t *testing.T) {
	for _, rate := range []float64{44100, 96000, 192000} {
		s := plug.Setup{Rate: rate, MaxBlock: 512, Channels: 2}
		frames := int(rate)
		for _, d := range []*plug.Descriptor{&ReverbDescriptor, &DelayDescriptor, &ColourDescriptor} {
			p := d.New(nil)
			if d == &ColourDescriptor {
				_ = p.SetState(ColourState{Units: []Unit{{Kind: Chorus}, {Kind: Phaser, Slot: 1},
					{Kind: Distortion, Slot: 2}}}.Encode())
			}
			out := rig{pr: p.Processor(), d: d, s: s}.run(t, noise(frames, frames/10))
			for ch := range out {
				for i, v := range out[ch] {
					if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 4 {
						t.Fatalf("%s at %g Hz: channel %d frame %d is %g", d.Name, rate, ch, i, v)
					}
				}
			}
			if peak(out[0][frames/5:], out[1][frames/5:]) == 0 && d != &ColourDescriptor {
				t.Errorf("%s at %g Hz: silence after the sound stops", d.Name, rate)
			}
		}
		// The delay's echo comes a dotted eighth late at 120 BPM, at any
		// rate.
		out := rig{pr: DelayDescriptor.New(nil).Processor(), d: &DelayDescriptor, s: s,
			tempo: func(int) float64 { return 120 }}.run(t, impulse(frames, 0))
		if got, want := firstSound(out[0], 0), int(0.375*rate); got != want {
			t.Errorf("at %g Hz the echo comes at frame %d, not %d", rate, got, want)
		}
	}
}

func TestAMonoSoundRunsThroughEach(t *testing.T) {
	for _, d := range []*plug.Descriptor{&ReverbDescriptor, &DelayDescriptor, &ColourDescriptor} {
		p := d.New(nil)
		res := plugtest.Drive(t, d, p.Processor(), plug.Setup{Rate: synth.Rate, MaxBlock: 300, Channels: 1}, 48000,
			plugtest.Ports{In: noise(48000, 4000)[:1]})
		for i, v := range res.Out[0] {
			if math.IsNaN(float64(v)) {
				t.Fatalf("%s: frame %d of a mono sound is left unwritten", d.Name, i)
			}
		}
		if peak(res.Out[0]) == 0 {
			t.Errorf("%s: a mono sound comes out silent", d.Name)
		}
	}
}
