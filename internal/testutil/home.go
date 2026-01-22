package testutil

import "testing"

// SetTestHomeDir sets the home directory for tests to work cross-platform.
// Both HOME (Unix) and USERPROFILE (Windows) are set so os.UserHomeDir() works everywhere.
func SetTestHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
