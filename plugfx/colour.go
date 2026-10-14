package plugfx

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"

	"github.com/marrasen/gunim-game-audio/synth"
)

// MaxUnits is the most units Colour's chain holds.
const MaxUnits = 6

// The kinds of unit in Colour's chain.
const (
	Chorus     = "chorus"
	Phaser     = "phaser"
	Distortion = "distortion"
)

// UnitKinds are the kinds of unit, in the order the views offer them.
var UnitKinds = []string{Chorus, Phaser, Distortion}

// kindName is how the views name a kind.
var kindName = map[string]string{Chorus: "Chorus", Phaser: "Phaser", Distortion: "Distortion"}

// ChorusModeNames name the chorus's modes, by ChorusModeParam's value,
// as [synth.ChorusKinds] lists them.
var ChorusModeNames = []string{"Soft", "Juno I", "Juno II", "Juno I+II", "Ensemble"}

// DistortTypeNames name the distortion's kinds, by DistortTypeParam's
// value, as [synth.DistortKinds] lists them.
var DistortTypeNames = []string{"Fuzz", "Amp", "Fold"}

// slotStride is how far apart the IDs of two slots' parameters start.
const slotStride = 100

// The parameters of a unit, by their place among its slot's: a unit's
// parameter has the ID [UnitParamID] gives. Each slot has every kind's,
// and its unit uses its own kind's.
const (
	// ChorusModeParam is the chorus's mode, one of ChorusModeNames, and
	// ChorusMixParam how much of it is heard, in percent.
	ChorusModeParam uint32 = 1
	ChorusMixParam  uint32 = 2
	// PhaserRateParam is how fast the phaser sweeps, in hertz,
	// PhaserDepthParam how far, PhaserFeedbackParam how much goes round
	// it, and PhaserMixParam how much of it is heard, 100% the deepest
	// notches, each in percent.
	PhaserRateParam     uint32 = 11
	PhaserDepthParam    uint32 = 12
	PhaserFeedbackParam uint32 = 13
	PhaserMixParam      uint32 = 14
	// DistortTypeParam is the distortion's kind, one of
	// DistortTypeNames, DistortDriveParam how hard it drives, and
	// DistortMixParam how much of it is heard, each in percent.
	// DistortAutoParam, on at 1, keeps the distortion as loud as the
	// sound fed to it, and DistortOutputParam turns the unit's sound up
	// or down after, in decibels.
	DistortTypeParam   uint32 = 21
	DistortDriveParam  uint32 = 22
	DistortMixParam    uint32 = 23
	DistortAutoParam   uint32 = 24
	DistortOutputParam uint32 = 25
)

// UnitParamID returns the ID of the parameter of the unit in slot, from
// 0, at place, as ChorusMixParam.
func UnitParamID(slot int, place uint32) uint32 { return uint32(slot+1)*slotStride + place }

// unitParams are each kind's parameters, as a slot's, by their places.
var unitParams = map[string][]plug.Param{
	Chorus: {
		{ID: ChorusModeParam, Name: "Mode", Max: float64(len(ChorusModeNames) - 1), Default: 1,
			Steps: len(ChorusModeNames) - 1, Flags: plug.Automatable,
			Format: func(v float64) string { return ChorusModeNames[stepOf(v, len(ChorusModeNames))] }},
		{ID: ChorusMixParam, Name: "Mix", Unit: "%", Max: 100, Default: 60, Flags: plug.Automatable, Format: percent},
	},
	Phaser: {
		{ID: PhaserRateParam, Name: "Rate", Unit: "Hz", Min: 0.05, Max: 10, Default: 0.4, Log: true,
			Flags: plug.Automatable, Format: hertz},
		{ID: PhaserDepthParam, Name: "Depth", Unit: "%", Max: 100, Default: 70, Flags: plug.Automatable, Format: percent},
		{ID: PhaserFeedbackParam, Name: "Feedback", Unit: "%", Max: 95, Default: 40, Flags: plug.Automatable,
			Format: percent},
		{ID: PhaserMixParam, Name: "Mix", Unit: "%", Max: 100, Default: 100, Flags: plug.Automatable, Format: percent},
	},
	Distortion: {
		{ID: DistortTypeParam, Name: "Type", Max: float64(len(DistortTypeNames) - 1), Steps: len(DistortTypeNames) - 1,
			Flags: plug.Automatable, Format: func(v float64) string { return DistortTypeNames[stepOf(v, len(DistortTypeNames))] }},
		{ID: DistortDriveParam, Name: "Drive", Unit: "%", Max: 100, Default: 40, Flags: plug.Automatable, Format: percent},
		{ID: DistortMixParam, Name: "Mix", Unit: "%", Max: 100, Default: 100, Flags: plug.Automatable, Format: percent},
		{ID: DistortAutoParam, Name: "Auto gain", Max: 1, Default: 1, Steps: 1, Flags: plug.Automatable, Format: onOff},
		{ID: DistortOutputParam, Name: "Output", Unit: "dB", Min: -24, Max: 12, Flags: plug.Automatable,
			Format: decibels},
	},
}

