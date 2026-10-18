package main

import "testing"

func TestRunCommandRoundTrip(t *testing.T) {
	exe := `C:\Users\Jane Doe\AppData\Local\Programs\clawdyX\helpers\clawdyx.exe`
	v := RunCommand(exe, `C:\Users\Jane Doe\.config\clawdyx\config.json`)
	if want := `"` + exe + `" run -config "C:\Users\Jane Doe\.config\clawdyx\config.json"`; v != want {
		t.Fatalf("command %q", v)
	}
	if got := runProgram(v); got != exe {
		t.Fatalf("program %q", got)
	}
	if runProgram(`clawdyx.exe run`) != "" || runProgram(`"unterminated`) != "" {
		t.Fatal("unquoted or broken values must not yield a program")
	}
}
