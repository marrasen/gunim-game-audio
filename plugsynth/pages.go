package plugsynth

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

// The panel's pages: Sound, the browser and the knobs used most;
// Patch, the patch editor; and Controls, a knob for every parameter, for
// a host's automation and control wires to drive.
const (
	SoundPage = iota
	PatchPage
	ControlsPage
)

// PageNames are the pages' names, as the tabs show them.
var PageNames = []string{"Sound", "Patch", "Controls"}

// pages are the panel's tabs and the pages beside its Sound page.
type pages struct {
	tabs     *widget.Segmented
	editor   *patchEditor
	controls *widget.Scroll
	sections []*plugui.Section
}

// tabsW is how wide the tabs stand.
const tabsW = 276

// newPages returns the pages of panel v.
func newPages(v *panel, h plug.Host) *pages {
	pg := &pages{tabs: widget.NewSegmented(PageNames...)}
	pg.tabs.OnChange = func(_ int, u *gunim.UI) gunim.Intent {
		u.Invalidate()
		return nil
	}
	pg.editor = newPatchEditor(v.p, h, v.edited)
	sec := func(name string, ids ...uint32) *plugui.Section {
		return plugui.NewSection(h, &Descriptor, name, 0, ids...)
	}
	pg.sections = []*plugui.Section{
		sec("FILTER", CutoffID, ResID, EnvID),
		sec("ENVELOPE", AttackID, DecayID, SustainID, ReleaseID),
		sec("TONE", DriveID, NoiseID, WidthID, FMID),
		sec("PITCH", TuneID),
		sec("LFO", LFORateID, LFODepthID),
		sec("VOICE", VowelID),
		sec("PLAYED", ModID, PressureID),
		sec("OUTPUT", GainID, PanID),
		sec("AHEAD", SwellID, GhostID),
	}
	pg.controls = widget.NewScroll(&flow{cards: pg.sections})
	return pg
}

// page is the page shown.
func (v *panel) page() int { return v.pages.tabs.Selected() }

// ShowPage shows the page named name, one of PageNames, and reports
// whether there is one, for a host to open the panel on it.
func (v *panel) ShowPage(name string, u *gunim.UI) bool {
	for i, n := range PageNames {
		if n == name {
			v.pages.tabs.SetSelected(i, u)
			if u != nil {
				u.Invalidate()
			}
			return true
		}
	}
	return false
}

// edited follows an edit the patch editor made: the panel shows the
// patch as it now is.
func (v *panel) edited(u *gunim.UI) {
	v.st = v.p.sound()
	v.dim()
	if v.keys != nil {
		v.keys.follow(v.st.Patch.Kind)
	}
	u.Invalidate()
}

// pageNodes are the tabs and the pages, as the panel's last children.
func (pg *pages) nodes() []gunim.Node { return []gunim.Node{pg.tabs, pg.editor, pg.controls} }

// layoutPages places the tabs at the right of the line under the name,
// and
// lays the pages out where page shows, in area, the others in none.
func (v *panel) layoutPages(kids gunim.Children, right float32, area geom.Rect) {
	n := kids.Len()
	tabs, editor, controls := kids.At(n-3), kids.At(n-2), kids.At(n-1)
	tabs.Layout(gunim.Tight(geom.Sz(tabsW, widget.SegmentedHeight.Get(nil))))
	tabs.Place(geom.Pt(right-tabsW, crumbY-7))
	for i, k := range []gunim.Child{editor, controls} {
		if v.page() == i+1 {
			k.Layout(gunim.Tight(area.Size()))
			k.Place(area.Min)
			continue
		}
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Point{})
	}
}

// layoutPage lays out a page other than Sound: the steps and the tabs
// in the header, the page under the header, and on the Patch page
// the keys under it, where the host lets them play; the Sound page's
// own parts take no room.
func (v *panel) layoutPage(size geom.Size, kids gunim.Children) geom.Size {
	d := v.detail(size)
	right := d.Max.X - panelInset
	kids.At(0).Layout(gunim.Tight(geom.Sz(stepW, stepW)))
	kids.At(0).Place(geom.Pt(right-2*stepW-4, nameY-2))
	kids.At(1).Layout(gunim.Tight(geom.Sz(stepW, stepW)))
	kids.At(1).Place(geom.Pt(right-stepW, nameY-2))
	for i := 2; i < kids.Len()-3; i++ {
		kids.At(i).Layout(gunim.Tight(geom.Size{}))
		kids.At(i).Place(geom.Point{})
	}
	area := geom.Rect{Min: geom.Pt(d.Min.X+panelInset/2, crumbY+26), Max: geom.Pt(d.Max.X-panelInset/2, size.H-panelInset/2)}
	v.shown = false
	if v.keys != nil && v.page() == PatchPage && area.Size().H > 200 {
		h := float32(64)
		w := min(area.Size().W, 15*28+2*(shiftW+4))
		area.Max.Y -= h + 10
		k := kids.At(kids.Len() - 4)
		k.Layout(gunim.Tight(geom.Sz(w, h)))
		k.Place(geom.Pt(d.Min.X+(d.Size().W-w)/2, area.Max.Y+6))
		v.shown = true
	}
	v.layoutPages(kids, right, area)
	return size
}

// shows reports whether child i of kids paints on the page shown.
func (v *panel) shows(i, n int) bool {
	switch v.page() {
	case SoundPage:
		return i < n-2
	case PatchPage:
		return i < 2 || i >= n-3 && i != n-1 || v.keys != nil && v.shown && i == n-4
	}
	return i < 2 || i == n-3 || i == n-1
}
