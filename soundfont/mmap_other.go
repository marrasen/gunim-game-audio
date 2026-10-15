//go:build !unix && !windows

package soundfont

import "os"

// mapFile reads the file at path whole, where the system maps none.
func mapFile(path string) ([]byte, error) { return os.ReadFile(path) }
