package soundfont

import (
	"os"
	"syscall"
	"unsafe"
)

// mapFile maps the file at path into memory, read only, for as long as
// the program runs: its pages are read as they are first touched, so a
// large bank costs the memory of the samples played, not the whole
// file. While it is mapped, the file cannot be removed.
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
	h, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, uint32(uint64(n)>>32), uint32(n), nil)
	if err != nil {
		return os.ReadFile(path)
	}
	// The view keeps the mapping open once its handle is closed.
	defer syscall.CloseHandle(h)
	addr, err := syscall.MapViewOfFile(h, syscall.FILE_MAP_READ, 0, 0, uintptr(n))
	if err != nil {
		return os.ReadFile(path)
	}
	// The view is the system's memory, out of the collector's reach, so
	// the slice is laid over its address as it stands.
	view := struct {
		addr     uintptr
		len, cap int
	}{addr, int(n), int(n)}
	return *(*[]byte)(unsafe.Pointer(&view)), nil
}