// ColourDescriptor is Colour, as it is registered.
var ColourDescriptor = func() plug.Descriptor {
	d := plug.Descriptor{
		ID:      "marrasen.colour",
		Name:    "Colour",
		Vendor:  "Marras",
		Version: "0.1.0",
		Kind:    "Fx|Modulation|Distortion",
		New:     func(h plug.Host) plug.Plugin { return newColour(h) },
	}
	for slot := range MaxUnits {
		for _, kind := range UnitKinds {
			for _, prm := range unitParams[kind] {
				prm.ID = UnitParamID(slot, prm.ID)
				prm.Name = fmt.Sprintf("%d %s %s", slot+1, kindName[kind], prm.Name)
				d.Params = append(d.Params, prm)
			}
		}
	}
	d.Patches = []plug.Patch{
		{Name: "Juno chorus", State: ColourState{Units: []Unit{{Kind: Chorus}}}.Encode()},
		{Name: "String ensemble", State: ColourState{Units: []Unit{{Kind: Chorus}}}.Encode(),
			Params: map[uint32]float64{UnitParamID(0, ChorusModeParam): 4, UnitParamID(0, ChorusMixParam): 80}},
		{Name: "Phased", State: ColourState{Units: []Unit{{Kind: Phaser}}}.Encode()},
		{Name: "Fuzz into chorus", State: ColourState{Units: []Unit{{Kind: Distortion}, {Kind: Chorus, Slot: 1}}}.Encode(),
			Params: map[uint32]float64{UnitParamID(1, ChorusModeParam): 2}},
		{Name: "Amp, phaser, chorus",
			State: ColourState{Units: []Unit{{Kind: Distortion}, {Kind: Phaser, Slot: 1}, {Kind: Chorus, Slot: 2}}}.Encode(),
			Params: map[uint32]float64{UnitParamID(0, DistortTypeParam): 1, UnitParamID(0, DistortDriveParam): 55,
				UnitParamID(1, PhaserMixParam): 60}},
	}
	return d
}()

// slotParam returns the parameter of ID id, its slot and its place, and
// whether it is one of a slot's.
func slotParam(id uint32) (prm plug.Param, slot int, place uint32, ok bool) {
	slot, place = int(id/slotStride)-1, id%slotStride
	if slot < 0 || slot >= MaxUnits {
		return plug.Param{}, 0, 0, false
	}
	prm, ok = ColourDescriptor.Param(id)
	return prm, slot, place, ok
}

// colourPlugin is one Colour.
type colourPlugin struct {
	host plug.Host
	// mu guards units, which State and SetState reach from the app's
	// goroutine and the views from the window's.
	mu    sync.Mutex
	units []Unit
	proc  colourProc
}

func newColour(h plug.Host) *colourPlugin {
	p := &colourPlugin{host: h, units: defaultUnits()}
	p.proc.offer(p.units)
	return p
}

// defaultUnits is the chain of an empty state: a Juno's chorus.
func defaultUnits() []Unit { return []Unit{{Kind: Chorus}} }

