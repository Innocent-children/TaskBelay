import Foundation
import XCTest
@testable import TaskBelayPet

/// Covers the activation and navigation decisions.
///
/// An explicit open may ask Core to start the local service once; background
/// recovery is read-only. Local read-only storage without a connectable service
/// never activates the desktop, and a navigation re-check must confirm the same
/// Core, the same data directory, and the same loopback address.
final class ActivationAndNavigationTests: XCTestCase {
    // MARK: - Activation from a Core status result

    func testReadyStatusActivates() {
        let decision = ActivationRules.decide(
            coreResult: .status(try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus())),
            alreadyStartedService: false
        )
        XCTAssertEqual(decision, .activate(url: TestFixtures.origin, readiness: .ready))
    }

    func testReadOnlyStatusWithConnectableServiceActivates() {
        let status = try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(readiness: "read_only"))
        XCTAssertEqual(
            ActivationRules.decide(coreResult: .status(status), alreadyStartedService: false),
            .activate(url: TestFixtures.origin, readiness: .readOnly)
        )
    }

    func testUnavailableStatusMayStartTheServiceOnceOnAnExplicitOpen() {
        let status = try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(readiness: "unavailable", url: "", pid: nil))
        XCTAssertEqual(
            ActivationRules.decide(coreResult: .status(status), alreadyStartedService: false),
            .startServiceThenRetry
        )
        XCTAssertEqual(
            ActivationRules.decide(coreResult: .status(status), alreadyStartedService: true),
            .reject(.noConnectableService)
        )
    }

    func testIncompatibleAndUnknownReadinessStopTheAttempt() {
        for readiness in ["incompatible", "degraded"] {
            let status = try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(readiness: readiness))
            XCTAssertEqual(
                ActivationRules.decide(coreResult: .status(status), alreadyStartedService: false),
                .reject(.incompatible)
            )
        }
    }

    func testReadyStatusWithoutAddressDoesNotActivate() {
        let status = try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(url: ""))
        XCTAssertEqual(
            ActivationRules.decide(coreResult: .status(status), alreadyStartedService: false),
            .reject(.noConnectableService)
        )
    }

    func testCoreCommandFailuresAreReported() {
        for failure: CoreCommandFailure in [.processUnavailable, .timedOut, .invalidOutput, .exited(code: 1, detail: "boom")] {
            XCTAssertEqual(
                ActivationRules.decide(coreResult: .failed(failure), alreadyStartedService: false),
                .reject(.coreUnavailable(failure))
            )
        }
    }

    // MARK: - Live identity verification

    func testVerifyAcceptsTheSameIdentityAddressAndReadableService() {
        let live = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(readiness: "read_only"))
        XCTAssertTrue(ActivationRules.verify(
            live: live,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            expectedURL: TestFixtures.origin
        ))
    }

    func testVerifyRejectsChangedIdentityDigestAddressOrUnreadableService() {
        let changedCore = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(coreIdentity: TestFixtures.otherCoreIdentity))
        XCTAssertFalse(ActivationRules.verify(
            live: changedCore,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            expectedURL: TestFixtures.origin
        ))

        let changedDigest = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(dataRootDigest: TestFixtures.otherDataRootDigest))
        XCTAssertFalse(ActivationRules.verify(
            live: changedDigest,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            expectedURL: TestFixtures.origin
        ))

        let changedURL = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(url: "http://127.0.0.1:60000"))
        XCTAssertFalse(ActivationRules.verify(
            live: changedURL,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            expectedURL: TestFixtures.origin
        ))

        let unavailable = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(readiness: "unavailable"))
        XCTAssertFalse(ActivationRules.verify(
            live: unavailable,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest,
            expectedURL: TestFixtures.origin
        ))
    }

    // MARK: - Core invocation

    func testCoreClientForwardsTheDataDirectoryAndBoundArguments() async {
        let runner = RecordingProcessRunner(outcomes: [
            ProcessOutcome(exitCode: 0, stdout: TestFixtures.coreRuntimeStatus(), stderr: "", timedOut: false),
        ])
        let client = CoreRuntimeClient(corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: runner)
        guard case .status(let status) = await client.status() else {
            return XCTFail("a successful Core call must decode its status")
        }
        XCTAssertEqual(status.coreIdentity, TestFixtures.coreIdentity)
        XCTAssertEqual(runner.calls.count, 1)
        XCTAssertEqual(runner.calls[0].executable, "/runtime/taskbelay")
        XCTAssertEqual(runner.calls[0].arguments, ["webui", "status", "--json"])
        XCTAssertEqual(runner.calls[0].environment["TASKBELAY_DATA_DIR"], "/data")
        XCTAssertEqual(runner.calls[0].timeout, CoreRuntimeClient.invocationTimeout)
    }

    func testStartServiceUsesTheNonOpeningForm() async {
        let runner = RecordingProcessRunner(outcomes: [
            ProcessOutcome(exitCode: 0, stdout: TestFixtures.coreRuntimeStatus(operation: "start"), stderr: "", timedOut: false),
        ])
        let client = CoreRuntimeClient(corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: runner)
        _ = await client.startService()
        XCTAssertEqual(runner.calls[0].arguments, ["webui", "start", "--no-open", "--json"])
    }

    func testCoreClientInspectsExitCodeAndOutputTogether() async {
        let failing = RecordingProcessRunner(outcomes: [
            ProcessOutcome(exitCode: 1, stdout: TestFixtures.coreRuntimeStatus(), stderr: " data dir missing \n", timedOut: false),
        ])
        let failingResult = await CoreRuntimeClient(
            corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: failing
        ).status()
        XCTAssertEqual(failingResult, .failed(.exited(code: 1, detail: "data dir missing")))

        let silent = RecordingProcessRunner(outcomes: [ProcessOutcome(exitCode: 0, stdout: Data(), stderr: "", timedOut: false)])
        let silentResult = await CoreRuntimeClient(
            corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: silent
        ).status()
        XCTAssertEqual(silentResult, .failed(.invalidOutput))

        let timedOut = RecordingProcessRunner(outcomes: [
            ProcessOutcome(exitCode: 0, stdout: TestFixtures.coreRuntimeStatus(), stderr: "", timedOut: true),
        ])
        let timedOutResult = await CoreRuntimeClient(
            corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: timedOut
        ).status()
        XCTAssertEqual(timedOutResult, .failed(.timedOut))

        let unavailable = RecordingProcessRunner(outcomes: [])
        let unavailableResult = await CoreRuntimeClient(
            corePath: "/runtime/taskbelay", dataDirectory: "/data", runner: unavailable
        ).status()
        XCTAssertEqual(unavailableResult, .failed(.processUnavailable))
    }

    // MARK: - Navigation targets

    func testNavigationOpensTheTaskDetailForReadablePhases() {
        for phase: DisplayPhase in [.working(node: "ACTIVE"), .blocked(node: "BLOCKED"), .completed, .cancelled, .archived] {
            XCTAssertEqual(
                NavigationRules.target(origin: TestFixtures.origin, phase: phase, selectedTaskID: "task-1"),
                .taskDetail(url: "\(TestFixtures.origin)/tasks/task-1")
            )
        }
    }

    func testNavigationOpensTheListWithoutASelectionOrWithAMissingTask() {
        XCTAssertEqual(
            NavigationRules.target(origin: TestFixtures.origin, phase: .noSelection, selectedTaskID: nil),
            .taskList(url: "\(TestFixtures.origin)/tasks")
        )
        XCTAssertEqual(
            NavigationRules.target(origin: TestFixtures.origin, phase: .taskMissing, selectedTaskID: "task-1"),
            .taskList(url: "\(TestFixtures.origin)/tasks")
        )
        XCTAssertEqual(
            NavigationRules.target(origin: TestFixtures.origin, phase: .working(node: "ACTIVE"), selectedTaskID: ""),
            .taskList(url: "\(TestFixtures.origin)/tasks")
        )
    }

    func testNavigationKeepsTheTaskIdentifierInsideOneSegment() {
        XCTAssertEqual(
            NavigationRules.target(origin: TestFixtures.origin, phase: .working(node: "ACTIVE"), selectedTaskID: "a/b"),
            .taskDetail(url: "\(TestFixtures.origin)/tasks/a%2Fb")
        )
    }

    func testNavigationUsesTheRecheckedOriginAfterRecovery() {
        XCTAssertEqual(
            NavigationRules.target(origin: "http://10.0.0.5:8080", phase: .working(node: "ACTIVE"), selectedTaskID: "task-1"),
            .blocked(.invalidAddress)
        )
        XCTAssertEqual(
            NavigationRules.target(origin: TestFixtures.origin, phase: .disconnected, selectedTaskID: "task-1"),
            .taskDetail(url: "\(TestFixtures.origin)/tasks/task-1")
        )
    }

    func testNavigationRecheckRequiresAReadableMatchingService() {
        let coreResult = CoreStatusResult.status(try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus()))
        let live = try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus())
        XCTAssertNil(NavigationRules.recheck(
            coreResult: coreResult,
            live: live,
            expectedCoreIdentity: TestFixtures.coreIdentity,
            expectedDataRootDigest: TestFixtures.dataRootDigest
        ))

        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: .failed(.processUnavailable),
                live: live,
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .serviceUnreachable
        )
        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: coreResult,
                live: nil,
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .serviceUnreachable
        )
        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: coreResult,
                live: try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(coreIdentity: TestFixtures.otherCoreIdentity)),
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .identityMismatch
        )
        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: coreResult,
                live: try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(dataRootDigest: TestFixtures.otherDataRootDigest)),
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .identityMismatch
        )
        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: coreResult,
                live: try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(url: "http://127.0.0.1:60000")),
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .serviceUnreachable
        )
    }

    func testRecheckRejectsACoreThatIsNotReadable() {
        let unavailable = CoreStatusResult.status(
            try! CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(readiness: "unavailable", url: "", pid: nil))
        )
        XCTAssertEqual(
            NavigationRules.recheck(
                coreResult: unavailable,
                live: try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus()),
                expectedCoreIdentity: TestFixtures.coreIdentity,
                expectedDataRootDigest: TestFixtures.dataRootDigest
            ),
            .serviceUnreachable
        )
    }
}

