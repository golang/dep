package gps

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func mkTestCmd(iterations int) *monitoredCmd {
	return newMonitoredCmd(context.Background(),
		exec.Command("./echosleep", "-n", fmt.Sprint(iterations)),
		200*time.Millisecond,
	)
}

func TestMonitoredCmd(t *testing.T) {
	err := exec.Command("go", "build", "./_testdata/cmd/echosleep.go").Run()
	if err != nil {
		t.Errorf("Unable to build echosleep binary: %s", err)
	}
	defer os.Remove("./echosleep")

	cmd := mkTestCmd(2)
	err = cmd.run()
	if err != nil {
		t.Errorf("Expected command not to fail: %s", err)
	}

	expectedOutput := "foo\nfoo\n"
	if cmd.stdout.buf.String() != expectedOutput {
		t.Errorf("Unexpected output:\n\t(GOT): %s\n\t(WNT): %s", cmd.stdout.buf.String(), expectedOutput)
	}

	cmd2 := mkTestCmd(10)
	err = cmd2.run()
	if err == nil {
		t.Error("Expected command to fail")
	}

	_, ok := err.(*timeoutError)
	if !ok {
		t.Errorf("Expected a timeout error, but got: %s", err)
	}

	expectedOutput = "foo\nfoo\nfoo\nfoo\n"
	if cmd2.stdout.buf.String() != expectedOutput {
		t.Errorf("Unexpected output:\n\t(GOT): %s\n\t(WNT): %s", cmd2.stdout.buf.String(), expectedOutput)
	}
}
