package main

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim-game-audio/soundfont"
)

// A bank without a Level gets one, a bank with one has it replaced, and the rest stay as they are.
func TestSetLevelsSetsOnlyTheBanksNamed(t *testing.T) {
	src := `package x

var Banks = []Bank{
	{
		Name: "New", File: "new.sf2",
		URL: "u",
	},
	{
		Name: "Old", File: "old.sf2",
		Level: -3,
		URL:   "u",
	},
	{
		Name: "Kit", File: "kit.sf2",
		Level: -11,
	},
}
`
	got, err := setLevels([]byte(src), map[string]float64{"New": -3.5, "Old": -7})
	if err != nil {
		t.Fatal(err)
	}
	want := `package x

var Banks = []Bank{
	{
		Name: "New", File: "new.sf2",
		Level: -3.5,
		URL:   "u",
	},
	{
		Name: "Old", File: "old.sf2",
		Level: -7,
		URL:   "u",
	},
	{
		Name: "Kit", File: "kit.sf2",
		Level: -11,
	},
}
`
	if string(got) != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestSetLevelsRefusesABankItCannotFind(t *testing.T) {
	_, err := setLevels([]byte("package x\n"), map[string]float64{"Gone": -1})
	if err == nil || !strings.Contains(err.Error(), "Gone") {
		t.Fatalf("got %v", err)
	}
}

// Only a recorded instrument's peak can hold it under the target.
func TestWantLevel(t *testing.T) {
	gm, one := soundfont.Bank{}, soundfont.Bank{Group: "Piano"}
	for _, c := range []struct {
		b           soundfont.Bank
		median, top float64
		want        float64
	}{
		{gm, -6.2, 14.6, -7},
		{one, -5.8, 8.2, -7},
		{one, -1.6, 14.5, -13},
	} {
		if got := wantLevel(c.b, c.median, c.top); got != c.want {
			t.Errorf("wantLevel(%q, %v, %v) = %v, want %v", c.b.Group, c.median, c.top, got, c.want)
		}
	}
}