// Processor implements [plug.Plugin].
func (p *colourPlugin) Processor() plug.Processor { return &p.proc }

// State implements [plug.Plugin]: the chain, as [ColourState] writes
// it.
func (p *colourPlugin) State() []byte { return ColourState{Units: p.Units()}.Encode() }

// SetState implements [plug.Plugin]: it runs the chain the state holds,
// and a Juno's chorus for an empty one. A state it cannot read changes
// nothing. The chain is made here, and handed to the processor whole,
// which crossfades to it.
func (p *colourPlugin) SetState(b []byte) error {
	units := defaultUnits()
	if len(b) > 0 {
		st, err := DecodeColourState(b)
		if err != nil {
			return err
		}
		units = st.Units
	}
	p.setUnits(units)
	return nil
}

// Units returns a copy of the chain's units, in their order.
func (p *colourPlugin) Units() []Unit {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.units)
}

// setUnits makes units the chain, for the sound too.
func (p *colourPlugin) setUnits(units []Unit) {
	units = slices.Clone(units)
	p.mu.Lock()
	p.units = units
	p.mu.Unlock()
	p.proc.offer(units)
}

// Close implements [plug.Plugin].
func (p *colourPlugin) Close() {}

// View implements [plug.Viewer].
func (p *colourPlugin) View(s plug.Size, h plug.Host) gunim.Node { return colourView(p, s, h) }

// colourUnit is a unit of the chain at work.
type colourUnit interface {
	// apply sets the unit as its slot's parameters, by their places,
	// say.
	apply(v *slotValues)
	// process runs the sound in l and r through the unit.
	process(l, r []float32)
	// carry makes the unit sound on as o, of its kind, does, from where
	// o is.
	carry(o colourUnit)
	clear()
}

// slotValues are a slot's parameters' values, by their places.
type slotValues [slotStride]float64

type chorusUnit struct{ c *synth.ChorusUnit }

func (u *chorusUnit) apply(v *slotValues) {
	u.c.Set(synth.ChorusKinds[stepOf(v[ChorusModeParam], len(synth.ChorusKinds))], v[ChorusMixParam]/100)
}
func (u *chorusUnit) process(l, r []float32) { u.c.Process(l, r) }
func (u *chorusUnit) carry(o colourUnit) {
	if c, ok := o.(*chorusUnit); ok {
		u.c.CopyFrom(c.c)
	}
}
func (u *chorusUnit) clear() { u.c.Clear() }

type phaserUnit struct{ p *synth.PhaserUnit }

func (u *phaserUnit) apply(v *slotValues) {
	u.p.Set(v[PhaserRateParam], v[PhaserDepthParam]/100, v[PhaserFeedbackParam]/100, v[PhaserMixParam]/100)
}
func (u *phaserUnit) process(l, r []float32) { u.p.Process(l, r) }
func (u *phaserUnit) carry(o colourUnit) {
	if p, ok := o.(*phaserUnit); ok {
		u.p.CopyFrom(p.p)
	}
}
func (u *phaserUnit) clear() { u.p.Clear() }

// distortUnit is a distortion, its loudness matched to the sound it is
// fed, blended with that sound, and turned up or down: dl and dr keep
// that sound while it runs.
type distortUnit struct {
	d      *synth.DistortionUnit
	auto   autoGain
	mix    blend
	dl, dr []float32
	// dry and wet are the block's sound fed in and out, as blend takes
	// them.
	dry, wet [2][]float32
	// out is the gain after the blend, gliding to outTo.
	out, outTo float32
}

func (u *distortUnit) apply(v *slotValues) {
	u.d.Set(v[DistortDriveParam]/100, synth.DistortKinds[stepOf(v[DistortTypeParam], len(synth.DistortKinds))])
	u.mix.mixTo = float32(v[DistortMixParam] / 100)
	u.auto.on = v[DistortAutoParam] > 0.5
	u.outTo = float32(math.Pow(10, v[DistortOutputParam]/20))
}

