package log

import (
	"fmt"
	"io"
)

// Logger is a minimal wrapper around an io.Writer.
type Logger struct {
	io.Writer
}

// New returns a new logger which writes to w.
func New(w io.Writer) *Logger {
	return &Logger{Writer: w}
}

// Logln logs a line.
func (l *Logger) Logln(args ...interface{}) {
	fmt.Fprintln(l, args...)
}

// Logf logs a formatted string.
func (l *Logger) Logf(f string, args ...interface{}) {
	fmt.Fprintf(l, f, args...)
}

// LogDepfln logs a formatted line, prefixed with `dep: `.
func (l *Logger) LogDepfln(format string, args ...interface{}) {
	fmt.Fprintf(l, "dep: "+format+"\n", args...)
}
