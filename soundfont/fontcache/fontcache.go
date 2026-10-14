// Package fontcache keeps the SoundFont banks programs play on: where a
// bank fetched is kept, fetching it, unpacking it where it comes in an
// archive, letting it go, and which bank was played last, so the MIDI
// player, the studio and any other program find the same banks,
// whichever fetched them.
package fontcache

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	urlpath "path"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/ulikunitz/xz"

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
	for _, b := range soundfont.Banks {
		if path, ok := Fetched(b); ok {
			return path
		}
	}
	return ""
}

// Path returns where bank is kept once fetched: in Dir, or, for a bank
// that comes in an archive, in a folder of its own there, its Key, with
// the notices the archive holds beside it.
func Path(bank soundfont.Bank) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if bank.Archived() {
		return filepath.Join(dir, bank.Key(), bank.File), nil
	}
	return filepath.Join(dir, bank.File), nil
}

// Fetched returns where bank is kept once fetched, and whether it is
// there.
func Fetched(bank soundfont.Bank) (string, bool) {
	path, err := Path(bank)
	if err != nil {
		return "", false
	}
	_, err = os.Stat(path)
	return path, err == nil
}

// Notices returns the files kept beside bank that say what may be done
// with it: its licence, and for a bank from an archive, its readme.
func Notices(bank soundfont.Bank) []string {
	path, err := Path(bank)
	if err != nil {
		return nil
	}
	if !bank.Archived() {
		lic := licencePath(path)
		if _, err := os.Stat(lic); err != nil {
			return nil
		}
		return []string{lic}
	}
	txt, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.txt"))
	return txt
}

// licencePath is where the licence of the bank at path is kept, for a
// bank fetched whole.
func licencePath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + "-LICENSE.md"
}

// Download fetches bank as Fetch does, with no way to stop it.
func Download(bank soundfont.Bank, progress func(float64)) (string, error) {
	return Fetch(context.Background(), bank, progress)
}

// Fetch fetches bank, and its licence, to Dir, unless it is there
// already, telling progress how far it has come, from 0 to 1, and
// returns where the bank is. A bank in an archive is unpacked: its .sf2
// kept, and its notices, the .txt files, beside it; the archive goes.
// What is fetched is checked against the bank's SHA256, where it has
// one. Ending ctx stops it, and leaves nothing of it behind.
func Fetch(ctx context.Context, bank soundfont.Bank, progress func(float64)) (string, error) {
	path, err := Path(bank)
	if err != nil {
		return "", fmt.Errorf("no cache folder to fetch a sound font to: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if progress == nil {
		progress = func(float64) {}
	}
	if !bank.Archived() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if bank.LicenseURL != "" {
			if err := get(ctx, bank.LicenseURL, licencePath(path), "", nil); err != nil {
				return "", err
			}
		}
		if err := get(ctx, bank.URL, path, bank.SHA256, progress); err != nil {
			return "", err
		}
		return path, nil
	}
	dir := filepath.Dir(path)
	// What was left of a fetch that failed goes first.
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(dir, urlBase(bank.URL))
	defer os.Remove(archive)
	// The download is most of the wait; unpacking, the rest.
	const fetched = 0.8
	err = get(ctx, bank.URL, archive, bank.SHA256, func(x float64) { progress(x * fetched) })
	if err == nil {
		err = unpack(ctx, bank, archive, path, func(x float64) { progress(fetched + x*(1-fetched)) })
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return path, nil
}

// Remove lets bank go: the bank fetched, and its notices. A bank open
// on a system that keeps a file open in use may be kept till the
// program ends; then Remove fails, and leaves it.
func Remove(bank soundfont.Bank) error {
	path, err := Path(bank)
	if err != nil {
		return err
	}
	if bank.Archived() {
		// The bank goes first: it alone may be in use.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.RemoveAll(filepath.Dir(path))
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(licencePath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// get fetches url to the file to, by way of a .part file, telling
// progress, where there is one, how far it has come, and checking what
// came against sum, the hex SHA-256, where it is not "".
func get(ctx context.Context, url, to, sum string, progress func(float64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
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
	var h hash.Hash
	var w io.Writer = out
	if sum != "" {
		h = sha256.New()
		w = io.MultiWriter(out, h)
	}
	err = copyTelling(w, resp.Body, resp.ContentLength, progress)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && h != nil && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
		err = fmt.Errorf("%s came other than it should: its SHA-256 is %x", url, h.Sum(nil))
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}

// copyTelling copies r to w, telling progress, where there is one, how
// far it has come of total bytes, where total is known.
func copyTelling(w io.Writer, r io.Reader, total int64, progress func(float64)) error {
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if progress != nil && total > 0 {
				progress(min(float64(done)/float64(total), 1))
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// unpack takes the bank, the one .sf2, from archive to the file to, and
// the notices, the .txt files, to its folder, by their names alone,
// telling progress how far it has come.
func unpack(ctx context.Context, bank soundfont.Bank, archive, to string, progress func(float64)) error {
	found := false
	take := func(name string, size int64, r io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		base := urlpath.Base(strings.ReplaceAll(name, "\\", "/"))
		switch strings.ToLower(urlpath.Ext(base)) {
		case ".sf2":
			if found {
				return fmt.Errorf("%s holds more than one bank", urlBase(bank.URL))
			}
			found = true
			total := size
			if total <= 0 {
				total = bank.Unpacked
			}
			return write(to, r, total, progress)
		case ".txt":
			if base == "" || strings.HasPrefix(base, ".") {
				return nil
			}
			return write(filepath.Join(filepath.Dir(to), base), r, 0, nil)
		}
		return nil
	}
	var err error
	switch kind := strings.ToLower(urlBase(bank.URL)); {
	case strings.HasSuffix(kind, ".7z"):
		err = unpack7z(archive, take)
	case strings.HasSuffix(kind, ".tar.xz"):
		err = unpackTar(archive, func(r io.Reader) (io.Reader, error) { return xz.NewReader(r) }, take)
	case strings.HasSuffix(kind, ".tar.bz2"):
		err = unpackTar(archive, func(r io.Reader) (io.Reader, error) { return bzip2.NewReader(r), nil }, take)
	default:
		err = fmt.Errorf("%s is no archive I can unpack", urlBase(bank.URL))
	}
	if err == nil && !found {
		err = fmt.Errorf("%s holds no bank", urlBase(bank.URL))
	}
	return err
}

// unpack7z hands take each file of the 7-Zip archive at name.
func unpack7z(name string, take func(name string, size int64, r io.Reader) error) error {
	z, err := sevenzip.OpenReader(name)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = take(f.Name, int64(f.UncompressedSize), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// unpackTar hands take each file of the tar archive at name, read
// through the decompressor dec.
func unpackTar(name string, dec func(io.Reader) (io.Reader, error), take func(name string, size int64, r io.Reader) error) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := dec(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if err := take(h.Name, h.Size, tr); err != nil {
			return err
		}
	}
}

// write writes what r holds to the file to, by way of a .part file,
// telling progress, where there is one, how far it has come of total
// bytes.
func write(to string, r io.Reader, total int64, progress func(float64)) error {
	tmp := to + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = copyTelling(out, r, total, progress)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}

// urlBase is the last part of url's path, the file it names.
func urlBase(url string) string { return urlpath.Base(url) }
