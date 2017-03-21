package gps

// build !windows

import "testing"

func TestInDirectory(t *testing.T) {
	testcase := func(path, dir string, want bool) func(*testing.T) {
		return func(t *testing.T) {
			have := inDirectory(path, dir)
			if have != want {
				t.Fail()
			}
		}
	}

	t.Run("one above", testcase("/d1/file", "/d1/d2", false))
	t.Run("identical", testcase("/d1/d2", "/d1/d2", false))
	t.Run("one below", testcase("/d1/d2/d3/file", "/d1/d2", true))
	t.Run("two below", testcase("/d1/d2/d3/d4/file", "/d1/d2", true))
	t.Run("root", testcase("/d1/file", "/", true))
	t.Run("both root", testcase("/", "/", true))
	t.Run("trailing slash", testcase("/d1/file/", "/d1", true))
}
