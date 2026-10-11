import Foundation
import XCTest
@testable import TaskBelayPet

/// Releases a suspended read exactly when the test asks for it, which is how the
/// late-response checks stay deterministic instead of racing a timer.
final class ReadGate<Value>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<Value, Never>?
    private var released: Value?

    func wait() async -> Value {
        await withCheckedContinuation { continuation in
            lock.lock()
            if let released {
                lock.unlock()
                continuation.resume(returning: released)
                return
            }
            self.continuation = continuation
            lock.unlock()
        }
    }

    func release(_ value: Value) {
        lock.lock()
        released = value
        let pending = continuation
        continuation = nil
        lock.unlock()
        pending?.resume(returning: value)
    }
}

/// A loopback client whose responses the test scripts directly.
///
/// It records every request so the checks can assert which reads actually
/// happened, and it can hold a detail or list read until the test releases it.
final class ScriptedWebUIClient: WebUIReading, @unchecked Sendable {
    let origin: String

    private let lock = NSLock()
    private var systemStatusResult: WebUIReadResult<SystemStatusResponse> = .failure(.unreachable)
    private var detailResults: [String: WebUIReadResult<TaskDetailResponse>] = [:]
    private var listResults: [TaskLifecycle?: WebUIReadResult<TaskListResponse>] = [:]
    private var pageResults: [String: WebUIReadResult<TaskListResponse>] = [:]
    private var heldDetailTasks: Set<String> = []
    private var holdsLists = false
    private var openDetailGates: [ReadGate<WebUIReadResult<TaskDetailResponse>>] = []
    private var openListGates: [ReadGate<WebUIReadResult<TaskListResponse>>] = []
    private var requests: [String] = []

    init(origin: String = TestFixtures.origin) {
        self.origin = origin
    }

    // MARK: - Scripting

    func setSystemStatus(_ result: WebUIReadResult<SystemStatusResponse>) {
        lock.lock(); systemStatusResult = result; lock.unlock()
    }

    func setDetail(taskID: String, _ result: WebUIReadResult<TaskDetailResponse>) {
        lock.lock(); detailResults[taskID] = result; lock.unlock()
    }

    func setList(lifecycle: TaskLifecycle?, _ result: WebUIReadResult<TaskListResponse>) {
        lock.lock(); listResults[lifecycle] = result; lock.unlock()
    }

    func setPage(_ page: Int, lifecycle: TaskLifecycle, _ result: WebUIReadResult<TaskListResponse>) {
        lock.lock(); pageResults["\(lifecycle.rawValue):\(page)"] = result; lock.unlock()
    }

    func holdDetail(taskID: String) {
        lock.lock(); heldDetailTasks.insert(taskID); lock.unlock()
    }

    func stopHoldingLists() { lock.lock(); holdsLists = false; lock.unlock() }

    func holdLists() {
        lock.lock(); holdsLists = true; lock.unlock()
    }

    // MARK: - Observation

    var recordedRequests: [String] {
        lock.lock(); defer { lock.unlock() }
        return requests
    }

    func requestCount(_ prefix: String) -> Int {
        lock.lock(); defer { lock.unlock() }
        return requests.filter { $0.hasPrefix(prefix) }.count
    }

    var openDetailGateCount: Int {
        lock.lock(); defer { lock.unlock() }
        return openDetailGates.count
    }

    var openListGateCount: Int {
        lock.lock(); defer { lock.unlock() }
        return openListGates.count
    }

    func releaseDetailGate(_ result: WebUIReadResult<TaskDetailResponse>) {
        lock.lock()
        let gate = openDetailGates.isEmpty ? nil : openDetailGates.removeFirst()
        lock.unlock()
        gate?.release(result)
    }

    func releaseListGate(_ result: WebUIReadResult<TaskListResponse>) {
        lock.lock()
        let gate = openListGates.isEmpty ? nil : openListGates.removeFirst()
        lock.unlock()
        gate?.release(result)
    }

    // MARK: - WebUIReading

    /// Every lock operation stays in a synchronous helper, because the
    /// concurrency checker rejects `NSLock` inside an asynchronous body.
    func systemStatus() async -> WebUIReadResult<SystemStatusResponse> {
        beginStatusRead()
    }

    func taskList(page: Int, lifecycle: TaskLifecycle?) async -> WebUIReadResult<TaskListResponse> {
        switch beginListRead(page: page, lifecycle: lifecycle) {
        case .immediate(let result): return result
        case .held(let gate): return await gate.wait()
        }
    }

    func taskDetail(taskID: String) async -> WebUIReadResult<TaskDetailResponse> {
        switch beginDetailRead(taskID: taskID) {
        case .immediate(let result): return result
        case .held(let gate): return await gate.wait()
        }
    }

    private enum ScriptedRead<Value: Equatable> {
        case immediate(WebUIReadResult<Value>)
        case held(ReadGate<WebUIReadResult<Value>>)
    }

    private func beginStatusRead() -> WebUIReadResult<SystemStatusResponse> {
        lock.lock()
        requests.append("status")
        let result = systemStatusResult
        lock.unlock()
        return result
    }

    private func beginListRead(page: Int, lifecycle: TaskLifecycle?) -> ScriptedRead<TaskListResponse> {
        lock.lock()
        requests.append("list:\(lifecycle?.rawValue ?? "any"):page=\(page)")
        if holdsLists {
            let gate = ReadGate<WebUIReadResult<TaskListResponse>>()
            openListGates.append(gate)
            lock.unlock()
            return .held(gate)
        }
        let result = pageResults["\(lifecycle?.rawValue ?? "any"):\(page)"] ?? listResults[lifecycle] ?? .value(TestDetails.list(summaries: []))
        lock.unlock()
        return .immediate(result)
    }

