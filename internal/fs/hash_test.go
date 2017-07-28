package fs

import (
	"testing"
)

func TestHashFromElementWithFile(t *testing.T) {
	actual, err := HashFromElement("./testdata/blob")
	if err != nil {
		t.Fatal(err)
	}
	expected := "2d1c82d4643e6c95d4a472c5bad4f3f044474fc20c42ced663f446a0b59524cd"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

func TestHashFromElementWithDirectory(t *testing.T) {
	actual, err := HashFromElement("testdata/recursive")
	if err != nil {
		t.Fatal(err)
	}
	expected := "ec272227655cca9517bdcbd27c925c50ae65112a124bc61d0d743a1ed9323d5e"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}
