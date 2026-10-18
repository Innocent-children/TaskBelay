package domain

func (g Goal) RequirementDigest() Digest {
	return Hash(g)
}

/**
 * Verification methods and forecasts belong to the agent. User outcomes and
 * explicit restrictions remain bound to the actual request.
 */
func (a Authorization) BoundaryDigest() Digest {
	a.UserRef = ""
	a.Limits = Limits{}
	return Hash(a)
}

func (a Authorization) RequiresDecision(previous Authorization) bool {
	return a.BoundaryDigest() != previous.BoundaryDigest() || !a.Limits.Within(previous.Limits)
}

/**
 * A forecast never grants permission. Explicit allowed paths and exclusions do.
 */
func (a Authorization) AllowsPath(path string) bool {
	allowed := len(a.AllowedPaths) == 0
	for _, scope := range a.AllowedPaths {
		allowed = allowed || MatchPath(scope, path)
	}
	for _, scope := range a.ForbiddenPaths {
		if MatchPath(scope, path) {
			return false
		}
	}
	return allowed
}
