package host

import "testing"

func TestRunProgram(t *testing.T) {
	if got := RunProgram(`"C:\P\clawdyX\helpers\clawdyxw.exe" -config "C:\c.json"`); got != `C:\P\clawdyX\helpers\clawdyx.exe` {
		t.Fatal(got)
	}
	if got := RunProgram(`"C:\x\clawdyx.exe" run -config "C:\y"`); got != `C:\x\clawdyx.exe` {
		t.Fatal(got)
	}
	if RunProgram("clawdyx.exe") != "" {
		t.Fatal("unquoted")
	}
}
