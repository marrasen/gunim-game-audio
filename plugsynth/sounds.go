package plugsynth

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

// The banks the sounds come from.
const (
	// Library is the patch library's presets, by its categories, as
	// Pads & strings.
	Library = "Library"
	// GeneralMIDI is General MIDI's 128 instruments, by its sixteen
	// families, as Piano or Brass, and its drum kits.
	GeneralMIDI = "General MIDI"
	// Songs is the patches the library's songs play, a group for each
	// song.
	Songs = "Songs"
)

// Banks are the module's own banks, in the order Presets lists them;
// the banks UseFont was handed stand between General MIDI and Songs.
var Banks = []string{Library, GeneralMIDI, Songs}

// A Sound is a sound the plugin offers: a patch, and where it comes
// from.
type Sound struct {
	// Bank is the bank it comes from, and Group the group it is filed
	// under there: a category of the library, a family of General MIDI,
	// or a song's title.
	Bank, Group, Name string
	// About says what it sounds like.
	About string
	patch *synth.Patch
	// drums says it is a kit, and pitch is the note a preview of it
	// plays, where it is not 0.
	drums bool
	pitch float64
}

// Patch returns the sound's patch, the caller's own to change.
func (s Sound) Patch() *synth.Patch { return clonePatch(s.patch) }

// State returns the plugin's state that plays s, for [plug.Plugin]'s
// SetState: the sound's name, where it comes from, and its whole patch.
func (s Sound) State() []byte {
	b := State{Bank: s.Bank, Group: s.Group, Name: s.Name, Patch: s.patch}.Encode()
	// The module's own sounds play: their states need no check.
	checked.add(b)
	return b
}

// gmFamilies are General MIDI's families of instruments, eight programs
// each, in order.
var gmFamilies = []string{
	"Piano", "Chromatic percussion", "Organ", "Guitar",
	"Bass", "Strings", "Ensemble", "Brass",
	"Reed", "Pipe", "Synth lead", "Synth pad",
	"Synth effects", "Ethnic", "Percussive", "Sound effects",
}

// GMDrums is the group of General MIDI's drum kits.
const GMDrums = "Drum kits"

// gmKits are the drum kits [synth.GMKit] plays, by the drum channel's
// program.
var gmKits = []struct {
	program     int
	name, about string
}{
	{0, "Standard kit", "General MIDI's drum kit: each drum on its note, a kick on 36, a snare on 38, hats on 42 and 46."},
	{24, "Electronic kit", "The standard kit with syn-toms and a bright snare, as Roland's Electronic kit."},
	{25, "TR-808 kit", "The standard kit with a long, booming kick and ringing toms, as Roland's TR-808 kit."},
}

// Sounds returns every sound the plugin offers, bank by bank, each
// bank group by group, in the order Presets lists them: the module's
// own, and those of the banks UseFont was handed. The first call reads
// the module's own; later calls share what it read, which they leave as
// it is.
func Sounds() []Sound { return slices.Clone(all()) }

// ownSounds reads the module's own sounds once.
var ownSounds = sync.OnceValue(func() []Sound {
	out, err := readSounds()
	if err != nil {
		panic("plugsynth: " + err.Error())
	}
	return out
})

// readSounds reads the sounds: the library's presets, General MIDI's
// instruments and kits, and the songs' patches.
func readSounds() ([]Sound, error) {
	presets, err := music.Presets()
	if err != nil {
		return nil, err
	}
	out := make([]Sound, 0, len(presets)+128+len(gmKits)+256)
	for _, p := range presets {
		out = append(out, Sound{Bank: Library, Group: p.Category, Name: p.Name, About: p.About, patch: p.Patch})
	}
	for prog, name := range &synth.GMNames {
		out = append(out, Sound{Bank: GeneralMIDI, Group: gmFamilies[prog/8], Name: name,
			About: fmt.Sprintf("General MIDI's program %d, played on the engine's own instruments.", prog+1),
			patch: synth.GMPatch(prog)})
	}
	for _, k := range gmKits {
		out = append(out, Sound{Bank: GeneralMIDI, Group: GMDrums, Name: k.name, About: k.about, patch: synth.GMKit(k.program)})
	}
	for _, name := range music.Songs() {
		b, err := music.Song(name)
		if err != nil {
			return nil, err
		}
		s, ok := b.(*synth.Song)
		if !ok {
			// A song of recorded parts plays no patches.
			continue
		}
		names := make([]string, 0, len(s.Patches))
		for n := range s.Patches {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			pt, about := s.Patches[n], fmt.Sprintf("The %s %s plays.", n, s.Title)
			// A track that changes its vowel note by note sings its
			// vowels in turn here, as in the song.
			if v := s.Vowels(n); v != "" && pt.Vowels == "" {
				pt = clonePatch(pt)
				pt.Vowels = v
				about += fmt.Sprintf(" Its notes sing %s in turn, as the song's do.", v)
			}
			out = append(out, Sound{Bank: Songs, Group: s.Title, Name: n, About: about, patch: pt})
		}
	}
	return out, nil
}

// Find returns the sound named name in group of bank, and whether there
// is one.
func Find(bank, group, name string) (Sound, bool) {
	for _, s := range all() {
		if s.Bank == bank && s.Group == group && s.Name == name {
			return s, true
		}
	}
	return Sound{}, false
}

// Named returns the first sound named name, in Sounds' order, and
// whether there is one: a name of the library, as "Juno pad", or of
// General MIDI, as "Acoustic Grand Piano", names one sound only.
func Named(name string) (Sound, bool) {
	for _, s := range all() {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Sound{}, false
}

// Groups returns the groups of bank, in order.
func Groups(bank string) []string {
	var out []string
	for _, s := range all() {
		if s.Bank == bank && (len(out) == 0 || out[len(out)-1] != s.Group) {
			out = append(out, s.Group)
		}
	}
	return out
}
