package screen

import "os"

// NewDesktop returns this OS's Desktop, which is also its Provider. exe is
// the running clawdyx binary (the capture helper is looked for next to it).
func NewDesktop(exe string) Desktop {
	m := Mac{TmpDir: os.TempDir()}
	if exe != "" {
		m.Helper = FindHelper(exe)
	}
	return m
}
