package fontcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim-game-audio/soundfont"
)

// serve serves the files of testdata, and sets the user's folders to
// fresh ones, for Dir to fetch to.
func serve(t *testing.T) *httptest.Server {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	t.Setenv("AppData", filepath.Join(home, "config"))
	s := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	t.Cleanup(s.Close)
	return s
}

// sumOf is the hex SHA-256 of the file testdata/name.
func sumOf(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestABankInAnArchiveIsUnpackedWithItsNotices(t *testing.T) {
	want, err := os.ReadFile("testdata/TestBank.sf2")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TestBank-SF2-lzma2.7z", "TestBank-SF2-bzip2.7z", "TestBank-SF2.tar.xz", "TestBank-SF2.tar.bz2"} {
		t.Run(name, func(t *testing.T) {
			s := serve(t)
			bank := soundfont.Bank{Name: "Test", File: "TestBank.sf2", URL: s.URL + "/" + name, SHA256: sumOf(t, name)}
			var last float64
			path, err := Fetch(context.Background(), bank, func(x float64) {
				if x < last {
					t.Errorf("the progress went back, from %v to %v", last, x)
				}
				last = x
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := Fetched(bank); !ok || got != path {
				t.Errorf("Fetched says %q, %v; Fetch said %q", got, ok, path)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("the bank unpacked is %d bytes (%v), want the %d of the bank", len(got), err, len(want))
			}
			if last != 1 {
				t.Errorf("the progress ended at %v, not 1", last)
			}
			found := Notices(bank)
			notices := make([]string, 0, len(found))
			for _, n := range found {
				notices = append(notices, filepath.Base(n))
			}
			slices.Sort(notices)
			if !slices.Equal(notices, []string{"LICENSE.txt", "README.txt"}) {
				t.Errorf("the notices are %v", notices)
			}
			// Nothing else is left: not the archive, nor a part.
			left, _ := os.ReadDir(filepath.Dir(path))
			if len(left) != 3 {
				t.Errorf("the bank's folder holds %d files, want the bank and its two notices", len(left))
			}
			if _, err := soundfont.Open(path); err != nil {
				t.Errorf("the bank unpacked does not open: %v", err)
			}
			if err := Remove(bank); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the bank's folder is still there once removed: %v", err)
			}
		})
	}
}

func TestABankWholeIsFetchedWithItsLicence(t *testing.T) {
	s := serve(t)
	bank := soundfont.Bank{Name: "Test", File: "TestBank.sf2", URL: s.URL + "/TestBank.sf2",
		LicenseURL: s.URL + "/TestBank.sf2", SHA256: sumOf(t, "TestBank.sf2")}
	path, err := Fetch(context.Background(), bank, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := Dir()
	if path != filepath.Join(dir, "TestBank.sf2") {
		t.Errorf("a bank fetched whole is kept at %s, not in the cache's folder itself", path)
	}
	if n := Notices(bank); len(n) != 1 || !strings.HasSuffix(n[0], "TestBank-LICENSE.md") {
		t.Errorf("the notices are %v", n)
	}
	if err := Remove(bank); err != nil {
		t.Fatal(err)
	}
	if _, ok := Fetched(bank); ok || len(Notices(bank)) != 0 {
		t.Error("the bank, or its licence, is still there once removed")
	}
}

func TestABankThatComesWrongIsRefusedAndLeavesNothing(t *testing.T) {
	s := serve(t)
	bank := soundfont.Bank{Name: "Test", File: "TestBank.sf2", URL: s.URL + "/TestBank-SF2.tar.xz",
		SHA256: strings.Repeat("0", 64)}
	if _, err := Fetch(context.Background(), bank, nil); err == nil {
		t.Fatal("a bank whose sum is wrong was taken")
	}
	path, _ := Path(bank)
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a fetch that failed left its folder: %v", err)
	}
}

func TestAFetchStoppedLeavesNothing(t *testing.T) {
	s := serve(t)
	bank := soundfont.Bank{Name: "Test", File: "TestBank.sf2", URL: s.URL + "/TestBank-SF2-lzma2.7z"}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := Fetch(ctx, bank, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("a fetch stopped says %v", err)
	}
	if _, ok := Fetched(bank); ok {
		t.Error("a fetch stopped left the bank")
	}
}

func TestTheCatalogueIsWhole(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range slices.Concat(soundfont.Banks, soundfont.Instruments) {
		if b.Name == "" || b.File == "" || b.URL == "" || b.License == "" || b.LicenseURL == "" || b.Credit == "" || b.Size <= 0 {
			t.Errorf("%s lacks what it should say: %+v", b.Name, b)
		}
		if seen[b.Key()] {
			t.Errorf("two banks are kept as %s", b.Key())
		}
		seen[b.Key()] = true
	}
	for _, b := range soundfont.Instruments {
		if !b.Archived() || len(b.SHA256) != 64 || b.Unpacked <= 0 || b.Group == "" || b.About == "" {
			t.Errorf("%s, an instrument, lacks its archive, its sum, its size unpacked, its group or what it is", b.Name)
		}
	}
}
