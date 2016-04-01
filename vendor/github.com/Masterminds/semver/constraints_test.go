package semver

import "testing"

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		in  string
		c   Constraint
		err bool
	}{
		{"*", Any(), false},
		{">= 1.2", rangeConstraint{
			min:        newV(1, 2, 0),
			includeMin: true,
		}, false},
		{"1.0", newV(1, 0, 0), false},
		{"foo", nil, true},
		{"<= 1.2", rangeConstraint{
			max:        newV(1, 2, 0),
			includeMax: true,
		}, false},
		{"=< 1.2", rangeConstraint{
			max:        newV(1, 2, 0),
			includeMax: true,
		}, false},
		{"=> 1.2", rangeConstraint{
			min:        newV(1, 2, 0),
			includeMin: true,
		}, false},
		{"v1.2", newV(1, 2, 0), false},
		{"=1.5", newV(1, 5, 0), false},
		{"> 1.3", rangeConstraint{
			min: newV(1, 3, 0),
		}, false},
		{"< 1.4.1", rangeConstraint{
			max: newV(1, 4, 1),
		}, false},
		{"~1.1.0", rangeConstraint{
			min:        newV(1, 1, 0),
			max:        newV(1, 2, 0),
			includeMin: true,
			includeMax: false,
		}, false},
		{"^1.1.0", rangeConstraint{
			min:        newV(1, 1, 0),
			max:        newV(2, 0, 0),
			includeMin: true,
			includeMax: false,
		}, false},
	}

	for _, tc := range tests {
		c, err := parseConstraint(tc.in)
		if tc.err && err == nil {
			t.Errorf("Expected error for %s didn't occur", tc.in)
		} else if !tc.err && err != nil {
			t.Errorf("Unexpected error %q for %s", err, tc.in)
		}

		// If an error was expected continue the loop and don't try the other
		// tests as they will cause errors.
		if tc.err {
			continue
		}

		if !constraintEq(tc.c, c) {
			t.Errorf("Incorrect version found on %s", tc.in)
		}
	}
}

func constraintEq(c1, c2 Constraint) bool {
	switch tc1 := c1.(type) {
	case any:
		if _, ok := c2.(any); !ok {
			return false
		}
		return true
	case none:
		if _, ok := c2.(none); !ok {
			return false
		}
		return true
	case *Version:
		if tc2, ok := c2.(*Version); ok {
			return tc1.Equal(tc2)
		}
		return false
	case rangeConstraint:
		if tc2, ok := c2.(rangeConstraint); ok {
			if len(tc1.excl) != len(tc2.excl) {
				return false
			}

			if tc1.min != nil {
				if !(tc1.includeMin == tc2.includeMin && tc1.min.Equal(tc2.min)) {
					return false
				}
			} else if tc2.min != nil {
				return false
			}

			if tc1.max != nil {
				if !(tc1.includeMax == tc2.includeMax && tc1.max.Equal(tc2.max)) {
					return false
				}
			} else if tc2.max != nil {
				return false
			}

			for k, e := range tc1.excl {
				if !e.Equal(tc2.excl[k]) {
					return false
				}
			}
			return true
		}
		return false
	case unionConstraint:
		if tc2, ok := c2.(unionConstraint); ok {
			if len(tc1) != len(tc2) {
				return false
			}

			for k, c := range tc1 {
				if !constraintEq(c, tc2[k]) {
					return false
				}
			}
			return true
		}
		return false
	}

	panic("unknown type")
}

// newV is a helper to create a new Version object.
func newV(major, minor, patch int64) *Version {
	return &Version{
		major: major,
		minor: minor,
		patch: patch,
	}
}

