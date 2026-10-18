import Foundation
import XCTest
@testable import TaskBelayPet

/// Covers polling, request attribution, default selection, and connection
/// recovery.
///
/// Connection generation, Task selection generation, and list generation decide
/// which response may be displayed. Background recovery is read-only, and only
/// an explicit retry may ask Core to start the local service.
final class TaskObserverTests: XCTestCase {
    private var directories: [PetTestDirectory] = []
    private var observers: [TaskObserver] = []

    override func tearDown() async throws {
        for observer in observers { await observer.cancel() }
        observers.removeAll()
        directories.removeAll()
        try await super.tearDown()
    }

    // MARK: - Reading the selected Task

    func testPublishesTheCurrentNodeOfTheSelectedTask() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail(taskID: "task-1")))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually("the observer published \(context.collector.count) updates") { context.collector.count >= 2 }

        let last = context.collector.snapshot.last
        XCTAssertEqual(last?.connection, .connected(url: TestFixtures.origin))
        XCTAssertEqual(last?.presentation.phase, .working(node: "ACTIVE"))
        XCTAssertEqual(last?.presentation.clip, .working)
        XCTAssertFalse(last?.presentation.playIntro ?? true)
        XCTAssertEqual(last?.selectedTaskID, "task-1")
        XCTAssertNotNil(last?.lastSyncAt)
    }

    func testTaskNotFoundIsNotADisconnect() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .notFound)
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }

        let last = context.collector.snapshot.last
        XCTAssertEqual(last?.connection, .connected(url: TestFixtures.origin))
        XCTAssertEqual(last?.presentation.phase, .taskMissing)
        XCTAssertTrue(last?.presentation.useRestFrame ?? false)
    }

    // MARK: - Late responses

    func testLateResponseForASupersededTaskIsDropped() async {
        let context = makeContext(selectedTaskID: "task-a")
        context.client.holdDetail(taskID: "task-a")
        context.client.setDetail(taskID: "task-b", .value(TestDetails.detail(taskID: "task-b", currentNode: "ACTIVE")))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.client.openDetailGateCount == 1 }

        await observer.select(taskID: "task-b")
        // Task A reaches DONE while its read is still in flight. Because the
        // selection moved on, the desktop must not show it and must not
        // celebrate a change nobody observed.
        context.client.releaseDetailGate(.value(TestDetails.detail(taskID: "task-a", currentNode: "DONE", lifecycle: .done)))

        await assertEventually { context.collector.snapshot.contains { $0.selectedTaskID == "task-b" } }
        // The dropped response publishes nothing, so the connected notice and the
        // Task B read are the only two updates.
        await assertEventually { context.collector.count >= 2 }
        let phases = context.collector.snapshot.map(\.presentation.phase)
        XCTAssertFalse(phases.contains(.completed), "the late response for task-a was displayed")
        XCTAssertEqual(context.collector.snapshot.last?.presentation.phase, .working(node: "ACTIVE"))
        XCTAssertFalse(context.collector.snapshot.contains { $0.presentation.playIntro })
        XCTAssertEqual(context.client.recordedRequests.filter { $0.hasPrefix("detail:") }, ["detail:task-a", "detail:task-b"])
    }

    func testLateResponseFromASupersededConnectionIsDropped() async {
        let directory = makeDirectory()
        let stale = ScriptedWebUIClient(origin: "http://127.0.0.1:51234")
        stale.setSystemStatus(.value(TestDetails.liveStatus()))
        stale.holdDetail(taskID: "task-1")
        let current = ScriptedWebUIClient(origin: "http://127.0.0.1:60001")
        current.setSystemStatus(.value(TestDetails.liveStatus(url: "http://127.0.0.1:60001")))
        current.setDetail(taskID: "task-1", .value(TestDetails.detail(currentNode: "ACTIVE")))

        // Reconnecting reads the Core status again and finds the service on a
        // new port, which supersedes the connection still holding a read.
        let runner = RecordingProcessRunner(
            outcomes: [readyOutcome(url: TestFixtures.origin), readyOutcome(url: "http://127.0.0.1:60001")],
            defaultOutcome: readyOutcome(url: "http://127.0.0.1:60001")
        )
        let collector = UpdateCollector()
        let factory = ClientSequence([stale, current])
        let observer = makeObserver(
            directory: directory,
            runner: runner,
            factory: factory,
            selectedTaskID: "task-1",
            collector: collector
        )

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { stale.openDetailGateCount == 1 }

        // The service moved to a new port; reconnecting supersedes the old
        // connection while its read is still in flight.
        await observer.retryConnection()
        stale.releaseDetailGate(.value(TestDetails.detail(currentNode: "DONE", lifecycle: .done)))

        await assertEventually {
            collector.snapshot.contains { $0.connection == .connected(url: "http://127.0.0.1:60001") }
        }
        await assertEventually { collector.snapshot.last?.presentation.phase == .working(node: "ACTIVE") }
        let phases = collector.snapshot.map(\.presentation.phase)
        XCTAssertFalse(phases.contains(.completed), "the late response from the old connection was displayed")
        XCTAssertFalse(collector.snapshot.contains { $0.presentation.playIntro })
        XCTAssertEqual(factory.requestedOrigins, ["http://127.0.0.1:51234", "http://127.0.0.1:60001"])
    }

    func testClosedPanelSupersedesItsOwnListSession() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }

        context.client.holdLists()
        let session = await observer.beginListSession()
        let pending = Task { await observer.loadList(page: 1, lifecycle: nil, session: session) }
        await assertEventually { context.client.openListGateCount == 1 }

        // Closing the panel opens a new session, so the page that arrives now
        // belongs to a panel that no longer exists.
        _ = await observer.beginListSession()
        context.client.releaseListGate(.value(TestDetails.list(summaries: [TestFixtures.summary(taskID: "task-late")])))

        let result = await pending.value
        XCTAssertEqual(result, .stale)
    }

    func testSupersededPageOfTheSameSessionIsStale() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }

        let session = await observer.beginListSession()
        let supersededPage = await observer.loadList(page: 1, lifecycle: nil, session: session - 1)
        XCTAssertEqual(supersededPage, .stale)
    }

    // MARK: - Default selection

    func testDefaultSelectionPrefersTheMostRecentlyUpdatedBlockedTask() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "blocked-old", lifecycle: .blocked, revision: 1),
            TestFixtures.summary(taskID: "blocked-new", lifecycle: .blocked, revision: 7),
        ])))
        context.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "active-new", lifecycle: .active, revision: 9),
        ])))
        context.client.setDetail(taskID: "blocked-new", .value(TestDetails.detail(taskID: "blocked-new", lifecycle: .blocked)))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "blocked-new" }

        let listRequests = context.client.recordedRequests.filter { $0.hasPrefix("list:") }
        XCTAssertEqual(listRequests, ["list:blocked:page=1", "list:active:page=1"])
        XCTAssertNil(context.preferences.current.selectedTask(for: TestFixtures.dataRootDigest))
    }

    func testDefaultSelectionFallsBackToActiveAndOtherwiseStaysIdle() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [])))
        context.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "active-only", lifecycle: .active, revision: 2),
        ])))
        context.client.setDetail(taskID: "active-only", .value(TestDetails.detail(taskID: "active-only")))

        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "active-only" }
        XCTAssertEqual(
            context.client.recordedRequests.filter { $0.hasPrefix("list:") },
            ["list:blocked:page=1", "list:active:page=1"]
        )

        let empty = makeContext(selectedTaskID: nil)
        empty.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [])))
        empty.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [])))
        await empty.observer.beginObserving()
        await assertEventually { empty.collector.count >= 2 }
        XCTAssertEqual(empty.collector.snapshot.last?.presentation.phase, .noSelection)
        XCTAssertNil(empty.collector.snapshot.last?.selectedTaskID)
        XCTAssertEqual(empty.collector.snapshot.last?.connection, .connected(url: TestFixtures.origin))
    }

    func testIdlePollingDiscoversANewTaskAndKeepsWatchingIt() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [])))
        context.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [])))
        await context.observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.presentation.phase == .noSelection }

        context.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "created-later", lifecycle: .active),
        ])))
        context.client.setDetail(taskID: "created-later", .value(TestDetails.detail(taskID: "created-later")))
        await context.observer.refreshNow()
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "created-later" }
        XCTAssertNil(context.preferences.current.selectedTask(for: TestFixtures.dataRootDigest))
        XCTAssertEqual(context.collector.snapshot.last?.presentation.phase, .working(node: "ACTIVE"))

        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "new-blocked", lifecycle: .blocked),
        ])))
        let lists = context.client.requestCount("list:")
        await context.observer.refreshNow()
        await assertEventually { context.client.requestCount("list:") > lists }
        let selected = await context.observer.currentSelection()
        XCTAssertEqual(selected, "new-blocked")
        XCTAssertEqual(context.client.requestCount("list:"), lists + 2)
    }

    func testClearingSelectionResumesDiscovery() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "blocked", lifecycle: .blocked),
        ])))
        context.client.setDetail(taskID: "blocked", .value(TestDetails.detail(taskID: "blocked", lifecycle: .blocked)))
        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "task-1" }
        await context.observer.select(taskID: nil)
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "blocked" }
        XCTAssertNil(context.preferences.current.selectedTask(for: TestFixtures.dataRootDigest))
    }

    func testOtherTaskUpdatesDoNotChangeTheWatchedTask() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail(taskID: "task-1")))
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [
            TestFixtures.summary(taskID: "task-other", lifecycle: .blocked, revision: 40),
        ])))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }
        await TestWaiting.pause(0.6)

        let selection = await observer.currentSelection()
        XCTAssertEqual(selection, "task-1")
        XCTAssertFalse(context.client.recordedRequests.contains("detail:task-other"))
        XCTAssertTrue(context.client.recordedRequests.contains { $0.hasPrefix("list:") })
    }

    // MARK: - Connection recovery

    func testReadFailureEntersDisconnectedAndBackgroundRecoveryStaysReadOnly() async {
        let directory = makeDirectory()
        let first = ScriptedWebUIClient(origin: "http://127.0.0.1:51234")
        first.setSystemStatus(.value(TestDetails.liveStatus()))
        first.setDetail(taskID: "task-1", .failure(.unreachable))
        let recovered = ScriptedWebUIClient(origin: "http://127.0.0.1:60001")
        recovered.setSystemStatus(.value(TestDetails.liveStatus(url: "http://127.0.0.1:60001")))
        recovered.setDetail(taskID: "task-1", .value(TestDetails.detail(currentNode: "ACTIVE")))

        let runner = RecordingProcessRunner(outcomes: [
            readyOutcome(url: "http://127.0.0.1:51234"),
            unavailableOutcome(),
            readyOutcome(url: "http://127.0.0.1:60001"),
        ], defaultOutcome: readyOutcome(url: "http://127.0.0.1:60001"))
        let collector = UpdateCollector()
        let factory = ClientSequence([first, recovered])
        let observer = makeObserver(
            directory: directory,
            runner: runner,
            factory: factory,
            selectedTaskID: "task-1",
            collector: collector
        )

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { collector.count >= 2 }
        XCTAssertEqual(collector.snapshot[1].connection, .disconnected)
        XCTAssertEqual(collector.snapshot[1].presentation.phase, .disconnected)
        XCTAssertTrue(collector.snapshot[1].presentation.isStaleSummary == false)

        // Background recovery only checks the service; it must never restart a
        // service the user stopped.
        await observer.refreshNow()
        await assertEventually { collector.count >= 3 }
        XCTAssertEqual(collector.snapshot[2].connection, .disconnected)
        XCTAssertFalse(runner.calls.contains { $0.arguments.contains("start") })

        await observer.refreshNow()
        await assertEventually { collector.snapshot.last?.connection == .connected(url: "http://127.0.0.1:60001") }
        await assertEventually { collector.snapshot.last?.presentation.phase == .working(node: "ACTIVE") }
        XCTAssertFalse(runner.calls.contains { $0.arguments.contains("start") })
        XCTAssertEqual(factory.requestedOrigins, ["http://127.0.0.1:51234", "http://127.0.0.1:60001"])
    }

    func testReconnectingAfterADisconnectDoesNotReplayTheCelebration() async {
        let directory = makeDirectory()
        let beforeDisconnect = ScriptedWebUIClient()
        beforeDisconnect.setSystemStatus(.value(TestDetails.liveStatus()))
        beforeDisconnect.setDetail(taskID: "task-1", .value(TestDetails.detail(lifecycle: .active)))
        let afterReconnect = ScriptedWebUIClient()
        afterReconnect.setSystemStatus(.value(TestDetails.liveStatus()))
        afterReconnect.setDetail(taskID: "task-1", .value(TestDetails.detail(currentNode: "DONE", lifecycle: .done)))

        let runner = RecordingProcessRunner(outcomes: [
            readyOutcome(url: TestFixtures.origin),
            unavailableOutcome(),
            readyOutcome(url: TestFixtures.origin),
        ], defaultOutcome: readyOutcome(url: TestFixtures.origin))
        let collector = UpdateCollector()
        let observer = makeObserver(
            directory: directory,
            runner: runner,
            factory: ClientSequence([beforeDisconnect, afterReconnect]),
            selectedTaskID: "task-1",
            collector: collector
        )

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { collector.count >= 2 }

        await observer.retryConnection()
        await assertEventually { collector.snapshot.last?.presentation.phase == .completed }
        // The Task reached DONE while the desktop was offline, so the user never
        // observed the change continuously and the celebration is not replayed.
        XCTAssertFalse(collector.snapshot.contains { $0.presentation.playIntro })
        XCTAssertTrue(collector.snapshot.last?.presentation.useRestFrame ?? false)
    }

    func testExplicitRetryMayAskCoreToStartTheService() async {
        let directory = makeDirectory()
        let client = ScriptedWebUIClient()
        client.setSystemStatus(.value(TestDetails.liveStatus()))
        client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        // Two unavailable statuses: the first keeps the desktop offline, and the
        // second makes the explicit retry ask Core to start the service once.
        let runner = RecordingProcessRunner(
            outcomes: [unavailableOutcome(), unavailableOutcome()],
            defaultOutcome: readyOutcome(url: TestFixtures.origin)
        )
        let collector = UpdateCollector()
        let observer = makeObserver(
            directory: directory,
            runner: runner,
            factory: ClientSequence([client]),
            selectedTaskID: "task-1",
            collector: collector
        )

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { collector.count >= 1 }
        XCTAssertEqual(collector.snapshot[0].connection, .disconnected)
        XCTAssertFalse(runner.calls.contains { $0.arguments.contains("start") }, "background polling must not start a service")

        await observer.retryConnection()
        await assertEventually { collector.snapshot.contains { $0.connection == .connected(url: TestFixtures.origin) } }
        XCTAssertEqual(runner.calls.map(\.arguments), [
            ["webui", "status", "--json"],
            ["webui", "status", "--json"],
            ["webui", "start", "--no-open", "--json"],
        ])
        await assertEventually { collector.snapshot.last?.presentation.phase == .working(node: "ACTIVE") }
    }

    // MARK: - Exit conditions

    func testMissingCoreExecutableExitsTheDesktop() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.directory.removeCoreExecutable()

        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 1 }
        XCTAssertEqual(context.collector.snapshot.last?.connection, .mustExit(reason: .coreExecutableMissing))
    }

    func testChangedCoreIdentityExitsTheDesktop() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setSystemStatus(.value(TestDetails.liveStatus(coreIdentity: TestFixtures.otherCoreIdentity)))

        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 1 }
        XCTAssertEqual(context.collector.snapshot.last?.connection, .mustExit(reason: .coreIdentityChanged))
    }

    func testChangedDataRootDigestExitsTheDesktop() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setSystemStatus(.value(TestDetails.liveStatus(dataRootDigest: TestFixtures.otherDataRootDigest)))

        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 1 }
        XCTAssertEqual(context.collector.snapshot.last?.connection, .mustExit(reason: .dataRootDigestChanged))
    }

    func testUnreadableServiceDoesNotActivateTheDesktop() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setSystemStatus(.value(TestDetails.liveStatus(readiness: .unavailable)))

        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 1 }
        XCTAssertEqual(context.collector.snapshot.last?.connection, .disconnected)
        XCTAssertFalse(context.client.recordedRequests.contains { $0.hasPrefix("detail:") })
    }

    // MARK: - Visibility

    func testHidingStopsReadingAndShowingReadsAgain() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        let observer = context.observer

        await observer.restoreSelectionFromPreferences()
        await observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }

        await observer.endObserving()
        let hiddenRequests = context.client.requestCount("detail:")
        await observer.refreshNow()
        await TestWaiting.pause(0.7)
        XCTAssertEqual(context.client.requestCount("detail:"), hiddenRequests, "a hidden desktop kept polling")

        await observer.beginObserving()
        await assertEventually { context.client.requestCount("detail:") > hiddenRequests }
    }

    // MARK: - Helpers

    func testPagesAllActiveTasksAndConfirmsDisappearedTaskThroughDetail() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setPage(1, lifecycle: .active, .value(TestDetails.list(page: 1, hasNext: true,
            summaries: [TestFixtures.summary(taskID: "a")])) )
        context.client.setPage(2, lifecycle: .active, .value(TestDetails.list(page: 2,
            summaries: [TestFixtures.summary(taskID: "a"), TestFixtures.summary(taskID: "b")])) )
        await context.observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.cards.count == 2 }
        XCTAssertTrue(context.client.recordedRequests.contains("list:active:page=2"))
        context.client.setPage(1, lifecycle: .active, .value(TestDetails.list(summaries: [TestFixtures.summary(taskID: "b")])) )
        context.client.setDetail(taskID: "a", .value(TestDetails.detail(taskID: "a", currentNode: "DONE", lifecycle: .done)))
        await context.observer.refreshNow(clearSelectionContinuity: false)
        await assertEventually { context.collector.snapshot.last?.presentation.phase == .completed }
        XCTAssertTrue(context.client.recordedRequests.contains("detail:a"))
        XCTAssertTrue(context.collector.snapshot.last?.cards.contains(where: { $0.taskID == "a" && $0.unread }) ?? false)
        await context.observer.acknowledge(taskID: "a")
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "b" }
    }

    func testHidingDropsAnInFlightCompletion() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.holdDetail(taskID: "task-1")
        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.client.openDetailGateCount == 1 }
        await context.observer.endObserving()
        let count = context.collector.count
        context.client.releaseDetailGate(.value(TestDetails.detail(currentNode: "DONE", lifecycle: .done)))
        await TestWaiting.pause(0.1)
        XCTAssertEqual(context.collector.count, count)
    }

    func testIdleRefreshChecksTheServiceAndRescansTasks() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setList(lifecycle: .blocked, .value(TestDetails.list(summaries: [])))
        context.client.setList(lifecycle: .active, .value(TestDetails.list(summaries: [])))
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }
        let count = context.collector.count
        await context.observer.refreshNow()
        await assertEventually { context.collector.count > count }
        XCTAssertEqual(context.client.requestCount("list:"), 4)
        XCTAssertEqual(context.client.requestCount("status"), 2)
    }

    func testFailedDefaultListDisconnectsInsteadOfLookingEmpty() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.setList(lifecycle: .blocked, .failure(.unreachable))
        await context.observer.beginObserving()
        await assertEventually { context.collector.snapshot.last?.connection == .disconnected }
        XCTAssertEqual(context.client.requestCount("list:"), 1)
    }

    func testLateDefaultSelectionKeepsTheUsersSelection() async {
        let context = makeContext(selectedTaskID: nil)
        context.client.holdLists()
        context.client.setDetail(taskID: "chosen", .value(TestDetails.detail(taskID: "chosen")))
        await context.observer.beginObserving()
        await assertEventually { context.client.openListGateCount == 1 }
        context.client.stopHoldingLists()
        await context.observer.select(taskID: "chosen")
        await assertEventually { context.collector.snapshot.last?.selectedTaskID == "chosen" }
        context.client.releaseListGate(.value(TestDetails.list(summaries: [TestFixtures.summary(taskID: "old-default")])))
        await TestWaiting.pause(0.1)
        let selected = await context.observer.currentSelection()
        XCTAssertEqual(selected, "chosen")
        XCTAssertEqual(context.preferences.current.selectedTask(for: TestFixtures.dataRootDigest), "chosen")
    }

    func testRemovingCoreAfterAHealthyReadExits() async {
        let context = makeContext(selectedTaskID: "task-1")
        context.client.setDetail(taskID: "task-1", .value(TestDetails.detail()))
        await context.observer.restoreSelectionFromPreferences()
        await context.observer.beginObserving()
        await assertEventually { context.collector.count >= 2 }
        context.directory.removeCoreExecutable()
        await context.observer.refreshNow()
        await assertEventually { context.collector.snapshot.last?.connection == .mustExit(reason: .coreExecutableMissing) }
    }

    private struct Context {
        let directory: PetTestDirectory
        let client: ScriptedWebUIClient
        let runner: RecordingProcessRunner
        let collector: UpdateCollector
        let preferences: PreferenceStore
        let observer: TaskObserver
    }

    private func makeContext(selectedTaskID: String?) -> Context {
        let directory = makeDirectory()
        let client = ScriptedWebUIClient()
        client.setSystemStatus(.value(TestDetails.liveStatus()))
        let runner = RecordingProcessRunner(outcomes: [], defaultOutcome: readyOutcome(url: TestFixtures.origin))
        let collector = UpdateCollector()
        let preferences = PreferenceStore(path: directory.settingsPath)
        if let selectedTaskID {
            preferences.update { $0.select(taskID: selectedTaskID, for: TestFixtures.dataRootDigest) }
        }
        let observer = makeObserver(
            directory: directory,
            runner: runner,
            factory: ClientSequence([client]),
            preferences: preferences,
            collector: collector
        )
        return Context(
            directory: directory,
            client: client,
            runner: runner,
            collector: collector,
            preferences: preferences,
            observer: observer
        )
    }

    private func makeObserver(
        directory: PetTestDirectory,
        runner: RecordingProcessRunner,
        factory: ClientSequence,
        preferences: PreferenceStore? = nil,
        selectedTaskID: String? = nil,
        collector: UpdateCollector
    ) -> TaskObserver {
        let store: PreferenceStore
        if let preferences {
            store = preferences
        } else {
            store = PreferenceStore(path: directory.settingsPath)
            if let selectedTaskID {
                store.update { $0.select(taskID: selectedTaskID, for: TestFixtures.dataRootDigest) }
            }
        }
        let observer = TaskObserver(
            core: CoreRuntimeClient(corePath: directory.corePath, dataDirectory: directory.root, runner: runner),
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            preferences: store,
            makeClient: { origin in factory.next(origin) }
        ) { update in
            collector.append(update)
        }
        observers.append(observer)
        return observer
    }

    private func makeDirectory() -> PetTestDirectory {
        let directory = PetTestDirectory()
        directories.append(directory)
        return directory
    }

    private func readyOutcome(url: String) -> ProcessOutcome {
        ProcessOutcome(
            exitCode: 0,
            stdout: TestFixtures.coreRuntimeStatus(url: url),
            stderr: "",
            timedOut: false
        )
    }

    private func unavailableOutcome() -> ProcessOutcome {
        ProcessOutcome(
            exitCode: 0,
            stdout: TestFixtures.coreRuntimeStatus(readiness: "unavailable", url: "", pid: nil),
            stderr: "",
            timedOut: false
        )
    }
}

/// Hands out the scripted clients in order and records which origin each was
/// requested for, so a recovered address is visible to the check.
final class ClientSequence: @unchecked Sendable {
    private let lock = NSLock()
    private var clients: [ScriptedWebUIClient]
    private var origins: [String] = []

    init(_ clients: [ScriptedWebUIClient]) {
        self.clients = clients
    }

    var requestedOrigins: [String] {
        lock.lock(); defer { lock.unlock() }
        return origins
    }

    func next(_ origin: String) -> WebUIReading? {
        lock.lock()
        origins.append(origin)
        let client = clients.isEmpty ? nil : clients.removeFirst()
        lock.unlock()
        return client
    }
}
