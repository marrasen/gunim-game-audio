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
	clear(l)
	clear(r)
	if parts != nil {
		for i := range parts {
			clear(parts[i].L)
			clear(parts[i].R)
		}
	}
	if in.vs != nil {
		in.vs.render(l, r, &in.ctx)
	}
	if in.sf != nil {
		in.sf.render(l, r, float64(in.p.live.tune))
	}
	if in.hs != nil {
		if parts != nil {
			in.hs.renderParts(l, r, parts, in.ctx.carry)
		} else {
			in.hs.render(l, r, in.ctx.carry)
		}
	}
	if in.level != 1 {
		scale(l, r, in.level)
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
