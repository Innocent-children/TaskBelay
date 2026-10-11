import Foundation
import XCTest
@testable import TaskBelayPet

/// Covers display priority and animation triggering.
///
/// Core owns lifecycle and terminal classification. These checks confirm that
/// the desktop only decides wording and posture, that a celebration happens once
/// and only after continuous observation of the same Task, and that a first read
/// never replays a historical prompt.
final class PresentationRulesTests: XCTestCase {
    func testActiveTaskKeepsWorkingArtwork() {
        let state = PresentationState()
        let review = state.apply(.task(TestFixtures.summary(currentNode: "ACTIVE", lifecycle: .active), detailReadiness: .ready))
        XCTAssertEqual(review.clip, .working)
        XCTAssertEqual(review.phase, .working(node: "ACTIVE"))
        let working = state.apply(.task(TestFixtures.summary(currentNode: "ACTIVE", lifecycle: .active), detailReadiness: .ready))
        XCTAssertEqual(working.clip, .working)
    }

    func testDisconnectedHasPriorityOverEveryOtherInput() {
        let result = PresentationRules.evaluate(
            input: .disconnected,
            previous: PresentationRules.Snapshot(summary: TestFixtures.summary(lifecycle: .done)),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: TestFixtures.summary(),
            lastKnownReadiness: .ready
        )
        XCTAssertEqual(result.phase, .disconnected)
        XCTAssertEqual(result.clip, .disconnected)
        XCTAssertTrue(result.playIntro)
        XCTAssertTrue(result.isStaleSummary)
        XCTAssertNil(result.detailReadiness)
    }

    func testContinuingDisconnectDoesNotRepeatItsPrompt() {
        let result = PresentationRules.evaluate(
            input: .disconnected,
            previous: nil,
            continuousObservation: false,
            previousPhase: .disconnected,
            lastKnownSummary: TestFixtures.summary(),
            lastKnownReadiness: .ready
        )
        XCTAssertFalse(result.playIntro)
    }

