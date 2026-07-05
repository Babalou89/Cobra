package worker

import "testing"

func TestGuardBlocksCommandPosition(t *testing.T) {
	for _, cmd := range []string{
		"sudo apt install x",
		"ls; sudo reboot",
		"echo hi && ssh host",
		"true | ssh host",
		"$(sudo id)",
		"git status",
	} {
		if err := guard(cmd); err == nil {
			t.Errorf("must block: %q", cmd)
		}
	}
}

func TestGuardAllowsArgumentsAndPaths(t *testing.T) {
	for _, cmd := range []string{
		"ls -la /home/billy/sudo",
		"stat /home/billy/sudo/",
		"du -sh /home/billy/continual.git",
		"grep -r sudoers /etc/hosts",
		"echo the word git appears here",
	} {
		if err := guard(cmd); err != nil {
			t.Errorf("must allow: %q (%v)", cmd, err)
		}
	}
}
