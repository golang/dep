package gps

import (
	"fmt"
	"strings"
)

const (
	successChar   = "✓"
	successCharSp = successChar + " "
	failChar      = "✗"
	failCharSp    = failChar + " "
)

func (s *solver) logStart(bmi bimodalIdentifier) {
	if !s.params.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.vqs)+1)
	// TODO(sdboyer) how...to list the packages in the limited space we have?
	s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? attempting %s (with %v packages)", bmi.id.errString(), len(bmi.pl)), prefix, prefix))
}

func (s *solver) logFinish(sol solution, err error) {
	if !s.params.Trace {
		return
	}

	if err == nil {
		var pkgcount int
		for _, lp := range sol.Projects() {
			pkgcount += len(lp.pkgs)
		}
		s.tl.Printf("%s found solution with %v packages from %v projects", successChar, pkgcount, len(sol.Projects()))
	} else {
		s.tl.Printf("%s solving failed", failChar)
	}
}

func (s *solver) logSolve(args ...interface{}) {
	if !s.params.Trace {
		return
	}

	preflen := len(s.vqs)
	var msg string
	if len(args) == 0 {
		// Generate message based on current solver state
		if len(s.vqs) == 0 {
			msg = successCharSp + "(root)"
		} else {
			vq := s.vqs[len(s.vqs)-1]
			msg = fmt.Sprintf("%s select %s at %s", successChar, vq.id.errString(), vq.current())
		}
	} else {
		// Use longer prefix length for these cases, as they're the intermediate
		// work
		preflen++
		switch data := args[0].(type) {
		case string:
			msg = tracePrefix(fmt.Sprintf(data, args[1:]), "| ", "| ")
		case traceError:
			// We got a special traceError, use its custom method
			msg = tracePrefix(data.traceString(), "| ", failCharSp)
		case error:
			// Regular error; still use the x leader but default Error() string
			msg = tracePrefix(data.Error(), "| ", failCharSp)
		default:
			// panic here because this can *only* mean a stupid internal bug
			panic("canary - must pass a string as first arg to logSolve, or no args at all")
		}
	}

	prefix := strings.Repeat("| ", preflen)
	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
}

func tracePrefix(msg, sep, fsep string) string {
	parts := strings.Split(strings.TrimSuffix(msg, "\n"), "\n")
	for k, str := range parts {
		if k == 0 {
			parts[k] = fmt.Sprintf("%s%s", fsep, str)
		} else {
			parts[k] = fmt.Sprintf("%s%s", sep, str)
		}
	}

	return strings.Join(parts, "\n")
}
