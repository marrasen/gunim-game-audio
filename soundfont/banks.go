package soundfont

import (
	"path"
	"strings"
)

// A Bank is a SoundFont bank that may be fetched, and what its licence
// allows. None is part of this module: a program fetches the one it
// chooses, or ships it beside itself, under its licence.
type Bank struct {
	// Name names it, File is the name to save it as, and URL and
	// LicenseURL where to fetch it and its licence. URL may name an
	// archive, a .7z, .tar.xz or .tar.bz2, that holds the bank, an .sf2,
	// and its notices, its licence and readme as .txt files; LicenseURL
	// then is where the licence may be read, as it is not fetched.
	Name, File, URL, LicenseURL string
	// License names its licence, and Notes says what to mind.
	License, Notes string
	// Size is how many bytes URL fetches, and Unpacked how many the
	// bank is, unpacked from its archive; 0 where URL is the bank.
	Size, Unpacked int64
	// SHA256 is the hex SHA-256 of what URL fetches, to check it by,
	// or "" to take it as it comes.
	SHA256 string
	// Group is the kind of instrument a bank of one plays, as "Piano";
	// "" for a General MIDI bank, of 128 instruments and its kits.
	Group string
	// Kit says the bank's instrument is a drum kit, or percussion, a
	// sound to a key.
	Kit bool
	// Level is how many decibels to turn the bank by for it to play
	// about as loud as the engine's own instruments, as measured on
	// them: a bank recorded hot, its peaks at full scale, goes down.
	Level float64
	// About says what was recorded, and Credit who made it, to show
	// where it plays, as its licence asks.
	About, Credit string
	// Page is where to read of it.
	Page string
}

// Archived reports whether URL fetches an archive that holds the bank,
// and not the bank itself.
func (b Bank) Archived() bool {
	return b.archive() != ""
}

// archive is the kind of archive URL fetches: "7z", "tar.xz",
// "tar.bz2", or "" for a bank itself.
func (b Bank) archive() string {
	name := strings.ToLower(path.Base(b.URL))
	for _, ext := range []string{"7z", "tar.xz", "tar.bz2"} {
		if strings.HasSuffix(name, "."+ext) {
			return ext
		}
	}
	return ""
}

// Key is a name for the bank that stays as it is from version to
// version, for a program to register it by and a song to name it by:
// its File without the extension.
func (b Bank) Key() string {
	return strings.TrimSuffix(b.File, path.Ext(b.File))
}

// Banks are some General MIDI banks free to use. MuseScore General is
// the one to choose to ship with a program: it is small, as it holds its
// samples compressed, and its MIT licence asks only that its notices go
// with it.
var Banks = []Bank{
	{
		Name: "MuseScore General", File: "MuseScore_General.sf3",
		URL:        "https://ftp.osuosl.org/pub/musescore/soundfont/MuseScore_General/MuseScore_General.sf3",
		LicenseURL: "https://ftp.osuosl.org/pub/musescore/soundfont/MuseScore_General/MuseScore_General_License.md",
		License:    "MIT", Size: 39900972,
		Notes:  "Based on FluidR3; the acknowledgements and copyright notices in its licence must go with it.",
		Credit: "MuseScore General: FluidR3 by Frank Wen, copyright 2000-02; FluidR3Mono by Michael Cowgill, copyright 2014-17; adapted by S. Christian Collins, copyright 2018-19; Temple Blocks by Ethan Winer, copyright 2002; Drumline Cymbals by Michael Schorsch, copyright 2016. MIT licence.",
		Page:   "https://musescore.org/en/handbook/3/soundfonts-and-sfz-files",
	},
	{
		Name: "GeneralUser GS", File: "GeneralUser-GS.sf2",
		URL:        "https://raw.githubusercontent.com/mrbumpy409/GeneralUser-GS/main/GeneralUser-GS.sf2",
		LicenseURL: "https://raw.githubusercontent.com/mrbumpy409/GeneralUser-GS/main/documentation/LICENSE.txt",
		License:    "GeneralUser GS License v2.0", Size: 32319396,
		Notes:  "Free for any use, commercial too; its author cannot vouch for where every sample came from.",
		Credit: "GeneralUser GS, by S. Christian Collins.",
		Page:   "https://schristiancollins.com/generaluser.php",
	},
}

// The licences of the Instruments.
const (
	cc0      = "CC0 1.0"
	cc0URL   = "https://creativecommons.org/publicdomain/zero/1.0/"
	ccBy3    = "CC BY 3.0"
	ccBy3URL = "https://creativecommons.org/licenses/by/3.0/"
	ccBy4    = "CC BY 4.0"
	ccBy4URL = "https://creativecommons.org/licenses/by/4.0/"
	gplX     = "GPLv3+ with the FreePats exception"
	gplXURL  = "https://freepats.zenvoid.org/licenses.html"
	// gplXNotes is what the FreePats exception allows.
	gplXNotes = "Music made with it is not bound by the GPL; the bank itself is."
	// freepats is the site the Instruments come from.
	freepats = "https://freepats.zenvoid.org/"
)

