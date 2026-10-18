import Foundation

/// What one read produced, after the connection layer already classified the
/// service state. Display priority is applied to these cases in order.
enum DisplayInput: Equatable {
    /// The service is unreachable, the identity does not match, or a read
    /// failed. The desktop shows the disconnected state and the last record.
    case disconnected
    /// The service is readable but no Task is selected.
    case noSelection
    /// HTTP 404: the selected Task is unavailable, which is not a disconnect.
    case taskMissing
    /// A readable Task with the detail readiness Core reports.
    case task(DesktopTaskSummary, detailReadiness: Readiness)
}

/// The displayed stage. Core owns lifecycle and terminal classification; these
/// cases only name what the desktop shows and which posture it uses.
enum DisplayPhase: Equatable {
    case disconnected
    case noSelection
    case taskMissing
    case archived
    case working(node: String)
    case blocked(node: String)
    case completed
    case cancelled
}

/// Pure display and animation-trigger rules. Prompts are in-memory display
/// events; they are never written to Core or to preferences.
enum PresentationRules {
    static func permitsIdleActivities(_ phase: DisplayPhase) -> Bool {
        switch phase {
        case .noSelection, .completed, .cancelled, .archived: return true
        default: return false
        }
    }

    static func needsTerminalRest(_ phase: DisplayPhase) -> Bool {
        switch phase {
        case .completed, .cancelled, .archived: return true
        default: return false
        }
    }

    struct Result: Equatable {
        let phase: DisplayPhase
        let clip: AnimationClip
        /// Play the one-shot attention segment of the clip once.
        let playIntro: Bool
        /// Show the clip's dedicated static frame instead of looping.
        let useRestFrame: Bool
        let summary: DesktopTaskSummary?
        /// The summary comes from the previous successful read because the
        /// service is currently unreachable.
        let isStaleSummary: Bool
        let detailReadiness: Readiness?
    }

    /// The comparison basis for one Task read.
    struct Snapshot: Equatable {
        let taskID: String
        let lifecycle: TaskLifecycle
        let currentNode: String
        let blockerText: String?

        init(summary: DesktopTaskSummary) {
            taskID = summary.taskID
            lifecycle = summary.lifecycle
            currentNode = summary.currentNode
            blockerText = summary.blocker
        }
    }

