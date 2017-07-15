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
	expected := "9ccd71eec554488c99eb9205b6707cb70379e1aa637faad58ac875278786f2ff"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

func TestHashFromNodeWithDirectory(t *testing.T) {
	actual, err := HashFromNode("../fs", "testdata/recursive")
	if err != nil {
		t.Fatal(err)
	}
	expected := "432949ff3f1687e7e09b31fd81ecab3b7d9b68da5f543aad18e1930b5bd22e30"
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