    func testDisconnectWithoutAnyRecordKeepsTheSummaryEmpty() {
        let result = PresentationRules.evaluate(
            input: .disconnected,
            previous: nil,
            continuousObservation: false,
            previousPhase: .noSelection,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertNil(result.summary)
        XCTAssertFalse(result.isStaleSummary)
    }

    func testNoSelectionIsIdleWithoutAPrompt() {
        let result = PresentationRules.evaluate(
            input: .noSelection,
            previous: nil,
            continuousObservation: false,
            previousPhase: .disconnected,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.phase, .noSelection)
        XCTAssertEqual(result.clip, .idle)
        XCTAssertFalse(result.playIntro)
        XCTAssertFalse(result.useRestFrame)
    }

    func testMissingTaskUsesTheStaticIdlePosture() {
        let result = PresentationRules.evaluate(
            input: .taskMissing,
            previous: nil,
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.phase, .taskMissing)
        XCTAssertEqual(result.clip, .idle)
        XCTAssertTrue(result.useRestFrame)
        XCTAssertNil(result.summary)
    }

    func testArchivedTaskBeatsItsCurrentNode() {
        let summary = TestFixtures.summary(currentNode: "ACTIVE", lifecycle: .done, archived: true)
        let result = PresentationRules.evaluate(
            input: .task(summary, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: TestFixtures.summary(lifecycle: .active)),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.phase, .archived)
        XCTAssertEqual(result.clip, .idle)
        XCTAssertTrue(result.useRestFrame)
        XCTAssertFalse(result.playIntro)
    }

    func testCelebratesOnceWhenContinuouslyObservedEnteringDone() {
        let previous = TestFixtures.summary(taskID: "task-1", lifecycle: .active)
        let done = TestFixtures.summary(taskID: "task-1", currentNode: "DONE", lifecycle: .done)
        let celebrated = PresentationRules.evaluate(
            input: .task(done, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: previous),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(celebrated.phase, .completed)
        XCTAssertEqual(celebrated.clip, .complete)
        XCTAssertTrue(celebrated.playIntro)
        XCTAssertFalse(celebrated.useRestFrame)
    }

    func testCelebratesWhenEnteringDoneFromBlocked() {
        let blocked = TestFixtures.summary(taskID: "task-1", lifecycle: .blocked, blocker: "needs a decision")
        let done = TestFixtures.summary(taskID: "task-1", currentNode: "DONE", lifecycle: .done)
        let result = PresentationRules.evaluate(
            input: .task(done, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: blocked),
            continuousObservation: true,
            previousPhase: .blocked(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertTrue(result.playIntro)
    }

    func testFirstReadOfDoneShowsTheStaticCompletionPosture() {
        let done = TestFixtures.summary(taskID: "task-1", currentNode: "DONE", lifecycle: .done)
        let cases: [(previous: PresentationRules.Snapshot?, continuous: Bool)] = [
            (nil, false),
            (nil, true),
            (PresentationRules.Snapshot(summary: TestFixtures.summary(lifecycle: .active)), false),
        ]
        for (previous, continuous) in cases {
            let result = PresentationRules.evaluate(
                input: .task(done, detailReadiness: .ready),
                previous: previous,
                continuousObservation: continuous,
                previousPhase: .noSelection,
                lastKnownSummary: nil,
                lastKnownReadiness: nil
            )
            XCTAssertEqual(result.phase, .completed)
            XCTAssertFalse(result.playIntro, "a first read must not replay a historical celebration")
            XCTAssertTrue(result.useRestFrame)
        }
    }

    func testDoesNotCelebrateAnotherTaskReachingDone() {
        let other = TestFixtures.summary(taskID: "task-other", lifecycle: .active)
        let done = TestFixtures.summary(taskID: "task-1", currentNode: "DONE", lifecycle: .done)
        let result = PresentationRules.evaluate(
            input: .task(done, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: other),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertFalse(result.playIntro)
        XCTAssertTrue(result.useRestFrame)
    }

    func testCancelledNeverCelebrates() {
        let result = PresentationRules.evaluate(
            input: .task(TestFixtures.summary(currentNode: "CANCELLED", lifecycle: .cancelled), detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: TestFixtures.summary(lifecycle: .active)),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.phase, .cancelled)
        XCTAssertEqual(result.clip, .idle)
        XCTAssertFalse(result.playIntro)
        XCTAssertTrue(result.useRestFrame)
    }

    func testAlertsOnceOnEnteringBlockedAndOnChangedBlockerText() {
        let previous = TestFixtures.summary(taskID: "task-1", lifecycle: .active)
        let blocked = TestFixtures.summary(taskID: "task-1", currentNode: "BLOCKED", lifecycle: .blocked, blocker: "tests fail")
        let entered = PresentationRules.evaluate(
            input: .task(blocked, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: previous),
            continuousObservation: true,
            previousPhase: .working(node: "ACTIVE"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(entered.phase, .blocked(node: "BLOCKED"))
        XCTAssertEqual(entered.clip, .blocked)
        XCTAssertTrue(entered.playIntro)
        XCTAssertFalse(entered.useRestFrame)

        // A plain revision increase inside the same block does not replay it.
        let sameBlockLater = TestFixtures.summary(
            taskID: "task-1", currentNode: "BLOCKED", lifecycle: .blocked, revision: 9, blocker: "tests fail"
        )
        let unchanged = PresentationRules.evaluate(
            input: .task(sameBlockLater, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: blocked),
            continuousObservation: true,
            previousPhase: .blocked(node: "BLOCKED"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertFalse(unchanged.playIntro)

        let newReason = TestFixtures.summary(
            taskID: "task-1", currentNode: "BLOCKED", lifecycle: .blocked, revision: 10, blocker: "needs a decision"
        )
        let changed = PresentationRules.evaluate(
            input: .task(newReason, detailReadiness: .ready),
            previous: PresentationRules.Snapshot(summary: sameBlockLater),
            continuousObservation: true,
            previousPhase: .blocked(node: "BLOCKED"),
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertTrue(changed.playIntro)
    }

    func testFirstReadOfBlockedTaskEntersTheQuietLoop() {
        let blocked = TestFixtures.summary(currentNode: "BLOCKED", lifecycle: .blocked, blocker: "tests fail")
        let result = PresentationRules.evaluate(
            input: .task(blocked, detailReadiness: .ready),
            previous: nil,
            continuousObservation: false,
            previousPhase: .noSelection,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.clip, .blocked)
        XCTAssertFalse(result.playIntro)
        XCTAssertFalse(result.useRestFrame)
    }

    func testMissingBlockerTextDoesNotInventAReason() {
        let blocked = TestFixtures.summary(currentNode: "BLOCKED", lifecycle: .blocked, blocker: nil)
        let result = PresentationRules.evaluate(
            input: .task(blocked, detailReadiness: .ready),
            previous: nil,
            continuousObservation: false,
            previousPhase: .noSelection,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertNil(result.summary?.blocker)
        let content = BubbleRules.content(
            result: result,
            lastSyncAt: nil,
            strings: .english,
            language: .english
        )
        XCTAssertEqual(content.blocker, PetStrings.english.blockedFallback)
    }

    func testActiveRefreshDoesNotReplayAnIntro() {
        let state = PresentationState()
        state.apply(.task(TestFixtures.summary(currentNode: "ACTIVE", lifecycle: .active), detailReadiness: .ready))
        let regressed = state.apply(.task(TestFixtures.summary(currentNode: "ACTIVE", lifecycle: .active), detailReadiness: .ready))
        XCTAssertEqual(regressed.phase, .working(node: "ACTIVE"))
        XCTAssertEqual(regressed.clip, .working)
        XCTAssertFalse(regressed.playIntro)
    }

    func testReadOnlyIsAnAdditionalFlagThatKeepsThePhase() {
        let result = PresentationRules.evaluate(
            input: .task(TestFixtures.summary(), detailReadiness: .readOnly),
            previous: nil,
            continuousObservation: false,
            previousPhase: .noSelection,
            lastKnownSummary: nil,
            lastKnownReadiness: nil
        )
        XCTAssertEqual(result.phase, .working(node: "ACTIVE"))
        XCTAssertEqual(result.detailReadiness, .readOnly)
        let content = BubbleRules.content(result: result, lastSyncAt: nil, strings: .english, language: .english)
        XCTAssertEqual(content.stage, "Implementation · \(PetStrings.english.readOnlyHint)")
    }

    func testStateSequenceKeepsTheRetainedRecordForDisconnectOnly() {
        var now = Date(timeIntervalSince1970: 1_800_000_000)
        let state = PresentationState { now }

        let working = state.apply(.task(TestFixtures.summary(), detailReadiness: .ready))
        XCTAssertEqual(working.summary?.taskID, "task-1")
        XCTAssertEqual(state.lastSyncAt, now)

        now = now.addingTimeInterval(5)
        let disconnected = state.apply(.disconnected)
        XCTAssertEqual(disconnected.summary?.taskID, "task-1")
        XCTAssertTrue(disconnected.isStaleSummary)

        // Reconnecting is a first read, so the retained record cannot trigger a
        // celebration for a change nobody observed.
        now = now.addingTimeInterval(15)
        let reconnected = state.apply(.task(TestFixtures.summary(currentNode: "DONE", lifecycle: .done), detailReadiness: .ready))
        XCTAssertFalse(reconnected.playIntro)
        XCTAssertTrue(reconnected.useRestFrame)
        XCTAssertFalse(reconnected.isStaleSummary)
    }

    func testDiscontinuityClearsThePromptBasis() {
        let state = PresentationState()
        state.apply(.task(TestFixtures.summary(lifecycle: .active), detailReadiness: .ready))
        state.apply(.task(TestFixtures.summary(currentNode: "DONE", lifecycle: .done), detailReadiness: .ready))
        XCTAssertTrue(state.result.playIntro)

        state.noteDiscontinuity()
        let afterWake = state.apply(.task(TestFixtures.summary(currentNode: "DONE", lifecycle: .done), detailReadiness: .ready))
        XCTAssertFalse(afterWake.playIntro)
        XCTAssertTrue(afterWake.useRestFrame)
    }

    func testSwitchingTaskDiscardsTheOtherTaskRecord() {
        let state = PresentationState()
        state.apply(.task(TestFixtures.summary(taskID: "task-1"), detailReadiness: .ready))
        state.noteDiscontinuity()
        state.discardLastKnownSummary()
        let disconnected = state.apply(.disconnected)
        XCTAssertNil(disconnected.summary)
        XCTAssertFalse(disconnected.isStaleSummary)
    }

    func testNoSelectionAndMissingTaskDropTheRetainedRecord() {
        let state = PresentationState()
        state.apply(.task(TestFixtures.summary(), detailReadiness: .ready))
        state.apply(.noSelection)
        let disconnected = state.apply(.disconnected)
        XCTAssertNil(disconnected.summary)
    }
}
