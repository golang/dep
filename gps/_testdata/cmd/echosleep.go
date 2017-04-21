package main

import (
	"flag"
	"fmt"
	"time"
)

func main() {
	n := flag.Int("n", 1, "number of iterations before stopping")
	flag.Parse()

	for i := 0; i < *n; i++ {
		fmt.Println("foo")
		time.Sleep(time.Duration(i) * 250 * time.Millisecond)
	}
}
