//go:build !unix && !windows

package soundfont

import "os"

// mapFile reads the file at path whole, where the system maps none, so
// there is nothing to release.
func mapFile(path string) ([]byte, func() error, error) {
	b, err := os.ReadFile(path)
	return b, nil, err
}
