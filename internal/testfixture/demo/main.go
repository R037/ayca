// Command demo writes the shared test fixture to a directory so ayca can be
// tried against it: go run ./internal/testfixture/demo <dir>
package main

import (
	"fmt"
	"os"

	"github.com/R037/ayca/internal/testfixture"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: demo <dir>")
		os.Exit(2)
	}
	f, err := testfixture.Write(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
	fmt.Printf("Fixture written to %s\n\nTry it:\n  HOME=%s ayca --path %s\n", f.Root, f.Home, f.Project)
}
