package main

import "github.com/golang/dep/log"

// Loggers holds standard loggers and a verbosity flag.
type Loggers struct {
	Out, Err *log.Logger
	// Whether verbose logging is enabled.
	Verbose bool
}
