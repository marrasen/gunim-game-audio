package synth

import (
	"errors"
	"fmt"
	"math"
	"reflect"
)

// CheckPatch reports what is wrong with p, a patch from outside the
// program, as one a user edited: a number that is not finite, a kind,
// a wave or a drum the engine has none of, or a patch whose notes fail
// to sound as numbers. It plays a few notes of p, held, bent and let
// go, and checks every frame. A patch it passes plays on an
// [Instrument] without a fault.
func CheckPatch(p *Patch) (err error) {
	if p == nil {
		return errors.New("synth: no patch")
	}
	if path, ok := finite(reflect.ValueOf(p).Elem(), "patch"); !ok {
		return fmt.Errorf("synth: %s is not a finite number", path)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("synth: the patch fails as it plays: %v", r)
		}
	}()
	in, err := NewInstrument(p)
	if err != nil {
		return err
	}
	l, r := make([]float32, 512), make([]float32, 512)
	play := func() error {
		in.Render(l, r)
		for i := range l {
			if v := float64(l[i]) + float64(r[i]); math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("synth: the patch sounds %v", v)
			}
		}
		return nil
	}
	in.Pedal(0, 1)
	for i, pitch := range []float64{36, 38, 42, 60, 64, 67, 96} {
		in.NoteOn(int32(i+1), pitch, 0.9, 0)
	}
	in.Bend(4, 2)
	for range 8 {
		if err := play(); err != nil {
			return err
		}
	}
	for i := range 7 {
		in.NoteOff(int32(i + 1))
	}
	in.Pedal(0, 0)
	for range 4 {
		if err := play(); err != nil {
			return err
		}
	}
	return nil
}

// finite reports whether every number in v is finite, and the path of
// the first that is not.
func finite(v reflect.Value, path string) (string, bool) {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		return path, !math.IsNaN(f) && !math.IsInf(f, 0)
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return "", true
		}
		return finite(v.Elem(), path)
	case reflect.Struct:
		for i := range v.NumField() {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if p, ok := finite(v.Field(i), path+"."+v.Type().Field(i).Name); !ok {
				return p, false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if p, ok := finite(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); !ok {
				return p, false
			}
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if p, ok := finite(it.Value(), fmt.Sprintf("%s[%v]", path, it.Key())); !ok {
				return p, false
			}
		}
	default:
	}
	return "", true
}
