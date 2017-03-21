package gps

// build windows

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

	t.Run("one above", testcase(`C:\d1\file`, `C:\d1\d2`, false))
	t.Run("identical", testcase(`C:\d1\d2`, `C:\d1\d2`, false))
	t.Run("one below", testcase(`C:\d1\d2\d3\file`, `C:\d1\d2`, true))
	t.Run("two below", testcase(`C:\d1\d2\d3\d4\file`, `C:\d1\d2`, true))
	t.Run("root", testcase(`C:\d1\file`, `C:\`, true))
	t.Run("both root", testcase(`C:\`, `C:\`, true))
	t.Run("trailing slash", testcase(`C:\d1\file\`, `C:\d1`, true))
	t.Run("different volume", testcase(`C:\d1\file`, `D:\`, false))
}
