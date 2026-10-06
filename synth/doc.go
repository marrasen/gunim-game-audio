// Package synth is a music engine for games: songs made in code, played
// by synthesizers as they are heard, which grow and change with the
// game.
//
// A [Song] is plain data, as a song.json holds it: a tempo, a key, a
// chord progression, written or generated, the patches that sound, and
// the tracks that play them. Each track plays a pattern in TidalCycles'
// mini-notation over the chords: degrees of the scale, the chord's
// tones, the whole chord, an arpeggio, or drums by name, with patterns
// of parameters that change each note, as TidalCycles' controls do:
//
//	bd(3,8) [~ sn]*2      drums, Euclid's way
//	x*16                  an arpeggio over the chord
//	<0 2 4>*2 ch          degrees, and the chord voiced
//
// The patches are a subtractive synth of layered oscillators, saw,
// pulse, triangle, sine and FM, in unison, through a filter, with
// envelopes and LFOs, which can sing a vowel; a plucked string, as
// Karplus and Strong pluck one; and a drum machine made in code, from a
// kick to a timpani tuned to the chord, with risers, downs and impacts.
// A song's tracks send to a reverb and a delay in time with it, and run
// through effects of their own, a sidechain duck, a compressor and a
// limiter.
//
// A song plays in tiers, which a game climbs as its combo grows, a
// riser marking the climb, or wanders, its tracks coming and going by
// themselves. Its [Player] is a [band.Player], so a game plays it as it
// plays gunim's other songs; it plays notes on a keypad in key, plays
// stings, and takes edits as it plays, for a tool such as gunim music
// studio, which it tells what it plays, note by note.
package synth
