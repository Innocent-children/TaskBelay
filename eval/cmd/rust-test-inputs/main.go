// Command rust-test-inputs describes Rust test ranges for isolated evaluation copies.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Innocent-children/taskbelay/eval/internal/rustinputs"
)

func main() {
	var files []rustinputs.Input
	if err := json.NewDecoder(os.Stdin).Decode(&files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	results := make([]rustinputs.Output, 0, len(files))
	for _, source := range files {
		results = append(results, rustinputs.Describe(source))
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
