package protocol

import (
	"embed"
	"encoding/json"
)

//go:embed runtime-contracts.json
var runtimeContracts embed.FS

/**
 * Native/HTTP descriptions are generated from isolated producer fixtures and
 * embedded in the executable. Discovery never opens storage or executes them.
 */
func auxiliaryContracts() []MethodContract {
	raw, err := runtimeContracts.ReadFile("runtime-contracts.json")
	if err != nil {
		panic(err)
	}
	var contracts []MethodContract
	if err := json.Unmarshal(raw, &contracts); err != nil {
		panic(err)
	}
	return contracts
}
