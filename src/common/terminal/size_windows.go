//go:build windows

package terminal

// querySize returns the 80x24 default on Windows, where no console API query
// is available without cgo or an external package.
func querySize() (int, int, bool) {
	return DefaultCols, DefaultRows, true
}
