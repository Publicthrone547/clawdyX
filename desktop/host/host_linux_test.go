package host

import "testing"

func TestServiceShow(t *testing.T) {
	m := ServiceShow("ActiveState=active\nMainPID=4242\nUnitFileState=enabled\n")
	if m["ActiveState"] != "active" || m["MainPID"] != "4242" || m["UnitFileState"] != "enabled" {
		t.Fatalf("%v", m)
	}
}
