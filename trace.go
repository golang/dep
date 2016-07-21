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

func (s *solver) traceVisit(bmi bimodalIdentifier, pkgonly bool) {
	if !s.params.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.vqs)+1)
	// TODO(sdboyer) how...to list the packages in the limited space we have?
	if pkgonly {
		s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? revisiting %s to add %v pkgs", bmi.id.errString(), len(bmi.pl)), prefix, prefix))
	} else {
		s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? attempting %s (with %v pkgs)", bmi.id.errString(), len(bmi.pl)), prefix, prefix))
	}
}

// Called just once after solving has finished, whether success or not
func (s *solver) traceFinish(sol solution, err error) {
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

// traceSelectRoot is called just once, when the root project is selected
func (s *solver) traceSelectRoot(ptree PackageTree, cdeps []completeDep) {
	if !s.params.Trace {
		return
	}

	// This duplicates work a bit, but we're in trace mode and it's only once,
	// so who cares
	rm := ptree.ExternalReach(true, true, s.ig)

	s.tl.Printf("Root project is %q", s.params.ImportRoot)

	var expkgs int
	for _, cdep := range cdeps {
		expkgs += len(cdep.pl)
	}

	// TODO(sdboyer) include info on ignored pkgs/imports, etc.
	s.tl.Printf(" %v transitively valid internal packages", len(rm))
	s.tl.Printf(" %v external packages imported from %v projects", expkgs, len(cdeps))
	s.tl.Printf(successCharSp + "select (root)")
}

// traceSelect is called when an atom is successfully selected
func (s *solver) traceSelect(awp atomWithPackages) {
	if !s.params.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.vqs))
	msg := fmt.Sprintf("%s select %s at %s", successChar, awp.a.id.errString(), awp.a.v)

	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
}

func (s *solver) traceInfo(args ...interface{}) {
	if !s.params.Trace {
		return
	}

	if len(args) == 0 {
		panic("must pass at least one param to traceInfo")
	}

	preflen := len(s.vqs) + 1
	var msg string
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
		panic(fmt.Sprintf("canary - unknown type passed as first param to traceInfo %T", data))
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
