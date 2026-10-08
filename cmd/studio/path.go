package main

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/marrasen/gunim-game-audio/synth"
)

// A path names a value of a song, its fields, keys and items split by
// slashes, as Patches/lead/Osc/0/Detune, Mix/Reverb/Size or
// Tracks/pad/Gain: a track, and anything else with a Name, may go by its
// name where an index would. Every control of the editors is bound to
// one, so the studio reads and sets every value the same way.

// errNoPath is returned for a path that leads nowhere.
var errNoPath = errors.New("studio: no such value")

// getPath returns the value at path in song: a number, a string or a
// bool, and false where there is none.
func getPath(song *synth.Song, path string) (any, bool) {
	v := reflect.ValueOf(song)
	for _, seg := range strings.Split(path, "/") {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil, false
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			v = v.FieldByName(seg)
		case reflect.Map:
			v = v.MapIndex(reflect.ValueOf(seg))
		case reflect.Slice:
			i, ok := item(v, seg)
			if !ok {
				return nil, false
			}
			v = v.Index(i)
		default:
			return nil, false
		}
		if !v.IsValid() {
			return nil, false
		}
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	return v.Interface(), true
}

// num returns the number at path in song, 0 where there is none.
func num(song *synth.Song, path string) float64 {
	x, ok := getPath(song, path)
	if !ok {
		return 0
	}
	switch x := x.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case uint64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

// str returns the string at path in song, "" where there is none.
func str(song *synth.Song, path string) string {
	x, _ := getPath(song, path)
	s, _ := x.(string)
	return s
}

// item returns the index seg names in the slice v: a number, or the
// name of an item with a Name.
func item(v reflect.Value, seg string) (int, bool) {
	if i, err := strconv.Atoi(seg); err == nil {
		return i, i >= 0 && i < v.Len()
	}
	for i := range v.Len() {
		e := v.Index(i)
		for e.Kind() == reflect.Pointer {
			if e.IsNil() {
				break
			}
			e = e.Elem()
		}
		if e.Kind() == reflect.Struct {
			if n := e.FieldByName("Name"); n.IsValid() && n.Kind() == reflect.String && n.String() == seg {
				return i, true
			}
		}
	}
	return 0, false
}

// setPath sets the value at path in song to x, a float64, a string or a
// bool, making what leads to it where it is missing: a map's entry, or
// a struct a nil pointer would hold.
func setPath(song *synth.Song, path string, x any) error {
	return set(reflect.ValueOf(song), strings.Split(path, "/"), x)
}

func set(v reflect.Value, segs []string, x any) error {
	if x == nil && len(segs) == 0 {
		// Nothing clears the value, as a nil pointer or an empty list.
		if !v.CanSet() {
			return errNoPath
		}
		v.Set(reflect.Zero(v.Type()))
		return nil
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if !v.CanSet() {
				return errNoPath
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if len(segs) == 0 {
		return assign(v, x)
	}
	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName(segs[0])
		if !f.IsValid() {
			return fmt.Errorf("%w: %s", errNoPath, segs[0])
		}
		return set(f, segs[1:], x)
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		key := reflect.ValueOf(segs[0])
		// A map's entry is copied out, set, and put back.
		e := reflect.New(v.Type().Elem()).Elem()
		if old := v.MapIndex(key); old.IsValid() {
			e.Set(old)
		}
		if err := set(e, segs[1:], x); err != nil {
			return err
		}
		v.SetMapIndex(key, e)
		return nil
	case reflect.Slice:
		i, ok := item(v, segs[0])
		if !ok {
			return fmt.Errorf("%w: %s", errNoPath, segs[0])
		}
		return set(v.Index(i), segs[1:], x)
	default:
	}
	return errNoPath
}

// assign sets v, a number, a string or a bool, to x.
func assign(v reflect.Value, x any) error {
	switch x := x.(type) {
	case float64:
		switch v.Kind() {
		case reflect.Float64, reflect.Float32:
			v.SetFloat(x)
		case reflect.Int, reflect.Int64:
			v.SetInt(int64(x + 0.5*sign(x)))
		case reflect.Uint64:
			v.SetUint(uint64(max(x, 0)))
		case reflect.Bool:
			v.SetBool(x != 0)
		default:
			return errNoPath
		}
	case string:
		if v.Kind() != reflect.String {
			return errNoPath
		}
		v.SetString(x)
	case bool:
		if v.Kind() != reflect.Bool {
			return errNoPath
		}
		v.SetBool(x)
	case []int:
		if v.Type() != reflect.TypeOf(x) {
			return errNoPath
		}
		v.Set(reflect.ValueOf(x))
	default:
		return errNoPath
	}
	return nil
}

func sign(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}

// addItem appends a new item to the slice at path: an oscillator or an
// LFO as a patch starts one.
func addItem(song *synth.Song, path string) error {
	segs := strings.Split(path, "/")
	return grow(reflect.ValueOf(song), segs)
}

func grow(v reflect.Value, segs []string) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return errNoPath
		}
		v = v.Elem()
	}
	if len(segs) == 0 {
		if v.Kind() != reflect.Slice {
			return errNoPath
		}
		e := reflect.New(v.Type().Elem()).Elem()
		switch e := e.Addr().Interface().(type) {
		case *synth.Osc:
			e.Wave, e.Level = "saw", 0.7
		case *synth.LFO:
			e.To, e.Wave, e.Hz, e.Depth = "cutoff", "sine", 2, 0.5
		}
		v.Set(reflect.Append(v, e))
		return nil
	}
	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName(segs[0])
		if !f.IsValid() {
			return errNoPath
		}
		return grow(f, segs[1:])
	case reflect.Map:
		e := v.MapIndex(reflect.ValueOf(segs[0]))
		if !e.IsValid() || e.Kind() != reflect.Pointer {
			return errNoPath
		}
		return grow(e, segs[1:])
	case reflect.Slice:
		i, ok := item(v, segs[0])
		if !ok {
			return errNoPath
		}
		return grow(v.Index(i), segs[1:])
	default:
	}
	return errNoPath
}

// removeItem takes item i out of the slice at path.
func removeItem(song *synth.Song, path string, i int) error {
	v := reflect.ValueOf(song)
	for _, seg := range strings.Split(path, "/") {
		for v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			v = v.FieldByName(seg)
		case reflect.Map:
			v = v.MapIndex(reflect.ValueOf(seg))
		case reflect.Slice:
			j, ok := item(v, seg)
			if !ok {
				return errNoPath
			}
			v = v.Index(j)
		default:
		}
		if !v.IsValid() {
			return errNoPath
		}
	}
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice || i < 0 || i >= v.Len() {
		return errNoPath
	}
	v.Set(reflect.AppendSlice(v.Slice(0, i), v.Slice(i+1, v.Len())))
	return nil
}
