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
	return newMonitoredCmd(
		exec.Command("./echosleep", "-n", fmt.Sprint(iterations)),
		500*time.Millisecond,
	)
}

func TestMonitoredCmd(t *testing.T) {
	// Sleeps and compile make this a bit slow
	if testing.Short() {
		t.Skip("skipping test with sleeps on short")
	}

	err := exec.Command("go", "build", "./_testdata/cmd/echosleep.go").Run()
	if err != nil {
		t.Errorf("Unable to build echosleep binary: %s", err)
	}
	defer os.Remove("./echosleep")

	cmd := mkTestCmd(2)
	err = cmd.run(context.Background())
	if err != nil {
		t.Errorf("Expected command not to fail: %s", err)
	}

	expectedOutput := "foo\nfoo\n"
	if cmd.stdout.buf.String() != expectedOutput {
		t.Errorf("Unexpected output:\n\t(GOT): %s\n\t(WNT): %s", cmd.stdout.buf.String(), expectedOutput)
	}

	cmd2 := mkTestCmd(10)
	err = cmd2.run(context.Background())
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

	ctx, cancel := context.WithCancel(context.Background())
	sync1, errchan := make(chan struct{}), make(chan error)
	cmd3 := mkTestCmd(2)
	go func() {
		close(sync1)
		errchan <- cmd3.run(ctx)
	}()

	// Make sure goroutine is at least started before we cancel the context.
	<-sync1
	// Give it a bit to get the process started.
	<-time.After(5 * time.Millisecond)
	cancel()

	err = <-errchan
	if err != context.Canceled {
		t.Errorf("should have gotten canceled error, got %s", err)
	}
}
