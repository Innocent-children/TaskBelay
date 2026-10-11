import Foundation
import XCTest
@testable import TaskBelayPet

/// Covers decoding of the current Core JSON output and the two address rules the
/// desktop depends on: only the confirmed loopback origin is readable, and a
/// Task identifier is always exactly one encoded path segment.
final class WebUIModelTests: XCTestCase {
    func testDecodesSystemStatusMembers() throws {
        let status = try WebUIDecoding.decodeSystemStatus(TestFixtures.systemStatus(readiness: "read_only"))
        XCTAssertEqual(status.readiness, .readOnly)
        XCTAssertEqual(status.coreIdentity, TestFixtures.coreIdentity)
        XCTAssertEqual(status.dataRootDigest, TestFixtures.dataRootDigest)
        XCTAssertEqual(status.url, TestFixtures.origin)
    }

    func testDecodesTaskListPagingAndItems() throws {
        let data = TestFixtures.taskList(
            page: 2,
            hasNext: true,
            summaries: [
                TestFixtures.taskSummary(taskID: "task-a"),
                TestFixtures.taskSummary(taskID: "task-b", lifecycle: "blocked", blocker: "needs a decision"),
            ]
        )
        let list = try WebUIDecoding.decodeTaskList(data)
        XCTAssertEqual(list.page, 2)
        XCTAssertTrue(list.hasNext)
        XCTAssertEqual(list.items.map(\.taskID), ["task-a", "task-b"])
        XCTAssertEqual(list.items[1].lifecycle, .blocked)
        XCTAssertEqual(list.items[1].blocker, "needs a decision")
        XCTAssertNil(list.items[0].blocker)
    }

    func testDecodesTaskDetailSummaryAndReadiness() throws {
        let detail = try WebUIDecoding.decodeTaskDetail(TestFixtures.taskDetail(readiness: "read_only"))
        XCTAssertEqual(detail.readiness, .readOnly)
        XCTAssertEqual(detail.summary.taskID, "task-1")
        XCTAssertEqual(detail.summary.currentNode, "ACTIVE")
        XCTAssertEqual(detail.summary.originHost, "codex")
        XCTAssertEqual(detail.summary.repositoryKeys, ["repo-a"])
        XCTAssertEqual(detail.summary.worktreePath, "/tmp/worktrees/task-1")
        XCTAssertEqual(detail.summary.revision, 3)
        XCTAssertFalse(detail.summary.archived)
    }

    func testDecodesNanosecondTimestampEmittedByGo() throws {
        let detail = try WebUIDecoding.decodeTaskDetail(
            TestFixtures.taskDetail(summary: TestFixtures.taskSummary(updatedAt: "2026-09-06T08:15:30.123456789Z"))
        )
        let truncated = try WebUIDecoding.decodeTaskDetail(
            TestFixtures.taskDetail(summary: TestFixtures.taskSummary(updatedAt: "2026-09-06T08:15:30.123Z"))
        )
        // Go emits up to nine fraction digits. The desktop keeps millisecond
        // precision, so the longer form reads back as the truncated instant.
        XCTAssertEqual(detail.summary.updatedAt, truncated.summary.updatedAt)

        let expected = try XCTUnwrap(DateComponents(
            calendar: Calendar(identifier: .gregorian),
            timeZone: TimeZone(secondsFromGMT: 0),
            year: 2026, month: 9, day: 6, hour: 8, minute: 15, second: 30, nanosecond: 123_000_000
        ).date)
        // `Calendar` and the ISO 8601 parser can differ in the last stored bit
        // of the same instant, so the comparison allows that difference and
        // nothing larger.
        XCTAssertEqual(
            detail.summary.updatedAt.timeIntervalSince1970,
            expected.timeIntervalSince1970,
            accuracy: 0.0002
        )
    }

