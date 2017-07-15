package fs

import (
	"testing"
)

func TestHashFromPathnameWithFile(t *testing.T) {
	actual, err := HashFromPathname("testdata/blob")
	if err != nil {
		t.Fatal(err)
	}
	expected := "825dc11fe41d8f604ab48a8cd6cecf304005bd82fd0228a6e411e992d4d03a08"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

func TestHashFromPathnameWithDirectory(t *testing.T) {
	actual, err := HashFromPathname("testdata/recursive")
	if err != nil {
		t.Fatal(err)
	}
	expected := "9b3a1f1f63c0c54860799cc5464a3c380a697a3ec49ca103a62d9c09ad9fedf8"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}
