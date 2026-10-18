package protocol

import (
	_ "embed"
	"encoding/json"
)

//go:embed examples.json
var exampleData []byte

func methodExamples(name string) []ContractExample {
	var records map[string][]ContractExample
	if json.Unmarshal(exampleData, &records) != nil {
		panic("invalid bundled method examples")
	}
	return append([]ContractExample{}, records[name]...)
}
