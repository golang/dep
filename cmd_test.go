package gps

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func mkTestCmd(iterations int) *monitoredCmd {
	return newMonitoredCmd(
		exec.Command("go", "run", "./_testdata/src/cmd/echosleep.go", "-n", fmt.Sprint(iterations)),
		200*time.Millisecond,
	)
}

func TestMonitoredCmd(t *testing.T) {
	cmd := mkTestCmd(2)
	err := cmd.run()
	if err != nil {
		t.Errorf("expected command not to fail:", err)
	}

	expectedOutput := "foo\nfoo\n"
	if cmd.buf.buf.String() != expectedOutput {
		t.Errorf("expected output %s to be %s", cmd.buf.buf.String(), expectedOutput)
	}

	cmd = mkTestCmd(10)
	err = cmd.run()
	if err == nil {
		t.Errorf("expected command to fail")
	}

	expectedOutput = "foo\nfoo\nfoo\nfoo\nfoo\n"
	if cmd.buf.buf.String() != expectedOutput {
		t.Errorf("expected output %s to be %s", cmd.buf.buf.String(), expectedOutput)
	}
}