/// Records every Core invocation so the tests can assert the argument array and
/// return a canned outcome per call.
final class RecordingProcessRunner: ProcessRunning, @unchecked Sendable {
    struct Call: Equatable {
        let executable: String
        let arguments: [String]
        let environment: [String: String]
        let timeout: TimeInterval
    }

    private let lock = NSLock()
    private var outcomes: [ProcessOutcome?]
    private let defaultOutcome: ProcessOutcome?
    private var recordedCalls: [Call] = []

    init(outcomes: [ProcessOutcome?], defaultOutcome: ProcessOutcome? = nil) {
        self.outcomes = outcomes
        self.defaultOutcome = defaultOutcome
    }

    var calls: [Call] {
        lock.lock(); defer { lock.unlock() }
        return recordedCalls
    }

    func run(
        executable: String,
        arguments: [String],
        environment: [String: String],
        timeout: TimeInterval
    ) async -> ProcessOutcome? {
        record(Call(executable: executable, arguments: arguments, environment: environment, timeout: timeout))
    }

    /// Keeps the lock out of the asynchronous body, which the concurrency checker
    /// rejects for `NSLock`.
    private func record(_ call: Call) -> ProcessOutcome? {
        lock.lock()
        recordedCalls.append(call)
        let outcome = outcomes.isEmpty ? defaultOutcome : outcomes.removeFirst()
        lock.unlock()
        return outcome
    }
}