func (u *distortUnit) process(l, r []float32) {
	n := len(l)
	copy(u.dl, l)
	copy(u.dr, r)
	u.d.Process(l, r)
	u.auto.run(u.dl[:n], u.dr[:n], l, r)
	u.dry, u.wet = [2][]float32{u.dl[:n], u.dr[:n]}, [2][]float32{l, r}
	u.mix.run(u.dry[:], u.wet[:], l, r, 0, n)
	for i := range l {
		u.out += (u.outTo - u.out) * u.mix.glide
		l[i] *= u.out
		r[i] *= u.out
	}
}

func (u *distortUnit) carry(o colourUnit) {
	if d, ok := o.(*distortUnit); ok {
		u.d.CopyFrom(d.d)
		u.mix.mix = d.mix.mix
		u.auto.carry(&d.auto)
		u.out = d.out
	}
}

func (u *distortUnit) clear() {
	u.d.Clear()
	u.mix.jump()
	u.auto.clear()
	u.out = u.outTo
}

// Auto gain's timing and its bounds. autoGainEnv is how long, in
// seconds, its measures of the sound in and out take to follow them,
// and autoGainGlide how long its gain takes to follow the measures.
// autoGainWarm is how long the sound in must be measured after a clear
// before the gain glides: till then it drops at once and rises over
// autoGainRise, from silence, so a first note does not come out loud.
// autoGainFloor is the mean square, -80 dBFS, under which
// the sound in is too quiet to measure, and the gain holds where it is.
// autoGainMax and autoGainMin are the most it turns the distortion up,
// +6 dB, and down, -48 dB.
const (
	autoGainEnv   = 0.12
	autoGainGlide = 0.04
	autoGainWarm  = 0.02
	autoGainRise  = 0.01
	autoGainFloor = 1e-8
	autoGainMax   = 2
	autoGainMin   = 1.0 / 256
)

// autoGain keeps a distortion as loud as the sound fed to it: it
// follows the mean square of each and turns the distortion by the root
// of their ratio. Off, its gain glides to 1, and the distortion passes
// as it is. On, its gain is 0 till it has measured anything, so a
// distortion of what is too quiet to measure stays silent.
type autoGain struct {
	on bool
	// dry and wet are the mean squares of the sound in and out, and
	// gain the gain on the sound out.
	dry, wet, gain float32
	// warm is how many frames more of measured sound the gain warms up
	// over, not glides, and warmLen how many it does after a clear.
	warm, warmLen int
	// env, glide and rise are how far the measures and the gain move
	// toward where they are going each frame.
	env, glide, rise float32
}

// prepare readies the gain for sound at rate frames a second.
func (a *autoGain) prepare(rate float64) {
	a.env = float32(1 - math.Exp(-1/(autoGainEnv*rate)))
	a.glide = float32(1 - math.Exp(-1/(autoGainGlide*rate)))
	a.warmLen = max(int(autoGainWarm*rate), 1)
	a.rise = float32(1 - math.Exp(-1/(autoGainRise*rate)))
	a.clear()
}

// clear forgets the sound measured.
func (a *autoGain) clear() { a.dry, a.wet, a.gain, a.warm = 0, 0, 0, a.warmLen }

// carry makes a measure as o's, from where o is.
func (a *autoGain) carry(o *autoGain) { a.dry, a.wet, a.gain, a.warm = o.dry, o.wet, o.gain, o.warm }

// run turns the distortion in wl and wr to the loudness of the sound
// fed to it in dl and dr.
func (a *autoGain) run(dl, dr, wl, wr []float32) {
	for i := range wl {
		a.dry += ((dl[i]*dl[i]+dr[i]*dr[i])*0.5 - a.dry) * a.env
		a.wet += ((wl[i]*wl[i]+wr[i]*wr[i])*0.5 - a.wet) * a.env
		measured := a.dry >= autoGainFloor
		switch {
		case !a.on && a.warm > 0:
			a.gain = 1
		case !a.on:
			a.gain += (1 - a.gain) * a.glide
		case !measured:
			// Too quiet to tell: the gain holds.
		default:
			to := float32(autoGainMax)
			if a.wet > 0 {
				to = min(max(float32(math.Sqrt(float64(a.dry/a.wet))), autoGainMin), autoGainMax)
			}
			if a.warm > 0 {
				// Down at once, and up quickly, as the measures may lag
				// what a cabinet holds back.
				a.gain = min(to, a.gain+(to-a.gain)*a.rise)
			} else {
				a.gain += (to - a.gain) * a.glide
			}
		}
		if measured && a.warm > 0 {
			a.warm--
		}
		wl[i] *= a.gain
		wr[i] *= a.gain
	}
	// What silence leaves of the measures is let go of before it is
	// too small for a float to hold quickly.
	if a.dry < 1e-20 {
		a.dry = 0
	}
	if a.wet < 1e-20 {
		a.wet = 0
	}
}

