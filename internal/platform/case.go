package platform

type CaseRule string

const (
	CaseSensitive   CaseRule = "sensitive"
	CaseInsensitive CaseRule = "insensitive"
	CaseUnknown     CaseRule = "unknown"
)
