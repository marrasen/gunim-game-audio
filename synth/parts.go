package synth

// A drums patch plays each of its drums into one of six parts, by the
// drum's General MIDI note, so a host can give each part an output of
// its own: the kick, the snares and claps, the hats, the toms, the
// cymbals, and the rest of the percussion.
const (
	KickPart = iota
	SnarePart
	HatPart
	TomPart
	CymbalPart
	PercPart
	// DrumParts is how many parts there are.
	DrumParts
)

// DrumPartNames are the parts' names, by part.
var DrumPartNames = [DrumParts]string{
	KickPart: "Kick", SnarePart: "Snare", HatPart: "Hats", TomPart: "Toms",
	CymbalPart: "Cymbals", PercPart: "Perc",
}

// drumParts are the parts of General MIDI's drum notes, by note.
var drumParts = func() (parts [128]uint8) {
	for key := range parts {
		parts[key] = PercPart
	}
	for part, keys := range [...][]int{
		KickPart:   {35, 36},
		SnarePart:  {37, 38, 39, 40},
		HatPart:    {42, 44, 46},
		TomPart:    {41, 43, 45, 47, 48, 50},
		CymbalPart: {49, 51, 52, 53, 55, 57, 59},
	} {
		for _, key := range keys {
			parts[key] = uint8(part)
		}
	}
	return parts
}()

// DrumPart returns the part General MIDI's drum note key plays into, as
// KickPart for 36: PercPart for a note of no other part.
func DrumPart(key int) int {
	if key < 0 || key > 127 {
		return PercPart
	}
	return int(drumParts[key])
}

// Part is room for a part's sound, its left and right channels, as long
// as each other.
type Part struct{ L, R []float32 }

// Drums reports whether the instrument plays a drums patch.
func (in *Instrument) Drums() bool { return in.p.kind == kindDrums }

// RenderParts fills l and r, as long as each other, with the
// instrument's sound, as Render does, but for each drum of a part that
// parts has room for: that one fills the part's room, as long as l. A
// part of nil L plays into l and r. A nil parts plays every drum into l
// and r, as Render does.
func (in *Instrument) RenderParts(l, r []float32, parts *[DrumParts]Part) {
	in.RenderSplit(l, r, parts, nil, nil)
}

// RenderSplit fills l and r with the song's notes, as RenderParts does,
// and ll and lr, as long as l, with the live part's: each drum of the
// live part plays into them too, whatever part of the kit it is. Nil ll
// and lr play the live part into l and r, as RenderParts does.
func (in *Instrument) RenderSplit(l, r []float32, parts *[DrumParts]Part, ll, lr []float32) {
	clear(l)
	clear(r)
	clear(ll)
	clear(lr)
	if parts != nil {
		for i := range parts {
			clear(parts[i].L)
			clear(parts[i].R)
		}
	}
	split := ll != nil
	if in.vs != nil {
		for _, v := range in.vs.vs {
			switch {
			case !v.on:
			case split && v.live:
				v.render(ll, lr, &in.ctx)
			default:
				v.render(l, r, &in.ctx)
			}
		}
	}
	if in.sf != nil {
		shift := float64(in.p.live.tune)
		for i := range in.sf.vs {
			switch v := &in.sf.vs[i]; {
			case !v.on:
			case split && v.livePart:
				v.shift = shift
				v.render(ll, lr)
			default:
				v.shift = shift
				v.render(l, r)
			}
		}
	}
	if in.hs != nil {
		for i := range in.hs.hs {
			h := &in.hs.hs[i]
			switch {
			case !h.on:
			case split && h.live:
				h.render(ll, lr, in.ctx.carry)
			case parts != nil && parts[h.part].L != nil:
				h.render(parts[h.part].L, parts[h.part].R, in.ctx.carry)
			default:
				h.render(l, r, in.ctx.carry)
			}
		}
	}
	if in.level != 1 {
		scale(l, r, in.level)
		scale(ll, lr, in.level)
		if parts != nil {
			for i := range parts {
				scale(parts[i].L, parts[i].R, in.level)
			}
		}
	}
}

// scale turns l and r, as long as each other, by level.
func scale(l, r []float32, level float32) {
	for i := range l {
		l[i] *= level
		r[i] *= level
	}
}

// KitParts reports which parts p plays into: for a drums patch, each
// part a drum of its kit plays into on one of General MIDI's notes, as
// the kit falls back to the built-in drums for the notes it leaves out;
// for any other patch, none, as its sound is all one.
func KitParts(p *Patch) (parts [DrumParts]bool) {
	if p == nil || p.Kind != "drums" {
		return parts
	}
	for key := range 128 {
		if kitDrum(p.Kit, key) != "" {
			parts[drumParts[key]] = true
		}
	}
	return parts
}