// chain is the units at work, in their order, each in its slot.
type chain struct {
	units []colourUnit
	plan  []Unit
}

// newChain makes the units of plan for sound at rate frames a second,
// in blocks of up to block frames.
func newChain(plan []Unit, rate float64, block int) *chain {
	c := &chain{plan: slices.Clone(plan)}
	for _, u := range plan {
		switch u.Kind {
		case Chorus:
			c.units = append(c.units, &chorusUnit{synth.NewChorusUnit(rate, synth.ChorusKinds[1])})
		case Phaser:
			c.units = append(c.units, &phaserUnit{synth.NewPhaserUnit(rate)})
		default:
			d := &distortUnit{d: synth.NewDistortionUnit(rate), dl: make([]float32, block), dr: make([]float32, block)}
			d.mix.prepare(rate)
			d.mix.widthTo = 1
			d.auto.prepare(rate)
			d.out, d.outTo = 1, 1
			c.units = append(c.units, d)
		}
	}
	return c
}

// find returns the unit of kind in slot, or nil.
func (c *chain) find(slot int, kind string) colourUnit {
	for i, u := range c.plan {
		if u.Slot == slot && u.Kind == kind {
			return c.units[i]
		}
	}
	return nil
}

func (c *chain) process(l, r []float32) {
	for _, u := range c.units {
		u.process(l, r)
	}
}

// colourWarm is how long a new chain runs unheard, in seconds, longer
// than a chorus's copies come late, so the sound held in its units is
// the sound it now runs on; colourFade is how long the sound then takes
// to cross over to it; and colourTail is how long a chain rings on.
const (
	colourWarm = 0.02
	colourFade = 0.012
	colourTail = 0.1
)

// colourProc runs the chain. A new chain, made off the audio thread,
// comes through next; the processor takes it as a block starts, its
// units of the chain before carried on in it. It runs unheard for
// colourWarm, and the sound then crosses over to it over colourFade
// from the chain before, which runs on till then.
type colourProc struct {
	// mu guards rate, block and plan, which Prepare and offer reach, off
	// the audio thread.
	mu    sync.Mutex
	rate  float64
	block int
	plan  []Unit
	// cur is the chain at work, and old the one it crosses over from,
	// for fade frames more: warm and then fadeLen.
	cur, old            *chain
	next                atomic.Pointer[chain]
	fade, warm, fadeLen int
	vals                [MaxUnits]slotValues
	al, ar              []float32
	bl, br              []float32
	tail                atomic.Int64
}

// offer makes the chain of plan and hands it to the processor, for a
// block to take, where the processor is prepared; Prepare makes it
// where not yet.
func (p *colourProc) offer(plan []Unit) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plan = slices.Clone(plan)
	if p.rate > 0 {
		p.next.Store(newChain(p.plan, p.rate, p.block))
	}
}

// Prepare implements [plug.Processor]: the chain is made anew for the
// rate, and every buffer for the blocks.
func (p *colourProc) Prepare(s plug.Setup) {
	p.mu.Lock()
	p.rate = s.Rate
	if p.rate <= 0 {
		p.rate = synth.Rate
	}
	p.block = max(s.MaxBlock, 1)
	p.cur = newChain(p.plan, p.rate, p.block)
	p.next.Store(nil)
	p.mu.Unlock()
	for _, prm := range ColourDescriptor.Params {
		p.take(plug.Change{ID: prm.ID, Value: prm.Default})
	}
	p.old, p.fade = nil, 0
	p.warm, p.fadeLen = int(colourWarm*p.rate), max(int(colourFade*p.rate), 1)
	for _, b := range []*[]float32{&p.al, &p.ar, &p.bl, &p.br} {
		*b = make([]float32, p.block)
	}
	p.keepTail()
	p.Reset()
}