    private func beginDetailRead(taskID: String) -> ScriptedRead<TaskDetailResponse> {
        lock.lock()
        requests.append("detail:\(taskID)")
        if heldDetailTasks.contains(taskID) {
            let gate = ReadGate<WebUIReadResult<TaskDetailResponse>>()
            openDetailGates.append(gate)
            lock.unlock()
            return .held(gate)
        }
        let result = detailResults[taskID] ?? .failure(.unreachable)
        lock.unlock()
        return .immediate(result)
    }
}

/// Collects the published observations of one observer.
final class UpdateCollector: @unchecked Sendable {
    private let lock = NSLock()
    private var updates: [ObservationUpdate] = []

    func append(_ update: ObservationUpdate) {
        lock.lock(); updates.append(update); lock.unlock()
    }

    var snapshot: [ObservationUpdate] {
        lock.lock(); defer { lock.unlock() }
        return updates
    }

    var count: Int {
        lock.lock(); defer { lock.unlock() }
        return updates.count
    }

    func reset() {
        lock.lock(); updates.removeAll(); lock.unlock()
    }
}

/// A temporary directory holding the stand-in Core executable and the pet
/// records, removed when the test ends.
final class PetTestDirectory {
    let root: String
    let corePath: String
    let settingsPath: String

    init() {
        root = (NSTemporaryDirectory() as NSString)
            .appendingPathComponent("taskbelay-pet-tests-\(UUID().uuidString)")
        try? FileManager.default.createDirectory(atPath: root, withIntermediateDirectories: true)
        corePath = (root as NSString).appendingPathComponent("taskbelay")
        FileManager.default.createFile(atPath: corePath, contents: Data("#!/bin/sh\n".utf8))
        settingsPath = (root as NSString).appendingPathComponent("settings.json")
    }

    func removeCoreExecutable() {
        try? FileManager.default.removeItem(atPath: corePath)
    }

    deinit {
        try? FileManager.default.removeItem(atPath: root)
    }
}

enum TestWaiting {
    /// Waits until `condition` holds or the timeout passes, and reports whether
    /// it held. Polling keeps the checks fast without depending on a fixed sleep.
    static func wait(
        timeout: TimeInterval = 5,
        condition: () -> Bool
    ) async -> Bool {
        let deadline = Date(timeIntervalSinceNow: timeout)
        while Date() < deadline {
            if condition() { return true }
            try? await Task.sleep(nanoseconds: 15_000_000)
        }
        return condition()
    }

    static func pause(_ interval: TimeInterval) async {
        try? await Task.sleep(nanoseconds: UInt64(interval * 1_000_000_000))
    }
}

/// Reports a polled condition as an assertion.
///
/// `XCTAssertTrue` takes an autoclosure, which cannot hold the `await` that
/// suspends until the observer publishes the expected update.
func assertEventually(
    _ message: String = "",
    timeout: TimeInterval = 5,
    file: StaticString = #filePath,
    line: UInt = #line,
    condition: @escaping () -> Bool
) async {
    let satisfied = await TestWaiting.wait(timeout: timeout, condition: condition)
    XCTAssertTrue(satisfied, message, file: file, line: line)
}

enum TestDetails {
    static func detail(
        taskID: String = "task-1",
        currentNode: String = "ACTIVE",
        lifecycle: TaskLifecycle = .active,
        revision: UInt64 = 1,
        archived: Bool = false,
        blocker: String? = nil,
        readiness: Readiness = .ready
    ) -> TaskDetailResponse {
        let data = TestFixtures.taskDetail(
            readiness: readiness.rawValue,
            summary: TestFixtures.taskSummary(
                taskID: taskID,
                currentNode: currentNode,
                lifecycle: lifecycle.rawValue,
                revision: revision,
                archived: archived,
                blocker: blocker
            )
        )
        return try! WebUIDecoding.decodeTaskDetail(data)
    }

    static func list(
        page: Int = 1,
        hasNext: Bool = false,
        summaries: [DesktopTaskSummary]
    ) -> TaskListResponse {
        let data = TestFixtures.taskList(
            page: page,
            hasNext: hasNext,
            summaries: summaries.map { summary in
                TestFixtures.taskSummary(
                    taskID: summary.taskID,
                    requestSummary: summary.requestSummary,
                    currentNode: summary.currentNode,
                    lifecycle: summary.lifecycle.rawValue,
                    revision: summary.revision,
                    // The update time orders the fixtures by recency, which is
                    // what the default-selection rule compares.
                    updatedAt: TestFixtures.updatedAt(offsetSeconds: Int(summary.revision)),
                    archived: summary.archived,
                    blocker: summary.blocker
                )
            }
        )
        return try! WebUIDecoding.decodeTaskList(data)
    }

    static func liveStatus(
        readiness: Readiness = .ready,
        coreIdentity: String = TestFixtures.coreIdentity,
        dataRootDigest: String = TestFixtures.dataRootDigest,
        url: String = TestFixtures.origin
    ) -> SystemStatusResponse {
        try! WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(
            readiness: readiness.rawValue,
            coreIdentity: coreIdentity,
            dataRootDigest: dataRootDigest,
            url: url
        ))
    }
}
