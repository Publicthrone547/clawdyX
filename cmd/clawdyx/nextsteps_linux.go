package main

// headlessSteps end the setup of a server: the systemd user service, kept
// running without a login.
const headlessSteps = "clawdyx service start           install and start the systemd user service\n" +
	"  loginctl enable-linger $USER    once: keep it running from boot, without a login\n" +
	"  clawdyx service status          check it (logs: journalctl --user -u clawdyx)"
