//go:build !aix && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package atomicfile

// Windows does not expose a portable directory fsync through os.File. The
// temporary file is still synced before the atomic rename.
func syncParent(string) error {
	return nil
}
