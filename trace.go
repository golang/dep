package gps

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	successChar   = "✓"
	successCharSp = successChar + " "
	failChar      = "✗"
	failCharSp    = failChar + " "
	backChar      = "←"
)

func (s *solver) traceCheckPkgs(bmi bimodalIdentifier) {
	if !s.params.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.vqs)+1)
	s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? revisit %s to add %v pkgs", bmi.id.errString(), len(bmi.pl)), prefix, prefix))
}

func (s *solver) traceCheckQueue(q *versionQueue, bmi bimodalIdentifier, cont bool, offset int) {
	if !s.params.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.vqs)+offset)
	vlen := strconv.Itoa(len(q.pi))
	if !q.allLoaded {
		vlen = "at least " + vlen
	}

	// TODO(sdboyer) how...to list the packages in the limited space we have?
	var verb string
	if cont {
		verb = "continue"
		vlen = vlen + " more"
	} else {
		verb = "attempt"
	}

	s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? %s %s with %v pkgs; %s versions to try", verb, bmi.id.errString(), len(bmi.pl), vlen), prefix, prefix))
}

// traceStartBacktrack is called with the bmi that first failed, thus initiating
// backtracking
func (s *solver) traceStartBacktrack(bmi bimodalIdentifier, err error, pkgonly bool) {
	if !s.params.Trace {
		return
	}

	var msg string
	if pkgonly {
		msg = fmt.Sprintf("%s could not add %v pkgs to %s; begin backtrack", backChar, len(bmi.pl), bmi.id.errString())
	} else {
		msg = fmt.Sprintf("%s no more versions of %s to try; begin backtrack", backChar, bmi.id.errString())
	}

	prefix := strings.Repeat("| ", len(s.sel.projects))
	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
}

// traceBacktrack is called when a package or project is poppped off during
// backtracking
func (s *solver) traceBacktrack(bmi bimodalIdentifier, pkgonly bool) {
	if !s.params.Trace {
		return
	}

	var msg string
	if pkgonly {
		msg = fmt.Sprintf("%s backtrack: popped %v pkgs from %s", backChar, len(bmi.pl), bmi.id.errString())
	} else {
		msg = fmt.Sprintf("%s backtrack: no more versions of %s to try", backChar, bmi.id.errString())
	}

	prefix := strings.Repeat("| ", len(s.sel.projects))
	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
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

	s.tl.Printf("Root project is %q", s.rpt.ImportRoot)

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
func (s *solver) traceSelect(awp atomWithPackages, pkgonly bool) {
	if !s.params.Trace {
		return
	}

	var msg string
	if pkgonly {
		msg = fmt.Sprintf("%s include %v more pkgs from %s", successChar, len(awp.pl), a2vs(awp.a))
	} else {
		msg = fmt.Sprintf("%s select %s w/%v pkgs", successChar, a2vs(awp.a), len(awp.pl))
	}

	prefix := strings.Repeat("| ", len(s.sel.projects)-1)
	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
}

func (s *solver) traceInfo(args ...interface{}) {
	if !s.params.Trace {
		return
	}

	if len(args) == 0 {
		panic("must pass at least one param to traceInfo")
	}

	preflen := len(s.sel.projects)
	var msg string
	switch data := args[0].(type) {
	case string:
		msg = tracePrefix(fmt.Sprintf(data, args[1:]...), "| ", "| ")
	case traceError:
		preflen += 1
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
			parts[k] = fsep + str
		} else {
			parts[k] = sep + str
		}
	}

	return strings.Join(parts, "\n")
}
