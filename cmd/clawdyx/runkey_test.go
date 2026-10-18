package main

import (
	"path/filepath"
	"testing"
)

func TestRunCommandRoundTrip(t *testing.T) {
	dir := filepath.Join(`C:\Users\Иван Петров\AppData\Local\Programs\clawdyX`, "helpers")
	exe := filepath.Join(dir, "clawdyx.exe")
	cfg := `C:\Users\Иван Петров\AppData\Roaming\clawdyx\config.json`

	v := RunCommand(exe, cfg, true)
	if want := `"` + filepath.Join(dir, "clawdyxw.exe") + `" -config "` + cfg + `"`; v != want {
		t.Fatalf("launcher command %q", v)
	}
	if got := runProgram(v); got != exe {
		t.Fatalf("program behind the launcher %q", got)
	}

	v = RunCommand(exe, cfg, false)
	if want := `"` + exe + `" service supervise -config "` + cfg + `"`; v != want {
		t.Fatalf("direct command %q", v)
	}
	if got := runProgram(v); got != exe {
		t.Fatalf("program %q", got)
	}

	// The 0.4.x value still names its worker.
	if got := runProgram(`"C:\x\clawdyx.exe" run -config "C:\y\config.json"`); got != `C:\x\clawdyx.exe` {
		t.Fatal(got)
	}
	if runProgram(`clawdyx.exe run`) != "" || runProgram(`"unterminated`) != "" {
		t.Fatal("unquoted or broken values must not yield a program")
	}
	if quoteArg(`C:\dir\`) != `"C:\dir\\"` {
		t.Fatal("a trailing backslash must not escape the quote")
	}
}
