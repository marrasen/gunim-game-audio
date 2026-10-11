package soundfont

// A Bank is a SoundFont bank that may be fetched, and what its licence
// allows. None is part of this module: a program fetches the one it
// chooses, or ships it beside itself, under its licence.
type Bank struct {
	// Name names it, File is the name to save it as, and URL and
	// LicenseURL where to fetch it and its licence.
	Name, File, URL, LicenseURL string
	// License names its licence, and Notes says what to mind.
	License, Notes string
	// Size is how many bytes it is.
	Size int64
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
		Notes: "Based on FluidR3; the acknowledgements and copyright notices in its licence must go with it.",
	},
	{
		Name: "GeneralUser GS", File: "GeneralUser-GS.sf2",
		URL:        "https://raw.githubusercontent.com/mrbumpy409/GeneralUser-GS/main/GeneralUser-GS.sf2",
		LicenseURL: "https://raw.githubusercontent.com/mrbumpy409/GeneralUser-GS/main/documentation/LICENSE.txt",
		License:    "GeneralUser GS License v2.0", Size: 32319396,
		Notes: "Free for any use, commercial too; its author cannot vouch for where every sample came from.",
	},
}
