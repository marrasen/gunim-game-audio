// Package fontcache keeps the SoundFont banks the tools play on: where a
// bank fetched is kept, fetching it, and which bank was played last, so
// the MIDI player and the studio find the same bank, whichever loaded it.
package fontcache

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/marrasen/gunim-game-audio/soundfont"
)

// Dir returns the folder banks are fetched to, in the user's cache. It
// is the MIDI player's, which fetched them first.
func Dir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gunim-midiplayer", "soundfonts"), nil
}

// Saved returns the file that remembers the bank played last, in the
// user's config, or "" where there is no config folder.
func Saved() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "gunim-midiplayer", "soundfont.txt")
}

// Remember saves path as the bank played last.
func Remember(path string) {
	file := Saved()
	if file == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, []byte(path), 0o644)
}

// Remembered returns the bank played last, where it is still there, or
// "".
func Remembered() string {
	file := Saved()
	if file == "" {
		return ""
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(b))
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// Find returns a bank to play on: the one played last, or else one of
// soundfont.Banks fetched before, or "" for none.
func Find() string {
	if path := Remembered(); path != "" {
		return path
	}
	dir, err := Dir()
	if err != nil {
		return ""
	}
	for _, b := range soundfont.Banks {
		path := filepath.Join(dir, b.File)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// Fetched returns where bank is kept once fetched, and whether it is
// there.
func Fetched(bank soundfont.Bank) (string, bool) {
	dir, err := Dir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(dir, bank.File)
	_, err = os.Stat(path)
	return path, err == nil
}

// Download fetches bank, and its licence beside it, to Dir, unless it is
// there already, telling progress how far it has come, from 0 to 1. It
// returns where the bank is.
func Download(bank soundfont.Bank, progress func(float64)) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", fmt.Errorf("no cache folder to fetch a sound font to: %w", err)
	}
	path := filepath.Join(dir, bank.File)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	licence := filepath.Join(dir, strings.TrimSuffix(bank.File, filepath.Ext(bank.File))+"-LICENSE.md")
	if err := get(bank.LicenseURL, licence, nil); err != nil {
		return "", err
	}
	if err := get(bank.URL, path, progress); err != nil {
		return "", err
	}
	return path, nil
}

// get fetches url to the file to, by way of a .part file, telling
// progress, where there is one, how far it has come.
func get(url, to string, progress func(float64)) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	tmp := to + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				return err
			}
			done += int64(n)
			if progress != nil && total > 0 {
				progress(float64(done) / float64(total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}
