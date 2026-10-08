package main

import (
	"image/color"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-game-audio/synth"
)

// kitPane edits a drums patch: its drums as pads, which light as the
// song hits them and play as they are pressed, and the drum chosen, its
// type, its knobs, and its sound drawn.
type kitPane struct {
	*widget.Scroll
	base    string
	name    string
	drum    string
	names   []string
	picker  *widget.Dropdown
	about   *widget.Label
	copy    *widget.IconButton
	load    *widget.IconButton
	save    *widget.IconButton
	pads    *pads
	title   *widget.Label
	typ     *widget.Dropdown
	ks      []*knob
	hit     *wave
	close   *wave
	changed func(kit, drum string) gunim.Intent
}

func newKitPane(changed func(kit, drum string) gunim.Intent) *kitPane {
	kp := &kitPane{changed: changed, drum: "bd"}
	kp.picker = widget.NewDropdown("–")
	kp.picker.Label = "Kit"
	kp.picker.OnChange = func(i int) gunim.Intent {
		if i < len(kp.names) {
			kp.choose(kp.names[i])
		}
		return kp.changed(kp.name, kp.drum)
	}
	newKit := widget.NewButton("New kit")
	newKit.Icon, newKit.On, newKit.Ghost = icon.Plus, PatchNew{Kind: "drums"}, true
	kp.copy = widget.NewIconButton(icon.Copy, "Copy the kit")
	kp.load = widget.NewIconButton(icon.FolderOpen, "Load a kit from a file in this one's place")
	kp.save = widget.NewIconButton(icon.Save, "Save the kit to a file")
	importK := widget.NewIconButton(icon.FileInput, "Load a kit from a file as a new one")
	importK.On = PatchLoad{}
	kp.about = small("")
	gap := widget.NewSpacer()
	head := widget.Row(kp.picker, kp.about, gap, newKit, kp.copy, importK, kp.load, kp.save).Grow(gap, 1)
	head.Cross = widget.CrossCenter

	kp.pads = &pads{names: synth.DrumNames}
	kp.pads.press = func(name string, u *gunim.UI) {
		kp.drum = name
		u.Send(kp.pads, Audition{Patch: kp.name, Drum: name})
		u.Send(kp.pads, kp.changed(kp.name, name))
	}
	kp.title = widget.NewLabel("")
	kp.title.Size = widget.HeadingSize
	kp.typ = widget.NewDropdown(synth.DrumTypes...)
	kp.typ.Label = "Type"
	kp.typ.OnChange = func(i int) gunim.Intent {
		return SetValue{Path: kp.base + "/Kit/" + kp.drum + "/Type", Str: synth.DrumTypes[i], IsStr: true}
	}
	b := &kp.base
	kp.ks = []*knob{
		newKnob("Tune", b, "", -24, 24, 0).center().units("st"),
		newKnob("Decay", b, "", 0.1, 4, 1).logScale().unsetIs(1),
		newKnob("Tone", b, "", 0, 1, 0),
		newKnob("Level", b, "", 0, 2, 1).unsetIs(1),
		newKnob("Pan", b, "", -1, 1, 0).center(),
	}
	kp.hit = &wave{spans: true, label: "the hit", fit: true}
	kp.close = &wave{label: "its first 50 ms", fit: true}
	play := widget.NewButton("Play it")
	play.Icon = icon.Play
	play.KeepFocus = true
	kp.pads.playButton = play
	drumHead := widget.Row(kp.title, widget.NewSpacer(), kp.typ, play)
	drumHead.Grow(drumHead.Children()[1], 1)
	drumHead.Cross = widget.CrossCenter
	editor := panelWith(drumHead, knobs(kp.ks...), sized(kp.hit, 0, 110), sized(kp.close, 0, 90))
	padPanel := panel("PADS · press one to play it and edit it", sized(kp.pads, 0, 340))
	body := widget.Row(padPanel, editor).Grow(padPanel, 1.1).Grow(editor, 1)
	body.Cross = widget.CrossStretch
	col := widget.Column(head, body)
	col.Cross = widget.CrossStretch
	kp.Scroll = widget.NewScroll(widget.NewPad(col))
	return kp
}

func (kp *kitPane) choose(name string) {
	kp.name = name
	kp.base = "Patches/" + name
}

