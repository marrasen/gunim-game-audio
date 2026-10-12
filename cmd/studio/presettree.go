package main

import (
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// presetTree is the patch library, docked open beside an editor: its
// categories as folders, the songs' patches a folder a song in theirs,
// each a click to open or close, and a preset a click to try. The one
// tried is lit, and its folders open of themselves.
type presetTree struct {
	// drums says it shows the kits, else the synths and plucks; patch
	// is the patch it tries presets on.
	drums bool
	patch string
	rows  []PresetRow
	// open are the folders open, by their path joined with /; trying
	// is the preset tried on the patch, and shown the row list drawn.
	open   map[string]bool
	trying string
	shown  []treeRow
	hot    int
	size   geom.Size
}

// treeRow is a row of the tree: a folder, at its path, or a preset.
type treeRow struct {
	depth  int
	label  string
	folder string
	id     string
	isOpen bool
}

const treeRowH = 21

func newPresetTree(drums bool) *presetTree {
	return &presetTree{drums: drums, open: map[string]bool{}, hot: -1}
}

// update shows the library for the patch name, from st.
func (pt *presetTree) update(st Studio, name string) {
	pt.patch = name
	pt.rows = pt.rows[:0]
	for _, r := range st.Presets {
		if r.Drums == pt.drums {
			pt.rows = append(pt.rows, r)
		}
	}
	id := st.Trying[name]
	if id != pt.trying && id != "" {
		// The folders of a preset tried open, so it shows.
		for _, r := range pt.rows {
			if r.ID == id {
				for i := range r.Path {
					pt.open[strings.Join(r.Path[:i+1], "/")] = true
				}
			}
		}
	}
	pt.trying = id
	pt.lay()
}

// lay lists the rows shown: each folder, and in it, where it is open,
// its folders and presets.
func (pt *presetTree) lay() {
	pt.shown = pt.shown[:0]
	seen := map[string]bool{}
	for _, r := range pt.rows {
		hidden := false
		for i := range r.Path {
			key := strings.Join(r.Path[:i+1], "/")
			if !seen[key] {
				seen[key] = true
				pt.shown = append(pt.shown, treeRow{depth: i, label: r.Path[i], folder: key, isOpen: pt.open[key]})
			}
			if !pt.open[key] {
				hidden = true
				break
			}
		}
		if !hidden {
			pt.shown = append(pt.shown, treeRow{depth: len(r.Path), label: r.Name, id: r.ID})
		}
	}
}

func (pt *presetTree) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	w := c.Max.W
	if w > 1e4 {
		w = 210
	}
	pt.size = c.Constrain(geom.Sz(w, float32(max(len(pt.shown), 1))*treeRowH+8))
	return pt.size
}

func (pt *presetTree) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	accent := widget.Accent.Get(f.Theme)
	for i, r := range pt.shown {
		y := 4 + float32(i)*treeRowH
		row := geom.Rc(2, y, box.W-4, treeRowH-2)
		switch {
		case r.id != "" && r.id == pt.trying:
			p.RRect(row, 5, paint.Solid(withAlpha(accent, 0.35)))
		case i == pt.hot:
			p.RRect(row, 5, paint.Solid(withAlpha(ink, 0.07)))
		}
		x := 8 + float32(r.depth)*12
		alpha := float32(0.75)
		if r.folder != "" {
			mark := "▸"
			if r.isOpen {
				mark = "▾"
			}
			m := audioui.Shaped(mark, 10, false, false)
			m.Paint(p, geom.Pt(x, y+(treeRowH-m.Height())/2-1), withAlpha(ink, 0.5))
			x += 13
			alpha = 0.9
		}
		t := audioui.Shaped(r.label, 11, r.folder != "" && r.depth == 0, false)
		t.Paint(p, geom.Pt(x, y+(treeRowH-t.Height())/2-1), withAlpha(ink, alpha))
	}
}

// at returns the row at y, or -1.
func (pt *presetTree) at(y float32) int {
	i := int((y - 4) / treeRowH)
	if y < 4 || i >= len(pt.shown) {
		return -1
	}
	return i
}

func (pt *presetTree) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter, input.PointerMove:
		pos := geom.Point{}
		if m, ok := e.(input.PointerMove); ok {
			pos = m.Pos
		} else {
			pos = e.(input.PointerEnter).Pos
		}
		if h := pt.at(pos.Y); h != pt.hot {
			pt.hot = h
			u.Invalidate()
		}
		return true
	case input.PointerLeave:
		pt.hot = -1
		u.Invalidate()
		return true
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := pt.at(e.Pos.Y)
		if i < 0 {
			return true
		}
		r := pt.shown[i]
		if r.folder != "" {
			pt.open[r.folder] = !pt.open[r.folder]
			pt.lay()
			u.Invalidate()
			return true
		}
		pt.trying = r.id
		u.Send(pt, PresetTry{Patch: pt.patch, ID: r.id})
		u.Invalidate()
		return true
	}
	return false
}

// folders returns the folders open, in order, for a test to read.
func (pt *presetTree) folders() []string {
	var out []string
	for k, v := range pt.open {
		if v {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}
