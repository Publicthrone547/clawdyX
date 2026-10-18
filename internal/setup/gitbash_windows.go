package setup

import (
	"os"
	"os/exec"
	"path/filepath"
)

// DetectGitBash finds the bash.exe of Git for Windows, which Claude Code's
// shell tool needs: next to git.exe on PATH, then the default install
// folders (machine-wide and per user). "" when Git for Windows is missing.
func DetectGitBash() string {
	var candidates []string
	if git, err := exec.LookPath("git"); err == nil {
		// …\Git\cmd\git.exe or …\Git\bin\git.exe → …\Git\bin\bash.exe
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(git)), "bin", "bash.exe"))
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if d := os.Getenv(env); d != "" {
			candidates = append(candidates, filepath.Join(d, "Git", "bin", "bash.exe"))
		}
	}
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		candidates = append(candidates, filepath.Join(d, "Programs", "Git", "bin", "bash.exe"))
	}
	return firstFile(candidates)
}