// Instruments are banks of one instrument each, recorded, free to use,
// from the FreePats project: a piano, a drum kit, a guitar, a harp. Each
// is fetched as an archive, which holds its licence. Those under CC BY
// ask that their Credit go where they play.
var Instruments = []Bank{
	{
		Name: "Salamander Grand Piano", File: "SalamanderGrandPiano.sf2", Group: "Piano",
		Level:  -3,
		URL:    "https://freepats.zenvoid.org/Piano/SalamanderGrandPiano/SalamanderGrandPiano-SF2-V3+20200602.tar.xz",
		SHA256: "15edb061d7ba60d58332f72dba8f8ce40988048cc703f935e6320f37d650e213",
		Size:   310397984, Unpacked: 1266462334,
		License: ccBy3, LicenseURL: ccBy3URL,
		About:  "A Yamaha C5 concert grand, recorded in 16 velocity layers.",
		Credit: "Salamander Grand Piano, by Alexander Holm; SF2 by Roberto for FreePats. CC BY 3.0.",
		Notes:  "The largest of all: 1.3 GB on disk, though only the notes played are read into memory.",
		Page:   freepats + "Piano/acoustic-grand-piano.html",
	},
	{
		Name: "YDP Grand Piano", File: "YDP-GrandPiano.sf2", Group: "Piano",
		Level:  -6.5,
		URL:    "https://freepats.zenvoid.org/Piano/YDP-GrandPiano/YDP-GrandPiano-SF2-20160804.tar.bz2",
		SHA256: "d243dc3e182a60df2a16e92828c1821cf3eb5748b45e2e2bdcfa9cf7af056026",
		Size:   36737684, Unpacked: 118398836,
		License: ccBy3, LicenseURL: ccBy3URL,
		About:  "A Yamaha Disklavier Pro grand, recorded by Zenph Studios for OLPC.",
		Credit: "Zenph Studios Yamaha Disklavier Pro Piano Multisamples for OLPC; SF2 by Roberto for FreePats. CC BY 3.0.",
		Page:   freepats + "Piano/acoustic-grand-piano.html",
	},
	{
		Name: "Upright Piano KW", File: "UprightPianoKW.sf2", Group: "Piano",
		Level:  -7.5,
		URL:    "https://freepats.zenvoid.org/Piano/UprightPianoKW/UprightPianoKW-SF2-20220221.7z",
		SHA256: "17c084c6e4205233dc49b34e4bc44a9b2d7c7a2c02b04729ecda77079b07c826",
		Size:   28832466, Unpacked: 57377848,
		License: cc0, LicenseURL: cc0URL,
		About:  "A Kawai upright in a living room, recorded in two velocity layers.",
		Credit: "Upright Piano KW, recorded by Gonzalo and Roberto for FreePats. CC0.",
		Page:   freepats + "Piano/acoustic-grand-piano.html",
	},
	{
		Name: "Honky-tonk Piano FB", File: "PianoFB.sf2", Group: "Piano",
		Level:  -7.5,
		URL:    "https://github.com/freepats/old-piano-FB/releases/download/2020-04-01/PianoFB-SF2-20200401.7z",
		SHA256: "1f2dea033f3cfef6f14e4f633a687fcc847e34bc7a452ba38891a2a1a7274f91",
		Size:   55558153, Unpacked: 106304890,
		License: cc0, LicenseURL: cc0URL,
		About:  "An old Francis Bacon player piano, with an honest honky-tonk tone.",
		Credit: "Piano FB, recorded by Piotr Barcz for FreePats. CC0.",
		Page:   freepats + "Piano/honky-tonk-piano.html",
	},
	{
		Name: "MuldjordKit", File: "MuldjordKit.sf2", Group: "Drums", Kit: true,
		Level:  -11,
		URL:    "https://github.com/freepats/muldjordkit/releases/download/2020-10-18/MuldjordKit-SF2-20201018.7z",
		SHA256: "aef4c9e0a96f9a899a7838f65ff07565f6af247bbb4ff31e24dba3331a671a96",
		Size:   54877940, Unpacked: 219362048,
		License: ccBy4, LicenseURL: ccBy4URL,
		About:  "A rock kit: two kicks, a snare, four toms, a hi-hat and five cymbals, on keys 48 to 66.",
		Credit: "MuldjordKit, by Lars Muldjord; SF2 by Roberto for FreePats. CC BY 4.0.",
		Page:   freepats + "Percussion/acoustic-drum-kit.html",
	},
	{
		Name: "World Percussion", File: "WorldPercussion.sf2", Group: "Drums", Kit: true,
		Level:  -4.5,
		URL:    "https://github.com/freepats/world-percussion/releases/download/2020-09-05/WorldPercussion-SF2-20200905.7z",
		SHA256: "5d72c35572259804091657e094fb094c4ee45ffd4bc37f1708878ef6cfeeb129",
		Size:   5100075, Unpacked: 9256408,
		License: cc0, LicenseURL: cc0URL,
		About:  "Cajón, bongos, congas, darbuka, shakers, tambourine, claves, castanets and maracas, on keys 48 to 71.",
		Credit: "World Percussion, recorded by Xavimart, Gonzalo and Roberto for FreePats, its claves and conga from the Versilian Community Sample Library. CC0.",
		Page:   freepats + "Percussion/world-and-rare-percussion.html",
	},
	{
		Name: "Timpani", File: "Timpani.sf2", Group: "Orchestral percussion",
		Level:  -7.5,
		URL:    "https://github.com/freepats/timpani/releases/download/2024-08-10/Timpani-SF2-20240810.7z",
		SHA256: "6a8dff96d9ad109c7ba6155b11f2c2dcc635850cdd43fc1db460f80c45a8e066",
		Size:   4723477, Unpacked: 22569396,
		License: cc0, LicenseURL: cc0URL,
		About:  "Orchestral timpani, tuned to the keys.",
		Credit: "Timpani, from the Versilian Community Sample Library; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "Percussion/orchestral-percussion.html",
	},
	{
		Name: "Tubular Bells", File: "TubularBells.sf2", Group: "Orchestral percussion",
		Level:  -4.5,
		URL:    "https://github.com/freepats/tubular-bells1/releases/download/2024-11-30/TubularBells-SF2-20241130.7z",
		SHA256: "e62e827ccfc11d843d91d6d54048627c02dbbc5ae56e2cdc1cb8dc5b765ea337",
		Size:   9677397, Unpacked: 18643170,
		License: cc0, LicenseURL: cc0URL,
		About:  "Orchestral chimes.",
		Credit: "Tubular Bells, from the Versilian Community Sample Library; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "ChromaticPercussion/tubular-bells.html",
	},
	{
		Name: "Xylophone", File: "Xylophone.sf2", Group: "Orchestral percussion",
		Level:  -7,
		URL:    "https://freepats.zenvoid.org/ChromaticPercussion/Xylophone1/Xylophone-MediumMallets-SF2-20200706.tar.xz",
		SHA256: "b142f2a25f0610f13d1d657c60ae3f28be58cf026d216826125c6fb9d1af631b",
		Size:   1756892, Unpacked: 4244686,
		License: cc0, LicenseURL: cc0URL,
		About:  "A xylophone, played with medium mallets.",
		Credit: "Xylophone, from the Versilian Community Sample Library; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "ChromaticPercussion/xylophone.html",
	},
	{
		Name: "Spanish Classical Guitar", File: "SpanishClassicalGuitar.sf2", Group: "Guitar",
		Level:  -4.5,
		URL:    "https://freepats.zenvoid.org/Guitar/SpanishClassicalGuitar/SpanishClassicalGuitar-SF2-20190618.7z",
		SHA256: "8c192f1fc640a553199a4a43a17ae12354a6a05e3d6aad79bd43b5d29314ff51",
		Size:   9990750, Unpacked: 19822546,
		License: cc0, LicenseURL: cc0URL,
		About:  "A nylon-string Spanish guitar.",
		Credit: "Spanish Classical Guitar, recorded by Roberto for FreePats. CC0.",
		Page:   freepats + "Guitar/acoustic-guitar.html",
	},
	{
		Name: "Steel-String Guitar", File: "FSS-SteelStringGuitar.sf2", Group: "Guitar",
		Level:  -5,
		URL:    "https://freepats.zenvoid.org/Guitar/FSS-SteelStringGuitar/FSS-SteelStringGuitar-SF2-20200521.tar.xz",
		SHA256: "16ce465c5863aa04f209b52db399c2b7192a96e7509ab32ed547d403b59f48f1",
		Size:   12965944, Unpacked: 26366094,
		License: gplX, LicenseURL: gplXURL, Notes: gplXNotes,
		About:  "A Seagull steel-string acoustic guitar.",
		Credit: "FS Seagull Steel String Acoustic Guitar, copyright 2008 Gary Campion (FlameStudios), modified 2016-2020 by Roberto for FreePats. GPLv3 or later, with the FreePats exception.",
		Page:   freepats + "Guitar/steel-acoustic-guitar.html",
	},
	{
		Name: "Clean Electric Guitar", File: "EGuitarFSBS-clean.sf2", Group: "Guitar",
		Level:  -7.5,
		URL:    "https://github.com/freepats/electric-guitar-FSBS-clean/releases/download/2026-08-07/EGuitarFSBS-clean-SF2-20260807.7z",
		SHA256: "6d338afefe0efb629afd9ba28a5c8c129fc28b97ef1b57a31ebceb7bf0e3bf65",
		Size:   135571190, Unpacked: 445922788,
		License: cc0, LicenseURL: cc0URL,
		About:  "A Fender electric guitar on its bridge pickup, through a clean amp.",
		Credit: "Electric Guitar FSBS (clean), by FreePats. CC0.",
		Page:   freepats + "ElectricGuitar/clean-electric-guitar.html",
	},
	{
		Name: "Finger Bass YR", File: "FingerBassYR.sf2", Group: "Bass",
		Level:  -4.5,
		URL:    "https://github.com/freepats/electric-bass-YR/releases/download/2019-09-30/FingerBassYR-SF2-20190930.7z",
		SHA256: "f5b5cfe04435a9e9037d1f97d515dc596535acf26637db9579d724cd757f179e",
		Size:   2613000, Unpacked: 4220300,
		License: cc0, LicenseURL: cc0URL,
		About:  "A Yamaha RBX electric bass, played with the fingers.",
		Credit: "Finger Bass YR, recorded by Andrea Biasior for FreePats. CC0.",
		Page:   freepats + "ElectricGuitar/clean-electric-bass.html",
	},
	{
		Name: "Concert Harp", File: "ConcertHarp.sf2", Group: "Strings",
		Level:  -4.5,
		URL:    "https://freepats.zenvoid.org/OrchestralStrings/ConcertHarp/ConcertHarp-SF2-20200702.tar.xz",
		SHA256: "a328faa1996369154f20f960dde2e255b96e823574d5cf80eb9e7f00aed6a150",
		Size:   9513292, Unpacked: 18810604,
		License: cc0, LicenseURL: cc0URL,
		About:  "A concert harp, in two velocity layers.",
		Credit: "Concert Harp, from the Versilian Community Sample Library; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "OrchestralStrings/harp.html",
	},
	{
		Name: "Clarinet", File: "Clarinet.sf2", Group: "Winds",
		Level:  -9,
		URL:    "https://freepats.zenvoid.org/Reed/Clarinet1/Clarinet-SF2-20190818.tar.xz",
		SHA256: "3f06aa4747cf52fb595d5778b147070a2a0213bcea331005e319d8b11d606a5e",
		Size:   6965072, Unpacked: 7394048,
		License: cc0, LicenseURL: cc0URL,
		About:  "A B-flat clarinet.",
		Credit: "Clarinet, recorded by Tyler and Kaili Dence; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "Reed/clarinet.html",
	},
	{
		Name: "Tenor Saxophone", File: "TenorSaxophone.sf2", Group: "Winds",
		Level:  -6,
		URL:    "https://freepats.zenvoid.org/Reed/TenorSaxophone/TenorSaxophone-SF2-20200717.tar.bz2",
		SHA256: "f7640cf2039662b15b4521bb5615e1ddc426731559b4c31079cb5e9f50e16fa0",
		Size:   25757059, Unpacked: 28117404,
		License: cc0, LicenseURL: cc0URL,
		About:  "A tenor saxophone, its notes held as long as the key is.",
		Credit: "Tenor Saxophone, from the Versilian Community Sample Library; bank by Roberto for FreePats. CC0.",
		Page:   freepats + "Reed/saxophone.html",
	},
	{
		Name: "Recorder", File: "Recorder.sf2", Group: "Winds",
		Level:  -8.5,
		URL:    "https://freepats.zenvoid.org/Wind/Recorder1/Recorder-SF2-20201205.7z",
		SHA256: "60258c60fcef9e398aca22749cdbeaba792b8fa7c9115e63ceef3aff5d4bd1c0",
		Size:   8289356, Unpacked: 8554576,
		License: cc0, LicenseURL: cc0URL,
		About:  "A wooden recorder.",
		Credit: "Recorder, played and recorded by Eugene Vlaskin for FreePats. CC0.",
		Page:   freepats + "Wind/recorder.html",
	},
	{
		Name: "Church Organ", File: "ChurchOrganEmulation.sf2", Group: "Organ",
		Level:  -6,
		URL:    "https://freepats.zenvoid.org/Organ/ChurchOrganEmulation/ChurchOrganEmulation-SF2-20190924.tar.xz",
		SHA256: "ed7f9a144fbe943cf3bcb251055da00e5a667d4a70fa471c42f0624027083c30",
		Size:   12895300, Unpacked: 14455484,
		License: cc0, LicenseURL: cc0URL,
		About:  "A pipe organ, recorded from the Aeolus organ synthesizer rather than a real organ.",
		Credit: "Church Organ Emulation, made with Aeolus by Roberto for FreePats. CC0.",
		Page:   freepats + "Organ/pipe-organ.html",
	},
}
