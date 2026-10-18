package vault

import "github.com/godbus/dbus/v5"

const secretsName = "org.freedesktop.secrets"

// Available reports whether a Secret Service (GNOME Keyring, KWallet, …)
// owns or can be started for org.freedesktop.secrets on the session bus.
// A headless server usually has neither a session bus nor a keyring.
// Tests replace it.
var Available = func() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	bus := conn.BusObject()
	var has bool
	if bus.Call("org.freedesktop.DBus.NameHasOwner", 0, secretsName).Store(&has) == nil && has {
		return true
	}
	var names []string
	if bus.Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&names) == nil {
		for _, n := range names {
			if n == secretsName {
				return true
			}
		}
	}
	return false
}
