// Package songpicker is a drop-down of songs grouped by category: each
// category opens a submenu of its songs, and a song of no category shows
// in the list itself, above the categories.
package songpicker

import (
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/widget"
)

// Picker is a drop-down of songs grouped by category. Songs are counted
// by their index in the list New and SetSongs take.
type Picker struct {
	*widget.Dropdown
	// titles and categories are the songs as last set, and paths each
	// song's path in the drop-down.
	titles, categories []string
	paths              [][]int
}

// New returns a picker of the songs titled titles, each in the category
// of the same index, "" for none. A pick of a song runs onPick with its
// index, as the drop-down's OnChange runs.
func New(titles, categories []string, onPick func(i int, u *gunim.UI) gunim.Intent) *Picker {
	p := &Picker{Dropdown: widget.NewDropdown(nil)}
	p.SetSongs(titles, categories)
	p.OnChange = func(i int, u *gunim.UI) gunim.Intent { return p.picked([]int{i}, u, onPick) }
	p.OnChangeSub = func(path []int, u *gunim.UI) gunim.Intent { return p.picked(path, u, onPick) }
	return p
}

// picked runs onPick with the song at path.
func (p *Picker) picked(path []int, u *gunim.UI, onPick func(i int, u *gunim.UI) gunim.Intent) gunim.Intent {
	i := slices.IndexFunc(p.paths, func(q []int) bool { return slices.Equal(q, path) })
	if i < 0 || onPick == nil {
		return nil
	}
	return onPick(i, u)
}

// SetSongs gives the picker its songs, where they differ from the ones
// it has, keeping the song chosen by its index.
func (p *Picker) SetSongs(titles, categories []string) {
	if slices.Equal(titles, p.titles) && slices.Equal(categories, p.categories) {
		return
	}
	chosen := p.Song()
	p.titles, p.categories = slices.Clone(titles), slices.Clone(categories)
	items, paths := Items(titles, categories)
	p.paths = paths
	p.SetItems(items)
	p.Choose(chosen, nil)
}

// Song returns the index of the song chosen, or -1 for none.
func (p *Picker) Song() int {
	path := p.SelectedPath()
	return slices.IndexFunc(p.paths, func(q []int) bool { return slices.Equal(q, path) })
}

// Choose shows song i as the one chosen and sends no intent; see
// [widget.Dropdown.SetSelectedPath].
func (p *Picker) Choose(i int, u *gunim.UI) {
	if i >= 0 && i < len(p.paths) {
		p.SetSelectedPath(p.paths[i], u)
	}
}

// Items returns the drop-down's items for the songs titled titles, each
// in the category of the same index, and each song's path among them.
// The songs of no category come first, in their order, then the
// categories by name, each a submenu of its songs in their order.
func Items(titles, categories []string) (items []widget.MenuItem, paths [][]int) {
	paths = make([][]int, len(titles))
	var names []string
	for i, t := range titles {
		c := ""
		if i < len(categories) {
			c = categories[i]
		}
		if c == "" {
			paths[i] = []int{len(items)}
			items = append(items, widget.MenuItem{Label: t})
		} else if !slices.Contains(names, c) {
			names = append(names, c)
		}
	}
	slices.Sort(names)
	for _, c := range names {
		sub := &widget.Submenu{}
		for i, t := range titles {
			if i < len(categories) && categories[i] == c {
				paths[i] = []int{len(items), len(sub.Items)}
				sub.Items = append(sub.Items, widget.MenuItem{Label: t})
			}
		}
		items = append(items, widget.MenuItem{Label: c, Sub: sub})
	}
	return items, paths
}
