import Foundation
@testable import TaskBelayPet

/// Fixtures shaped like the current Core JSON output. Only the members the
/// desktop displays are asserted; the real responses carry more fields, which
/// the models deliberately ignore.
enum TestFixtures {
    static let coreIdentity = "0.4.1"
    static let otherCoreIdentity = "0.4.2"
    static let dataRootDigest = String(repeating: "ab", count: 32)
    static let otherDataRootDigest = String(repeating: "cd", count: 32)
    static let origin = "http://127.0.0.1:51234"

    static func systemStatus(
        readiness: String = "ready",
        coreIdentity: String = TestFixtures.coreIdentity,
        dataRootDigest: String = TestFixtures.dataRootDigest,
        url: String = TestFixtures.origin
    ) -> Data {
        json("""
        {"ok":true,"request_id":"req-1","readiness":"\(readiness)",\
        "core_identity":"\(coreIdentity)","data_root_digest":"\(dataRootDigest)","url":"\(url)"}
        """)
    }

    static func taskSummary(
        taskID: String = "task-1",
        requestSummary: String = "Add the desktop pet",
        originHost: String = "codex",
        currentNode: String = "ACTIVE",
        lifecycle: String = "active",
        revision: UInt64 = 3,
        updatedAt: String = "2026-09-06T08:15:30.123456789Z",
        archived: Bool = false,
        repositoryKeys: [String] = ["repo-a"],
        worktreePath: String = "/tmp/worktrees/task-1",
        blocker: String? = nil
    ) -> String {
        let blockerValue = blocker.map { "\"\($0)\"" } ?? "null"
        let repositories = repositoryKeys.map { "{\"key\":\"\($0)\",\"repository_path\":\"\(worktreePath)\",\"branch\":\"main\"}" }.joined(separator: ",")
        return """
        {"task_id":"\(taskID)","request_summary":"\(requestSummary)","host":"\(originHost)",\
        "status":"\(currentNode.lowercased())","lifecycle":"\(lifecycle)",\
        "revision":\(revision),"updated_at":"\(updatedAt)","archived":\(archived),\
        "repositories":[\(repositories)],\
        "blocker":\(blockerValue),"outcome":""}
        """
    }

    static func taskList(
        readiness: String = "ready",
        page: Int = 1,
        hasNext: Bool = false,
        summaries: [String] = []
    ) -> Data {
        json("""
        {"ok":true,"request_id":"req-2","readiness":"\(readiness)","page":\(page),\
        "has_next":\(hasNext),"items":[\(summaries.joined(separator: ","))]}
        """)
    }

    static func taskDetail(
        readiness: String = "ready",
        summary: String = TestFixtures.taskSummary()
    ) -> Data {
        json("""
        {"ok":true,"request_id":"req-3","readiness":"\(readiness)","summary":\(summary),\
        "intent":"implement","acceptance_criteria":[],"method_profile":"default"}
        """)
    }

    static func coreRuntimeStatus(
        readiness: String = "ready",
        coreIdentity: String = TestFixtures.coreIdentity,
        dataRootDigest: String = TestFixtures.dataRootDigest,
        url: String = TestFixtures.origin,
        pid: Int? = 4242
    ) -> Data {
        let pidMember = pid.map { ",\"pid\":\($0)" } ?? ""
        return json("""
        {"readiness":"\(readiness)","core_identity":"\(coreIdentity)",\
        "data_root_digest":"\(dataRootDigest)","url":"\(url)"\(pidMember)}
        """)
    }

    static func summary(
        taskID: String = "task-1",
        requestSummary: String = "Add the desktop pet",
        currentNode: String = "ACTIVE",
        lifecycle: TaskLifecycle = .active,
        revision: UInt64 = 3,
        archived: Bool = false,
        blocker: String? = nil
    ) -> DesktopTaskSummary {
        let data = taskDetail(summary: taskSummary(
            taskID: taskID,
            requestSummary: requestSummary,
            currentNode: currentNode,
            lifecycle: lifecycle.rawValue,
            revision: revision,
            archived: archived,
            blocker: blocker
        ))
        // The fixture is built from the same decoder the client uses, so a
        // malformed fixture fails the test instead of silently skewing it.
        return try! WebUIDecoding.decodeTaskDetail(data).summary
    }

    /// Orders fixture summaries by recency in whole seconds.
    ///
    /// The desktop keeps millisecond precision, so a difference that exists only
    /// in the nanosecond digits would not survive decoding and the recency rule
    /// would compare equal instants.
    static func updatedAt(offsetSeconds: Int) -> String {
        listTimestampFormatter.string(from: listBaseDate.addingTimeInterval(TimeInterval(offsetSeconds)))
    }

    private static let listBaseDate = Date(timeIntervalSince1970: 1_788_000_000)

    private static let listTimestampFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        return formatter
    }()

    private static func json(_ value: String) -> Data {
        Data(value.utf8)
    }
}
