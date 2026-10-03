package workflow

import (
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

type GraphNode struct {
	NodeID  domain.NodeID
	Kind    string
	Purpose string
}

type GraphTransition struct {
	TransitionID domain.TransitionID
	Source       domain.NodeID
	Destination  domain.NodeID
}

type CommittedTraversal struct {
	Revision     uint64
	Kind         domain.OperationKind
	Source       domain.NodeID
	Destination  domain.NodeID
	TransitionID *domain.TransitionID
	Reason       string
	CreatedAt    time.Time
}

type ControlCenterGraph struct {
	Safe                      bool
	Process                   domain.ProcessReference
	TaskRevision              uint64
	CurrentNode               domain.NodeID
	ResumeNode                *domain.NodeID
	Nodes                     []GraphNode
	Transitions               []GraphTransition
	Traversals                []CommittedTraversal
	CurrentLegalTransitionIDs []domain.TransitionID
	FutureNodeIDs             []domain.NodeID
	FutureTransitionIDs       []domain.TransitionID
}

// ProjectControlCenterGraph combines Core definition, Action and committed-event facts without persisting a graph cursor.
func ProjectControlCenterGraph(task domain.ProcessTask, traversals []CommittedTraversal) ControlCenterGraph {
	projection := ControlCenterGraph{
		Process:      task.Process,
		TaskRevision: task.Revision,
		CurrentNode:  task.CurrentNode,
		ResumeNode:   cloneNodeID(task.ResumeNode),
		Traversals:   append([]CommittedTraversal(nil), traversals...),
	}
	definition, err := ResolveDefinition(task.Process)
	if err != nil {
		return projection
	}
	projection.Nodes = make([]GraphNode, len(definition.Nodes))
	for index, node := range definition.Nodes {
		kind := "normal"
		if node.NodeID.Terminal() {
			kind = "terminal"
		} else if node.NodeID == domain.NodeBlocked {
			kind = "blocked"
		}
		projection.Nodes[index] = GraphNode{NodeID: node.NodeID, Kind: kind, Purpose: node.Purpose}
	}
	projection.Transitions = make([]GraphTransition, len(definition.Transitions))
	for index, transition := range definition.Transitions {
		projection.Transitions[index] = GraphTransition{TransitionID: transition.TransitionID, Source: transition.Source, Destination: transition.Destination}
	}
	if !consistentTraversalHistory(task, definition, traversals) {
		return projection
	}
	if task.CurrentAction != nil {
		projection.CurrentLegalTransitionIDs = make([]domain.TransitionID, len(task.CurrentAction.AvailableTransitions))
		for index, transition := range task.CurrentAction.AvailableTransitions {
			projection.CurrentLegalTransitionIDs[index] = transition.TransitionID
		}
	}
	projection.FutureNodeIDs, projection.FutureTransitionIDs = reachableGraph(definition, task)
	projection.Safe = true
	return projection
}

func consistentTraversalHistory(task domain.ProcessTask, definition domain.ProcessDefinition, traversals []CommittedTraversal) bool {
	if task.Revision == 0 || len(traversals) != int(task.Revision) {
		return false
	}
	cursor := definition.EntryNode
	for index, traversal := range traversals {
		if traversal.Revision != uint64(index+1) || !traversal.Kind.IsValid() || !traversal.Source.IsValid() || !traversal.Destination.IsValid() || traversal.CreatedAt.IsZero() || traversal.Source != cursor {
			return false
		}
		if traversal.TransitionID != nil {
			transition, err := TransitionFor(definition, traversal.Source, *traversal.TransitionID)
			if err != nil || transition.Destination != traversal.Destination || transition.ReasonRequired != (traversal.Reason != "") {
				return false
			}
		} else if !exceptionalTraversal(index, traversal) {
			return false
		}
		cursor = traversal.Destination
	}
	return cursor == task.CurrentNode
}

func exceptionalTraversal(index int, traversal CommittedTraversal) bool {
	if index == 0 {
		return traversal.Kind == domain.OperationOpenTask && traversal.Source == domain.NodeRequirements && traversal.Destination == domain.NodeRequirements
	}
	if traversal.Kind == domain.OperationCancelTask || traversal.Kind == domain.OperationAbandonTask {
		return traversal.Destination == domain.NodeCancelled
	}
	if traversal.Kind == domain.OperationPrepareTaskBranchRename || traversal.Kind == domain.OperationPrepareFileChange || traversal.Kind == domain.OperationPrepareTaskRelocation {
		return traversal.Source.Normal() && traversal.Destination == domain.NodeBlocked
	}
	if traversal.Kind == domain.OperationObserveWorkspace {
		return traversal.Source.Normal() && (traversal.Destination == domain.NodeBlocked || traversal.Destination == domain.NodeImplement) ||
			traversal.Source == domain.NodeBlocked && traversal.Destination == domain.NodeBlocked
	}
	if traversal.Kind != domain.OperationApplyAction {
		return false
	}
	if traversal.Destination == domain.NodeBlocked {
		return traversal.Source.Normal()
	}
	return traversal.Source == domain.NodeBlocked && traversal.Destination.Normal()
}

func reachableGraph(definition domain.ProcessDefinition, task domain.ProcessTask) ([]domain.NodeID, []domain.TransitionID) {
	if task.CurrentNode.Terminal() {
		return []domain.NodeID{}, []domain.TransitionID{}
	}
	start := task.CurrentNode
	if start == domain.NodeBlocked {
		if task.ResumeNode == nil {
			return []domain.NodeID{}, []domain.TransitionID{}
		}
		start = *task.ResumeNode
	}
	outgoing := make(map[domain.NodeID][]domain.TransitionDefinition, len(definition.Nodes))
	for _, transition := range definition.Transitions {
		outgoing[transition.Source] = append(outgoing[transition.Source], transition)
	}
	visitedNodes := map[domain.NodeID]bool{start: true}
	visitedTransitions := map[domain.TransitionID]bool{}
	queue := []domain.NodeID{start}
	nodes := make([]domain.NodeID, 0)
	transitions := make([]domain.TransitionID, 0)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, transition := range outgoing[node] {
			if !visitedTransitions[transition.TransitionID] {
				visitedTransitions[transition.TransitionID] = true
				transitions = append(transitions, transition.TransitionID)
			}
			if visitedNodes[transition.Destination] {
				continue
			}
			visitedNodes[transition.Destination] = true
			nodes = append(nodes, transition.Destination)
			queue = append(queue, transition.Destination)
		}
	}
	return nodes, transitions
}

func cloneNodeID(value *domain.NodeID) *domain.NodeID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
