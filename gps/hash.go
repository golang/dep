package gps

import (
	"bytes"
	"crypto/sha256"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/dep/gps/pkgtree"
)

// string headers used to demarcate sections in hash input creation
const (
	hhConstraints = "-CONSTRAINTS-"
	hhImportsReqs = "-IMPORTS/REQS-"
	hhIgnores     = "-IGNORES-"
	hhOverrides   = "-OVERRIDES-"
	hhAnalyzer    = "-ANALYZER-"
)

// HashInputs computes a hash digest of all data in SolveParams and the
// RootManifest that act as function inputs to Solve().
//
// The digest returned from this function is the same as the digest that would
// be included with a Solve() Result. As such, it's appropriate for comparison
// against the digest stored in a lock file, generated by a previous Solve(): if
// the digests match, then manifest and lock are in sync, and a Solve() is
// unnecessary.
//
// (Basically, this is for memoization.)
func (s *solver) HashInputs() (digest []byte) {
	h := sha256.New()
	s.writeHashingInputs(h)

	hd := h.Sum(nil)
	digest = hd[:]
	return
}

func (s *solver) writeHashingInputs(w io.Writer) {
	writeString := func(s string) {
		// Skip zero-length string writes; it doesn't affect the real hash
		// calculation, and keeps misleading newlines from showing up in the
		// debug output.
		if s != "" {
			// All users of writeHashingInputs cannot error on Write(), so just
			// ignore it
			w.Write([]byte(s))
		}
	}

	// We write "section headers" into the hash purely to ease scanning when
	// debugging this input-constructing algorithm; as long as the headers are
	// constant, then they're effectively a no-op.
	writeString(hhConstraints)

	// getApplicableConstraints will apply overrides, incorporate requireds,
	// apply local ignores, drop stdlib imports, and finally trim out
	// ineffectual constraints.
	for _, pd := range s.rd.getApplicableConstraints() {
		writeString(string(pd.Ident.ProjectRoot))
		writeString(pd.Ident.Source)
		writeString(pd.Constraint.typedString())
	}

	// Write out each discrete import, including those derived from requires.
	writeString(hhImportsReqs)
	imports := s.rd.externalImportList()
	sort.Strings(imports)
	for _, im := range imports {
		writeString(im)
	}

	// Add ignores, skipping any that point under the current project root;
	// those will have already been implicitly incorporated by the import
	// lister.
	writeString(hhIgnores)
	ig := make([]string, 0, len(s.rd.ig))
	for pkg := range s.rd.ig {
		if !strings.HasPrefix(pkg, s.rd.rpt.ImportRoot) || !isPathPrefixOrEqual(s.rd.rpt.ImportRoot, pkg) {
			ig = append(ig, pkg)
		}
	}
	sort.Strings(ig)

	for _, igp := range ig {
		writeString(igp)
	}

	// Overrides *also* need their own special entry distinct from basic
	// constraints, to represent the unique effects they can have on the entire
	// solving process beyond root's immediate scope.
	writeString(hhOverrides)
	for _, pc := range s.rd.ovr.asSortedSlice() {
		writeString(string(pc.Ident.ProjectRoot))
		if pc.Ident.Source != "" {
			writeString(pc.Ident.Source)
		}
		if pc.Constraint != nil {
			writeString(pc.Constraint.typedString())
		}
	}

	writeString(hhAnalyzer)
	an, av := s.rd.an.Info()
	writeString(an)
	writeString(strconv.Itoa(av))
}

// bytes.Buffer wrapper that injects newlines after each call to Write().
type nlbuf bytes.Buffer

func (buf *nlbuf) Write(p []byte) (n int, err error) {
	n, _ = (*bytes.Buffer)(buf).Write(p)
	(*bytes.Buffer)(buf).WriteByte('\n')
	return n + 1, nil
}

// HashingInputsAsString returns the raw input data used by Solver.HashInputs()
// as a string.
//
// This is primarily intended for debugging purposes.
func HashingInputsAsString(s Solver) string {
	ts := s.(*solver)
	buf := new(nlbuf)
	ts.writeHashingInputs(buf)

	return (*bytes.Buffer)(buf).String()
}

type sortPackageOrErr []pkgtree.PackageOrErr

func (s sortPackageOrErr) Len() int      { return len(s) }
func (s sortPackageOrErr) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s sortPackageOrErr) Less(i, j int) bool {
	a, b := s[i], s[j]
	if a.Err != nil || b.Err != nil {
		// Sort errors last.
		if b.Err == nil {
			return false
		}
		if a.Err == nil {
			return true
		}
		// And then by string.
		return a.Err.Error() < b.Err.Error()
	}
	// And finally, sort by import path.
	return a.P.ImportPath < b.P.ImportPath
}
