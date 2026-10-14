//go:build unix

package soundfont

import (
	"os"
	"syscall"
)

// mapFile maps the file at path into memory, read only, for as long as
// the program runs: its pages are read as they are first touched, so a
// large bank costs the memory of the samples played, not the whole
// file. Removing or replacing the file leaves the mapping as it was.
func mapFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	n := st.Size()
	if n == 0 || n != int64(int(n)) {
		return os.ReadFile(path)
	}
	b, err := syscall.Mmap(int(f.Fd()), 0, int(n), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return os.ReadFile(path)
	}
	return b, nil
}
