package synth

import "fmt"

// PreviewPatch renders a note of p alone, mono, as a tool draws it: the
// note pitch held for hold seconds and let go, secs in all, with no
// effects after it.
func PreviewPatch(p *Patch, pitch int, hold, secs float64) ([]float32, error) {
	c, err := p.compile("preview")
	if err != nil {
		return nil, err
	}
	n := int(secs * rate)
	out := make([]float32, n)
	ctx := &renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)}
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	if c.kind == kindDrums {
		return nil, fmt.Errorf("synth: %s is a drums patch; preview its drums", "preview")
	}
	vs := newVoices(1, 1)
	vs.play(c, note{pitch: float32(pitch), vel: 1, gate: int64(hold * rate), res: -1})
	for at := 0; at < n; at += maxBlock {
		m := min(maxBlock, n-at)
		clear(l[:m])
		clear(r[:m])
		vs.render(l[:m], r[:m], ctx)
		for i := range m {
			out[at+i] = (l[i] + r[i]) / 2
		}
	}
	return out, nil
}

// PreviewDrum renders the drum named drum of the drums patch p, mono,
// secs long, as a tool draws it. A timpani plays D2.
func PreviewDrum(p *Patch, drum string, secs float64) ([]float32, error) {
	c, err := p.compile("preview")
	if err != nil {
		return nil, err
	}
	d, ok := c.kit[drum]
	if c.kind != kindDrums || !ok {
		return nil, fmt.Errorf("synth: no drum %q in the kit", drum)
	}
	n := int(secs * rate)
	out := make([]float32, n)
	hs := newHits(1)
	hs.play(d, 1, 0, 38, int64(0.5*rate), 0)
	l, r := make([]float32, maxBlock), make([]float32, maxBlock)
	for at := 0; at < n; at += maxBlock {
		m := min(maxBlock, n-at)
		clear(l[:m])
		clear(r[:m])
		hs.render(l[:m], r[:m])
		for i := range m {
			out[at+i] = (l[i] + r[i]) / 2
		}
	}
	return out, nil
}
