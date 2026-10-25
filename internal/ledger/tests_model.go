package ledger

type TestAssertion struct {
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
	Line     int    `json:"line"`
}

type TestFact struct {
	At           string          `json:"at"`
	Framework    string          `json:"framework"`
	Declaration  string          `json:"declaration"`
	Change       string          `json:"change"`
	Line         int             `json:"line"`
	Requirements []string        `json:"requirements"`
	RelatedCode  []string        `json:"related_code"`
	Assertions   []TestAssertion `json:"assertions"`
	Unknown      []string        `json:"unknown"`
	AgentOnly    bool            `json:"agent_only"`
	Mixed        bool            `json:"mixed"`
	Retained     bool            `json:"retained"`
	Evidence     []string        `json:"evidence"`
}

type TestRiskSource struct {
	Kind     string `json:"kind"`
	At       string `json:"at"`
	Line     int    `json:"line"`
	Evidence string `json:"evidence"`
}

type TestRisk struct {
	Requirement string           `json:"requirement"`
	Recommended int              `json:"recommended"`
	Added       int              `json:"added"`
	Modified    int              `json:"modified"`
	Existing    int              `json:"existing"`
	Retained    int              `json:"retained_added"`
	Branches    int              `json:"branches"`
	Logic       bool             `json:"logic"`
	Complete    bool             `json:"complete"`
	Sources     []TestRiskSource `json:"sources"`
	Evidence    []string         `json:"evidence"`
}
