package synth

import "testing"

func TestAnExclusiveClassCutsOffOnlyItsOwnPart(t *testing.T) {
	f := testFont(t, "part-test")
	rs := f.AppendRegions(nil, 0, 0, 60, 100)
	if len(rs) == 0 {
		t.Fatal("the test bank plays no region for key 60")
	}
	r := rs[0]
	r.Exclusive = 1
	vv := newSFVoices(4)
	vv.part = true
	key := vv.play(&r, 60, 100, 0)
	key.livePart = true
	song := vv.play(&r, 60, 100, 0)
	if !key.on || key.released >= 0 {
		t.Error("a song's note of the class cut off the key the player holds")
	}
	vv.play(&r, 60, 100, 0)
	if song.released < 0 {
		t.Error("a song's note of the class left the song's note before it sounding")
	}
}

func TestABusySongLetsGoOfNoHeldKey(t *testing.T) {
	testFont(t, "busy-test")
	in, err := NewInstrument(&Patch{SoundFont: "busy-test"})
	if err != nil {
		t.Fatal(err)
	}
	in.LiveNoteOn(1, 60, 1, 0)
	for i := range maxLive {
		in.NoteOn(int32(i+1), 60, 1, 0)
	}
	if in.find(1, true) < 0 {
		t.Error("the song's notes, past the most held, let go of the key the player holds")
	}
}
