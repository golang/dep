// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
		490*time.Millisecond,
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

	tests := []struct {
		name       string
		iterations int
		output     string
		err        bool
		timeout    bool
	}{
		{"success", 2, "foo\nfoo\n", false, false},
		{"timeout", 5, "foo\nfoo\nfoo\nfoo\n", true, true},
	}

	for _, want := range tests {
		t.Run(want.name, func(t *testing.T) {
			cmd := mkTestCmd(want.iterations)

			err := cmd.run(context.Background())
			if !want.err && err != nil {
				t.Errorf("Eexpected command not to fail, got error: %s", err)
			} else if want.err && err == nil {
				t.Error("expected command to fail")
			}

			got := cmd.stdout.String()
			if want.output != got {
				t.Errorf("unexpected output:\n\t(GOT):\n%s\n\t(WNT):\n%s", got, want.output)
			}

			if want.timeout {
				_, ok := err.(*noProgressError)
				if !ok {
					t.Errorf("Expected a timeout error, but got: %s", err)
				}
			}
		})
	}

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		sync, errchan := make(chan struct{}), make(chan error)
		cmd := mkTestCmd(2)
		go func() {
			close(sync)
			errchan <- cmd.run(ctx)
		}()

		// Make sure goroutine is at least started before we cancel the context.
		<-sync
		// Give it a bit to get the process started.
		<-time.After(5 * time.Millisecond)
		cancel()

		err := <-errchan
		if err != context.Canceled {
			t.Errorf("expected a canceled error, got %s", err)
		}
	})
}
