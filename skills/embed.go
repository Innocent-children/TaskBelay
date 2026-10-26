package skills

import "embed"

//go:embed taskbelay-write-a-well-document taskbelay-write-technical-plans
var Files embed.FS

var Names = []string{"taskbelay-write-a-well-document", "taskbelay-write-technical-plans"}