// update shows st.
func (kp *kitPane) update(st Studio) {
	song := st.Doc
	if song == nil {
		return
	}
	var names []string
	for _, n := range sortedNames(song.Patches) {
		if song.Patches[n].Kind == "drums" {
			names = append(names, n)
		}
	}
	if !slices.Equal(names, kp.names) {
		kp.names = names
		kp.picker.Items = names
		if len(names) == 0 {
			kp.picker.Items = []string{"no kits"}
		}
	}
	if _, ok := song.Patches[kp.name]; !ok && len(names) > 0 {
		kp.choose(names[0])
	}
	kp.picker.Selected = max(slices.Index(names, kp.name), 0)
	p := song.Patches[kp.name]
	if p == nil {
		return
	}
	var users []string
	for _, t := range song.Tracks {
		if t.Patch == kp.name {
			users = append(users, t.Name)
		}
	}
	kp.about.SetText("kit · played by " + joinNames(users))
	if len(users) == 0 {
		kp.about.SetText("kit · played by nothing yet")
	}
	kp.copy.On, kp.load.On, kp.save.On = PatchCopy{Name: kp.name}, PatchLoad{Name: kp.name}, PatchSave{Name: kp.name}
	kp.pads.kit, kp.pads.chosen = p, kp.drum
	kp.pads.playButton.On = Audition{Patch: kp.name, Drum: kp.drum}
	typ := p.DrumType(kp.drum)
	kp.title.SetText(kp.drum)
	kp.typ.Selected = max(slices.Index(synth.DrumTypes, typ), 0)
	for i, rel := range []string{"Tune", "Decay", "Tone", "Gain", "Pan"} {
		kp.ks[i].rel = "/Kit/" + kp.drum + "/" + rel
	}
	showKnobs(song, kp.ks...)
	if st.DrumPreview.Patch == kp.name && st.DrumPreview.Drum == kp.drum {
		kp.hit.data, kp.close.data = st.DrumPreview.Wave, st.DrumPreview.Cycle
	}
	// The pads the song hits now light.
	heard := heardAt(st.Clock, st.Clock.At)
	for _, n := range st.Notes {
		if !n.Drum || n.Track < 0 || n.Track >= len(st.Tracks) || st.Tracks[n.Track].Patch != kp.name {
			continue
		}
		if age := heard - float64(n.Frame); age >= 0 && age < 0.05*audioRate {
			kp.pads.flash(n.DrumName, n.Vel)
		}
	}
}

// pads are a kit's drums as pads, four across, each its drum's name and
// type, lit as the drum is hit.
type pads struct {
	names      []string
	kit        *synth.Patch
	chosen     string
	press      func(name string, u *gunim.UI)
	glow       map[string]float32
	size       geom.Size
	playButton *widget.Button
}

const padCols = 4

func (pd *pads) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	pd.size = c.Max
	return c.Max
}

func (pd *pads) rect(i int) geom.Rect {
	rows := (len(pd.names) + padCols - 1) / padCols
	w := pd.size.W / padCols
	h := pd.size.H / float32(rows)
	return geom.Rc(float32(i%padCols)*w+3, float32(i/padCols)*h+3, w-6, h-6)
}

func (pd *pads) flash(name string, vel float32) {
	if pd.glow == nil {
		pd.glow = map[string]float32{}
	}
	pd.glow[name] = max(pd.glow[name], 0.4+0.6*vel)
}

func (pd *pads) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := audioui.Ink.Get(f.Theme)
	for i, name := range pd.names {
		r := pd.rect(i)
		g := pd.glow[name]
		c := padColor(i)
		fill := lighten(color.NRGBA{0x26, 0x2b, 0x38, 0xff}, 0)
		fill = mixColor(fill, c, 0.25+0.6*g)
		op := paint.RRectOp{Rect: r, Radius: 10, Fill: paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y),
			Start: lighten(fill, 0.08), End: fill}},
			Inset: [2]paint.Shadow{{Offset: geom.Pt(0, -3), Blur: 6, Color: color.NRGBA{A: 0x60}}}}
		if g > 0.05 {
			op.Shadow = paint.Shadow{Blur: 16 * g, Color: withAlpha(c, g)}
		}
		if name == pd.chosen {
			op.Stroke = paint.Stroke{Width: 2, Color: lighten(c, 0.4)}
		}
		p.DrawRRect(op)
		run := audioui.Shaped(name, 15, true, false)
		run.Paint(p, geom.Pt(r.Min.X+10, r.Min.Y+8), ink)
		typ := ""
		if pd.kit != nil {
			typ = pd.kit.DrumType(name)
		}
		t := audioui.Shaped(typ, 10.5, false, false)
		t.Paint(p, geom.Pt(r.Min.X+10, r.Max.Y-t.Height()-8), withAlpha(ink, 0.55))
	}
}

func (pd *pads) Handle(e input.Event, u *gunim.UI) bool {
	if d, ok := e.(input.PointerDown); ok {
		for i, name := range pd.names {
			if !pd.rect(i).Contains(d.Pos) {
				continue
			}
			pd.flash(name, 1)
			pd.chosen = name
			if pd.press != nil {
				pd.press(name, u)
			}
			u.Invalidate()
			return true
		}
	}
	return false
}

func (pd *pads) Step(dt time.Duration) bool {
	moving := false
	for k, g := range pd.glow {
		g -= float32(dt.Seconds()) * 3
		if g <= 0 {
			delete(pd.glow, k)
			continue
		}
		pd.glow[k] = g
		moving = true
	}
	return moving
}

// padColor is pad i's colour.
func padColor(i int) color.NRGBA {
	cs := []color.NRGBA{{0xff, 0x6b, 0x5f, 0xff}, {0xff, 0xa2, 0x4c, 0xff}, {0xff, 0xcf, 0x5c, 0xff}, {0x9c, 0xe0, 0x5a, 0xff},
		{0x4f, 0xd6, 0xc0, 0xff}, {0x5c, 0xb8, 0xff, 0xff}, {0x7c, 0x8c, 0xff, 0xff}, {0xc5, 0x8c, 0xff, 0xff}, {0xff, 0x6b, 0xd5, 0xff}}
	return cs[i%len(cs)]
}

// mixColor blends a toward b by t.
func mixColor(a, b color.NRGBA, t float32) color.NRGBA {
	t = min(max(t, 0), 1)
	m := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}
