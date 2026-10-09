package songpicker

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
)

func TestItemsGroupTheSongsByCategory(t *testing.T) {
	titles := []string{"No music", "Star Drift", "Boss Entrance", "Candy Clouds", "Alien Entrance"}
	categories := []string{"", "Calm rooms", "Boss fights", "Calm rooms", "Boss fights"}
	items, paths := Items(titles, categories)
	got := make([]string, 0, len(items))
	for _, it := range items {
		parts := []string{it.Label}
		if it.Sub != nil {
			for _, sub := range it.Sub.Items {
				parts = append(parts, sub.Label)
			}
		}
		got = append(got, strings.Join(parts, "/"))
	}
	want := []string{"No music", "Boss fights/Boss Entrance/Alien Entrance", "Calm rooms/Star Drift/Candy Clouds"}
	if !slices.Equal(got, want) {
		t.Fatalf("the items are %v, want %v", got, want)
	}
	if want := "[[0] [2 0] [1 0] [2 1] [1 1]]"; fmt.Sprint(paths) != want {
		t.Fatalf("the paths are %v, want %s", paths, want)
	}
}

func TestAPickerTellsTheSongByItsIndex(t *testing.T) {
	var picked []int
	p := New([]string{"Greek Themes", "Star Drift", "Boss Entrance"}, []string{"", "Calm rooms", "Boss fights"},
		func(i int, _ *gunim.UI) gunim.Intent {
			picked = append(picked, i)
			return nil
		})
	if p.Song() != 0 {
		t.Fatalf("a new picker chose song %d, want 0", p.Song())
	}
	p.Choose(1, nil)
	if p.Song() != 1 || p.Access().Value != "Star Drift" {
		t.Fatalf("Choose(1) chose song %d, showing %q", p.Song(), p.Access().Value)
	}
	// The drop-down's callbacks give the song's index.
	p.OnChangeSub([]int{1, 0}, nil)
	p.OnChange(0, nil)
	if !slices.Equal(picked, []int{2, 0}) {
		t.Fatalf("picks of Boss Entrance and Greek Themes told %v, want [2 0]", picked)
	}
	// New songs keep the one chosen by its index.
	p.SetSongs([]string{"Greek Themes", "Star Drift", "Boss Entrance"}, []string{"", "Boss fights", "Boss fights"})
	if p.Song() != 1 || p.Access().Value != "Star Drift" {
		t.Fatalf("after new categories the picker chose song %d, showing %q", p.Song(), p.Access().Value)
	}
}
