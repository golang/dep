package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFromNodeWithFile(t *testing.T) {
	actual, err := HashFromNode("", "./testdata/blob")
	if err != nil {
		t.Fatal(err)
	}
	expected := "bf7c45881248f74466f9624e8336747277d7901a4f7af43940be07c5539b78a8"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

func TestHashFromNodeWithDirectory(t *testing.T) {
	actual, err := HashFromNode("../fs", "testdata/recursive")
	if err != nil {
		t.Fatal(err)
	}
	expected := "d5ac28114417eae59b9ac02e3fac5bdff673e93cc91b408cde1989e1cd2efbd0"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

var goSource = filepath.Join(os.Getenv("GOPATH"), "src")

func BenchmarkHashFromNode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := HashFromNode("", goSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}
