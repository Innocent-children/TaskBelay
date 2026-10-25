package skills

import "embed"

//go:embed write-a-well-document write-technical-plans
var Files embed.FS

var Names = []string{"write-a-well-document", "write-technical-plans"}