    func testDecodesTimestampWithoutFractionAndWithOffset() throws {
        let plain = try WebUIDecoding.decodeTaskDetail(
            TestFixtures.taskDetail(summary: TestFixtures.taskSummary(updatedAt: "2026-09-06T08:15:30Z"))
        )
        let offset = try WebUIDecoding.decodeTaskDetail(
            TestFixtures.taskDetail(summary: TestFixtures.taskSummary(updatedAt: "2026-09-06T16:15:30+08:00"))
        )
        XCTAssertEqual(plain.summary.updatedAt, offset.summary.updatedAt)
    }

    func testRejectsTimestampThatIsNotRFC3339() {
        XCTAssertThrowsError(try WebUIDecoding.decodeTaskDetail(
            TestFixtures.taskDetail(summary: TestFixtures.taskSummary(updatedAt: "yesterday"))
        ))
    }

    func testUnknownLifecycleAndReadinessBecomeUnknown() throws {
        let list = try WebUIDecoding.decodeTaskList(
            TestFixtures.taskList(readiness: "degraded", summaries: [TestFixtures.taskSummary(lifecycle: "paused")])
        )
        XCTAssertEqual(list.readiness, .unknown)
        XCTAssertEqual(list.items[0].lifecycle, .unknown)
        XCTAssertFalse(list.items[0].lifecycle.isTerminal)
    }

    func testTerminalLifecycleClassification() {
        XCTAssertTrue(TaskLifecycle.done.isTerminal)
        XCTAssertTrue(TaskLifecycle.cancelled.isTerminal)
        XCTAssertFalse(TaskLifecycle.active.isTerminal)
        XCTAssertFalse(TaskLifecycle.blocked.isTerminal)
    }

    func testDecodesCoreRuntimeStatusAtTopLevel() throws {
        let status = try CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(operation: "start", readiness: "unavailable", pid: nil))
        XCTAssertEqual(status.operation, "start")
        XCTAssertEqual(status.readiness, .unavailable)
        XCTAssertNil(status.pid)
        XCTAssertEqual(status.coreIdentity, TestFixtures.coreIdentity)
        XCTAssertEqual(status.dataRootDigest, TestFixtures.dataRootDigest)
    }

    func testRejectsIncompleteCoreRuntimeStatus() {
        XCTAssertThrowsError(try CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(coreIdentity: "")))
        XCTAssertThrowsError(try CoreRuntimeStatus.decode(TestFixtures.coreRuntimeStatus(dataRootDigest: "")))
        XCTAssertThrowsError(try CoreRuntimeStatus.decode(Data("not json".utf8)))
    }

    func testAcceptsOnlyConfirmedLoopbackOrigin() {
        XCTAssertTrue(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:1"))
        XCTAssertTrue(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:65535"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:0"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:65536"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://localhost:8080"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("https://127.0.0.1:8080"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:8080/tasks"))
        XCTAssertFalse(LoopbackWebUIClient.isConfirmedLoopbackOrigin("http://127.0.0.1:"))
        XCTAssertNil(LoopbackWebUIClient(origin: "http://10.0.0.5:8080"))
        XCTAssertNotNil(LoopbackWebUIClient(origin: TestFixtures.origin))
    }

    func testEncodesTaskIdentifierAsOnePathSegment() {
        XCTAssertEqual(LoopbackWebUIClient.pathSegment("task-1"), "task-1")
        XCTAssertEqual(LoopbackWebUIClient.pathSegment("a/b"), "a%2Fb")
        XCTAssertEqual(LoopbackWebUIClient.pathSegment(".."), "..")
        XCTAssertEqual(LoopbackWebUIClient.pathSegment("../../etc"), "..%2F..%2Fetc")
        XCTAssertEqual(LoopbackWebUIClient.pathSegment("a b?c#d"), "a%20b%3Fc%23d")
        XCTAssertNil(LoopbackWebUIClient.pathSegment(""))
    }

    func testTaskDetailRequestPathStaysInsideTheTaskSegment() async {
        // A Task identifier containing a separator can never widen the request
        // path, so the detail read still targets one Task.
        let segment = LoopbackWebUIClient.pathSegment("task/../../admin")
        XCTAssertEqual(segment, "task%2F..%2F..%2Fadmin")
        XCTAssertFalse(segment?.contains("/") ?? true)
    }
}