func TestConstraintCheck(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		check      bool
	}{
		{"= 2.0", "1.2.3", false},
		{"= 2.0", "2.0.0", true},
		{"4.1", "4.1.0", true},
		{"!=4.1", "4.1.0", false},
		{"!=4.1", "5.1.0", true},
		{">1.1", "4.1.0", true},
		{">1.1", "1.1.0", false},
		{"<1.1", "0.1.0", true},
		{"<1.1", "1.1.0", false},
		{"<1.1", "1.1.1", false},
		{">=1.1", "4.1.0", true},
		{">=1.1", "1.1.0", true},
		{">=1.1", "0.0.9", false},
		{"<=1.1", "0.1.0", true},
		{"<=1.1", "1.1.0", true},
		{"<=1.1", "1.1.1", false},
		//{"<2.0.0", "2.0.0-alpha1", false},
		//{"<=2.0.0", "2.0.0-alpha1", true},
	}

	for _, tc := range tests {
		c, err := parseConstraint(tc.constraint)
		if err != nil {
			t.Errorf("err: %s", err)
			continue
		}

		v, err := NewVersion(tc.version)
		if err != nil {
			t.Errorf("err: %s", err)
			continue
		}

		a := c.Admits(v) == nil
		if a != tc.check {
			t.Errorf("Constraint '%s' failing", tc.constraint)
		}
	}
}

func TestNewConstraint(t *testing.T) {
	tests := []struct {
		input string
		c     Constraint
		err   bool
	}{
		{">= 1.1", rangeConstraint{
			min:        newV(1, 1, 0),
			includeMin: true,
		}, false},
		{"2.0", newV(2, 0, 0), false},
		{">= bar", nil, true},
		{"^1.1.0", rangeConstraint{
			min:        newV(1, 1, 0),
			max:        newV(2, 0, 0),
			includeMin: true,
		}, false},
		{">= 1.2.3, < 2.0 || => 3.0, < 4", unionConstraint{
			rangeConstraint{
				min:        newV(1, 2, 3),
				max:        newV(2, 0, 0),
				includeMin: true,
			},
			rangeConstraint{
				min:        newV(3, 0, 0),
				max:        newV(4, 0, 0),
				includeMin: true,
			},
		}, false},
		{"3-4 || => 1.0, < 2", Union(
			rangeConstraint{
				min:        newV(3, 0, 0),
				max:        newV(4, 0, 0),
				includeMin: true,
				includeMax: true,
			},
			rangeConstraint{
				min:        newV(1, 0, 0),
				max:        newV(2, 0, 0),
				includeMin: true,
			},
		), false},
		// demonstrates union compression
		{"3-4 || => 3.0, < 4", rangeConstraint{
			min:        newV(3, 0, 0),
			max:        newV(4, 0, 0),
			includeMin: true,
			includeMax: true,
		}, false},
		{">=1.1.0, <2.0.0", rangeConstraint{
			min:        newV(1, 1, 0),
			max:        newV(2, 0, 0),
			includeMin: true,
			includeMax: false,
		}, false},
		{"!=1.4.0", rangeConstraint{
			excl: []*Version{
				newV(1, 4, 0),
			},
		}, false},
		{">=1.1.0, !=1.4.0", rangeConstraint{
			min:        newV(1, 1, 0),
			includeMin: true,
			excl: []*Version{
				newV(1, 4, 0),
			},
		}, false},
	}

	for _, tc := range tests {
		c, err := NewConstraint(tc.input)
		if tc.err && err == nil {
			t.Errorf("expected but did not get error for: %s", tc.input)
			continue
		} else if !tc.err && err != nil {
			t.Errorf("unexpectederror for input %s: %s", tc.input, err)
			continue
		}
		if tc.err {
			continue
		}

		if !constraintEq(tc.c, c) {
			t.Errorf("%q produced constraint %q, but expected %q", tc.input, c, tc.c)
		}
	}
}

