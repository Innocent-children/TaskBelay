package domain

// BaselineHistory is retained in full. Page limits apply only to read projections.
type BaselineHistoryQuery struct {
	Revision uint64 `json:"revision"`
	After    uint64 `json:"after"`
	Limit    int    `json:"limit"`
}

type BaselineHistoryEntry struct {
	Sequence  uint64            `json:"sequence"`
	Reference BaselineReference `json:"reference"`
}

type BaselineHistoryPage struct {
	TaskID    ID                     `json:"task_id"`
	Revision  uint64                 `json:"revision"`
	Total     uint64                 `json:"total"`
	NextAfter *uint64                `json:"next_after"`
	Entries   []BaselineHistoryEntry `json:"entries"`
}

func (t *ProcessTask) AppendBaselineHistory(ref BaselineReference) error {
	if ref.Validate() != nil {
		return WithExplanation(ErrInvalidArgument, "The prior baseline reference is invalid.")
	}
	var highest uint32
	for _, prior := range t.BaselineHistory {
		if prior.Kind != ref.Kind {
			continue
		}
		if prior.Revision == ref.Revision {
			if prior != ref {
				return WithExplanation(ErrInvalidArgument, "The baseline revision already has a different saved reference.")
			}
			return nil
		}
		if prior.Revision > highest {
			highest = prior.Revision
		}
	}
	if highest == ^uint32(0) || ref.Revision != highest+1 {
		return WithExplanation(ErrInvalidArgument, "The baseline reference must continue its kind's revision chain.")
	}
	t.BaselineHistory = append(t.BaselineHistory, ref)
	return nil
}

// ReadBaselineHistory pages the exact saved array without changing its retention.
// Every cursor belongs to one Task revision; byte shrinking never skips a reference.
func (t ProcessTask) ReadBaselineHistory(query BaselineHistoryQuery) (BaselineHistoryPage, error) {
	if query.Limit < 1 || query.Limit > MaxBaselineHistoryPageEntries || query.After != 0 && query.Revision == 0 {
		return BaselineHistoryPage{}, WithExplanation(ErrInvalidArgument, "History requires limit 1..32 and the returned Task revision when after is nonzero.")
	}
	if query.Revision != 0 && query.Revision != t.Revision {
		return BaselineHistoryPage{}, WithExplanation(ErrRevisionConflict, "The Task changed between history pages; restart from after=0 with its current revision.")
	}
	page := BaselineHistoryPage{TaskID: t.TaskID, Revision: t.Revision, Total: uint64(len(t.BaselineHistory)), Entries: []BaselineHistoryEntry{}}
	if query.After > page.Total {
		return BaselineHistoryPage{}, WithExplanation(ErrInvalidArgument, "The history cursor exceeds the saved history count.")
	}
	for sequence := query.After; sequence < page.Total && len(page.Entries) < query.Limit; sequence++ {
		page.Entries = append(page.Entries, BaselineHistoryEntry{Sequence: sequence + 1, Reference: t.BaselineHistory[sequence]})
	}
	for {
		page.NextAfter = nil
		if after := query.After + uint64(len(page.Entries)); after < page.Total {
			page.NextAfter = &after
		}
		size, err := compactJSONSize(page)
		if err != nil {
			return BaselineHistoryPage{}, err
		}
		if size <= MaxBaselineHistoryPageBytes {
			return page, nil
		}
		if len(page.Entries) <= 1 {
			return BaselineHistoryPage{}, WithExplanation(ErrInvalidArgument, "The saved baseline reference exceeds the history page byte limit.")
		}
		page.Entries = page.Entries[:len(page.Entries)-1]
	}
}