// applyAll sets each of c's units as its slot's parameters say.
func (p *colourProc) applyAll(c *chain) {
	for i, u := range c.units {
		u.apply(&p.vals[c.plan[i].Slot])
	}
}

// keepTail keeps the chain's tail, for Tail.
func (p *colourProc) keepTail() {
	t := int64(0)
	if len(p.cur.units) > 0 {
		t = int64(math.Ceil(colourTail * p.rate))
	}
	p.tail.Store(t)
}

// take takes a change to a parameter.
func (p *colourProc) take(c plug.Change) {
	prm, slot, place, ok := slotParam(c.ID)
	if !ok {
		return
	}
	p.vals[slot][place] = prm.Clamp(c.Value)
	for _, ch := range []*chain{p.cur, p.old} {
		if ch == nil {
			continue
		}
		for i, u := range ch.plan {
			if u.Slot == slot {
				ch.units[i].apply(&p.vals[slot])
			}
		}
	}
}

// adopt takes the chain offered, if any, unless a crossover is under
// way: its units carry on from those of the chain at work of the same
// slot and kind, and the sound crosses over to it.
func (p *colourProc) adopt() {
	if p.fade > 0 {
		return
	}
	c := p.next.Swap(nil)
	if c == nil {
		return
	}
	for i, u := range c.units {
		if o := p.cur.find(c.plan[i].Slot, c.plan[i].Kind); o != nil {
			u.carry(o)
		}
	}
	p.applyAll(c)
	p.old, p.cur, p.fade = p.cur, c, p.warm+p.fadeLen
	p.keepTail()
}

// Process implements [plug.Processor].
func (p *colourProc) Process(b *plug.Block) {
	if p.cur == nil || b.Frames > len(p.al) {
		return
	}
	p.adopt()
	runSegments(b, p)
}

// segment runs frames from to to.
func (p *colourProc) segment(b *plug.Block, from, to int) {
	al, ar := p.al[from:to], p.ar[from:to]
	feed(b.In, p.al, p.ar, from, to)
	if p.fade > 0 && p.old != nil {
		bl, br := p.bl[from:to], p.br[from:to]
		copy(bl, al)
		copy(br, ar)
		p.old.process(bl, br)
		p.cur.process(al, ar)
		for i := range al {
			if p.fade == 0 {
				break
			}
			t := min(float32(p.fade)/float32(p.fadeLen), 1)
			al[i] += (bl[i] - al[i]) * t
			ar[i] += (br[i] - ar[i]) * t
			p.fade--
		}
		if p.fade == 0 {
			p.old = nil
		}
	} else {
		p.cur.process(al, ar)
	}
	outL, outR := sides(b.Out)
	if outL == nil {
		return
	}
	if len(b.Out) == 1 {
		for i := range al {
			outL[from+i] = (al[i] + ar[i]) * 0.5
		}
		return
	}
	copy(outL[from:to], al)
	copy(outR[from:to], ar)
	for _, ch := range b.Out[2:] {
		clear(ch[from:to])
	}
}

// Latency implements [plug.Processor]: the chain sounds on time, 0.
func (p *colourProc) Latency() int { return 0 }

// Tail implements [plug.Processor]: the longest a chorus's copies come
// late, and a phaser's feedback rings, where the chain has units.
func (p *colourProc) Tail() int { return int(p.tail.Load()) }

// Reset implements [plug.Processor]: what the units hold, silenced; a
// crossover under way ends on the chain at work.
func (p *colourProc) Reset() {
	if p.cur == nil {
		return
	}
	for _, u := range p.cur.units {
		u.clear()
	}
	p.old, p.fade = nil, 0
}
