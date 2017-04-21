package internal

import "testing"

func TestIsStdLib(t *testing.T) {
	fix := []struct {
		ip string
		is bool
	}{
		{"appengine", true},
		{"net/http", true},
		{"github.com/anything", false},
		{"foo", true},
	}

	for _, f := range fix {
		r := doIsStdLib(f.ip)
		if r != f.is {
			if r {
				t.Errorf("%s was marked stdlib but should not have been", f.ip)
			} else {
				t.Errorf("%s was not marked stdlib but should have been", f.ip)

			}
		}
	}
}