func TestConstraintsCheck(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		check      bool
	}{
		{"*", "1.2.3", true},
		{"~0.0.0", "1.2.3", false}, // npm allows this weird thing, but we don't
		{"~0.0.0", "0.1.9", false},
		{"~0.0.0", "0.0.9", true},
		{"= 2.0", "1.2.3", false},
		{"= 2.0", "2.0.0", true},
		{"4.1", "4.1.0", true},
		{"4.1.x", "4.1.3", true},
		{"1.x", "1.4", true},
		{"!=4.1", "4.1.0", false},
		{"!=4.1", "5.1.0", true},
		{"!=4.x", "5.1.0", true},
		{"!=4.x", "4.1.0", false},
		{"!=4.1.x", "4.2.0", true},
		{"!=4.2.x", "4.2.3", false},
		{">1.1", "4.1.0", true},
		{">1.1", "1.1.0", false},
		{"<1.1", "0.1.0", true},
		{"<1.1", "1.1.0", false},
		{"<1.1", "1.1.1", false},
		{"<1.x", "1.1.1", true},
		{"<1.x", "2.1.1", false},
		{"<1.1.x", "1.2.1", false},
		{"<1.1.x", "1.1.500", true},
		{"<1.2.x", "1.1.1", true},
		{">=1.1", "4.1.0", true},
		{">=1.1", "1.1.0", true},
		{">=1.1", "0.0.9", false},
		{"<=1.1", "0.1.0", true},
		{"<=1.1", "1.1.0", true},
		{"<=1.x", "1.1.0", true},
		{"<=2.x", "3.1.0", false},
		{"<=1.1", "1.1.1", false},
		{"<=1.1.x", "1.2.500", false},
		{">1.1, <2", "1.1.1", true},
		{">1.1, <3", "4.3.2", false},
		{">=1.1, <2, !=1.2.3", "1.2.3", false},
		{">=1.1, <2, !=1.2.3 || > 3", "3.1.2", true},
		{">=1.1, <2, !=1.2.3 || >= 3", "3.0.0", true},
		{">=1.1, <2, !=1.2.3 || > 3", "3.0.0", false},
		{">=1.1, <2, !=1.2.3 || > 3", "1.2.3", false},
		{"1.1 - 2", "1.1.1", true},
		{"1.1-3", "4.3.2", false},
		{"^1.1", "1.1.1", true},
		{"^1.1", "4.3.2", false},
		{"^1.x", "1.1.1", true},
		{"^2.x", "1.1.1", false},
		{"^1.x", "2.1.1", false},
		{"~*", "2.1.1", true},
		{"~1.x", "2.1.1", false},
		{"~1.x", "1.3.5", true},
		{"~1.x", "1.4", true},
		{"~1.1", "1.1.1", true},
		{"~1.2.3", "1.2.5", true},
		{"~1.2.3", "1.2.2", false},
		{"~1.2.3", "1.3.2", false},
		{"~1.1", "1.2.3", false},
		{"~1.3", "2.4.5", false},
	}

	for _, tc := range tests {
		c, err := NewConstraint(tc.constraint)
		if err != nil {
			t.Errorf("err: %s", err)
			continue
		}

		v, err := NewVersion(tc.version)
		if err != nil {
			t.Errorf("err: %s", err)
			continue
		}

		a := c.Admits(v) == nil
		if a != tc.check {
			if a {
				t.Errorf("Input %q produced constraint %q; should not have admitted %q, but did", tc.constraint, c, tc.version)
			} else {
				t.Errorf("Input %q produced constraint %q; should have admitted %q, but did not", tc.constraint, c, tc.version)
			}
		}
	}
}

func TestRewriteRange(t *testing.T) {
	tests := []struct {
		c  string
		nc string
	}{
		{"2-3", ">= 2, <= 3"},
		{"2-3, 2-3", ">= 2, <= 3,>= 2, <= 3"},
		{"2-3, 4.0.0-5.1", ">= 2, <= 3,>= 4.0.0, <= 5.1"},
	}

	for _, tc := range tests {
		o := rewriteRange(tc.c)

		if o != tc.nc {
			t.Errorf("Range %s rewritten incorrectly as '%s'", tc.c, o)
		}
	}
}

func TestIsX(t *testing.T) {
	tests := []struct {
		t string
		c bool
	}{
		{"A", false},
		{"%", false},
		{"X", true},
		{"x", true},
		{"*", true},
	}

	for _, tc := range tests {
		a := isX(tc.t)
		if a != tc.c {
			t.Errorf("Function isX error on %s", tc.t)
		}
	}
}
