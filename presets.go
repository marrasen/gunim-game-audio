package music

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"

	"github.com/marrasen/gunim-game-audio/synth"
)

// patches holds the patch library: library.json, which names the
// presets and files them under categories, and a .patch.json a preset.
//
//go:embed patches
var patches embed.FS

// A Preset is a patch of the library, ready to try in a song: its name,
// the category it is filed under, as Bass or Drum kits, what it sounds
// like, and the patch.
type Preset struct {
	Name, Category, About string
	Patch                 *synth.Patch
}

// library is patches/library.json.
type library struct {
	Categories []struct {
		Name    string
		Presets []struct{ Name, File, About string }
	}
}

// Presets returns the patch library's presets, category by category, in
// order. Each call reads them afresh, so a preset changed is the
// caller's own.
func Presets() ([]Preset, error) {
	b, err := patches.ReadFile("patches/library.json")
	if err != nil {
		return nil, err
	}
	var lib library
	if err := json.Unmarshal(b, &lib); err != nil {
		return nil, fmt.Errorf("music: patches/library.json: %w", err)
	}
	var out []Preset
	for _, c := range lib.Categories {
		for _, p := range c.Presets {
			b, err := patches.ReadFile(path.Join("patches", p.File))
			if err != nil {
				return nil, fmt.Errorf("music: preset %s: %w", p.Name, err)
			}
			patch := new(synth.Patch)
			if err := json.Unmarshal(b, patch); err != nil {
				return nil, fmt.Errorf("music: preset %s: %w", p.Name, err)
			}
			out = append(out, Preset{Name: p.Name, Category: c.Name, About: p.About, Patch: patch})
		}
	}
	return out, nil
}