    /// Evaluates one read against the previous snapshot of the same Task.
    ///
    /// `continuousObservation` is true only when the desktop stayed online on
    /// the same connection and the same Task since the previous successful
    /// read. Starting, switching Tasks, showing the window again, waking from
    /// sleep, and reconnecting all produce a first read that establishes the
    /// current picture without replaying a historical prompt.
    ///
    /// `previousPhase` decides whether a disconnect is newly entered, because a
    /// continuing disconnect must not repeat its attention prompt.
    static func evaluate(
        input: DisplayInput,
        previous: Snapshot?,
        continuousObservation: Bool,
        previousPhase: DisplayPhase,
        lastKnownSummary: DesktopTaskSummary?,
        lastKnownReadiness: Readiness?
    ) -> Result {
        switch input {
        case .disconnected:
            return Result(
                phase: .disconnected,
                clip: .disconnected,
                playIntro: previousPhase != .disconnected,
                useRestFrame: false,
                summary: lastKnownSummary,
                isStaleSummary: lastKnownSummary != nil,
                detailReadiness: nil
            )
        case .noSelection:
            return Result(
                phase: .noSelection,
                clip: .idle,
                playIntro: false,
                useRestFrame: false,
                summary: nil,
                isStaleSummary: false,
                detailReadiness: nil
            )
        case .taskMissing:
            return Result(
                phase: .taskMissing,
                clip: .idle,
                playIntro: false,
                useRestFrame: true,
                summary: nil,
                isStaleSummary: false,
                detailReadiness: nil
            )
        case .task(let summary, let detailReadiness):
            if summary.archived {
                return Result(
                    phase: .archived,
                    clip: .idle,
                    playIntro: false,
                    useRestFrame: true,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            }
            let sameTask = previous?.taskID == summary.taskID
            let observedChange = continuousObservation && sameTask
            switch summary.lifecycle {
            case .done:
                let celebrate = observedChange && previous?.lifecycle.isTerminal == false
                return Result(
                    phase: .completed,
                    clip: .complete,
                    playIntro: celebrate,
                    useRestFrame: !celebrate,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            case .cancelled:
                return Result(
                    phase: .cancelled,
                    clip: .idle,
                    playIntro: false,
                    useRestFrame: true,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            case .blocked:
                let enteredBlocked = previous?.lifecycle != .blocked
                let blockerTextChanged = previous?.blockerText != summary.blocker
                let alert = observedChange && (enteredBlocked || blockerTextChanged)
                return Result(
                    phase: .blocked(node: summary.currentNode),
                    clip: .blocked,
                    playIntro: alert,
                    useRestFrame: false,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            case .active:
                return Result(
                    phase: .working(node: summary.currentNode),
                    clip: .working,
                    playIntro: false,
                    useRestFrame: false,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            case .unknown:
                return Result(
                    phase: .working(node: summary.currentNode),
                    clip: .working,
                    playIntro: false,
                    useRestFrame: false,
                    summary: summary,
                    isStaleSummary: false,
                    detailReadiness: detailReadiness
                )
            }
        }
    }
}

/// Holds the in-memory comparison basis between reads. Nothing here is
/// persisted; preferences only store position, the animation switch, and the
/// selected Task identifiers.
final class PresentationState {
    private(set) var result: PresentationRules.Result
    private var snapshot: PresentationRules.Snapshot?
    private var lastKnownSummary: DesktopTaskSummary?
    private var lastKnownReadiness: Readiness?
    private var continuousObservation = false
    private(set) var lastSyncAt: Date?
    private let now: () -> Date

    init(now: @escaping () -> Date = Date.init) {
        self.now = now
        result = PresentationRules.evaluate(
            input: .noSelection,
            previous: nil,
            continuousObservation: false,
            previousPhase: .noSelection,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
    }

    /// Clears the basis for the previous round of prompts. Starting, switching
    /// Tasks, showing the window again, waking, and reconnecting all call this
    /// so the next read only establishes the current picture.
    func noteDiscontinuity() {
        continuousObservation = false
        snapshot = nil
    }

    /// Forgets the retained last record, used when the selected Task changes so
    /// a disconnected view cannot show another Task's information.
    func discardLastKnownSummary() {
        lastKnownSummary = nil
        lastKnownReadiness = nil
    }

    func consumePrompt() {
        result = PresentationRules.Result(phase: result.phase, clip: result.clip, playIntro: false,
            useRestFrame: result.useRestFrame, summary: result.summary, isStaleSummary: result.isStaleSummary,
            detailReadiness: result.detailReadiness)
    }

    @discardableResult
    func apply(_ input: DisplayInput) -> PresentationRules.Result {
        let evaluated = PresentationRules.evaluate(
            input: input,
            previous: snapshot,
            continuousObservation: continuousObservation,
            previousPhase: result.phase,
            lastKnownSummary: lastKnownSummary,
            lastKnownReadiness: lastKnownReadiness
        )
        switch input {
        case .task(let summary, let detailReadiness):
            snapshot = PresentationRules.Snapshot(summary: summary)
            lastKnownSummary = summary
            lastKnownReadiness = detailReadiness
            lastSyncAt = now()
            continuousObservation = true
        case .disconnected:
            snapshot = nil
            continuousObservation = false
            lastSyncAt = now()
        case .noSelection, .taskMissing:
            snapshot = nil
            continuousObservation = false
            lastKnownSummary = nil
            lastKnownReadiness = nil
            lastSyncAt = now()
        }
        result = evaluated
        return evaluated
    }
}
