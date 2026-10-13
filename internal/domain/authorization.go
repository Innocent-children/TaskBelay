package domain

import "slices"

func (g Goal) RequirementDigest() Digest {
	g.Checks = nil
	return Hash(g)
}

/**
 * Verification methods and forecasts belong to the agent. User outcomes and
 * explicit restrictions remain bound to the actual request.
 */
func (a Authorization) BoundaryDigest() Digest {
	a.UserRef = ""
	a.ExpectedPaths = nil
	a.ReviewAfterLines = 0
	a.Limits = Limits{}
	a.Goals = slices.Clone(a.Goals)
	for i := range a.Goals {
		a.Goals[i].Checks = nil
	}
	return Hash(a)
}

func (a Authorization) RequiresDecision(previous Authorization) bool {
	return a.BoundaryDigest() != previous.BoundaryDigest() || !a.Limits.Within(previous.Limits)
}
