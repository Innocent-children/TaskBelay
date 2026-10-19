import Foundation
import Darwin
import XCTest
@testable import TaskBelayPet

/// Covers the private native entry contract, the single-instance and shutdown
/// decisions, and the preference and position rules.
///
/// Identity is decided from actual process facts, never from a PID or an
/// application name. Preferences hold only position, the animation switch, and
/// the selected Task identifiers; language is never stored.
final class LaunchAndInstanceTests: XCTestCase {
    private let digest = TestFixtures.dataRootDigest

    func testNativeIdentityAndInstanceDecisionsIgnoreLocaleAndTimezone() throws {
        let keys = ["LANG", "LC_ALL", "LC_TIME", "TZ"]
        let original = keys.map { key in (key, getenv(key).map { String(cString: $0) }) }
        defer {
            for (key, value) in original {
                if let value { setenv(key, value, 1) } else { unsetenv(key) }
            }
        }
        let initial = try XCTUnwrap(NativeProcess.currentIdentity())
        XCTAssertNotNil(initial.startIdentity.range(of: "^[0-9]+:[0-9]+$", options: .regularExpression))
        let record = makeRecord(pid: Int(initial.pid), processStartIdentity: initial.startIdentity,
            executablePath: initial.executablePath)
        for (locale, timezone) in [("zh_CN.UTF-8", "Asia/Shanghai"), ("en_US.UTF-8", "America/Los_Angeles"), ("C", "UTC")] {
            for key in ["LANG", "LC_ALL", "LC_TIME"] { setenv(key, locale, 1) }
            setenv("TZ", timezone, 1)
            let live = try XCTUnwrap(NativeProcess.identity(pid: initial.pid))
            XCTAssertEqual(live, initial, "\(locale), \(timezone)")
            XCTAssertEqual(InstanceRules.decideRun(lockAcquired: false, record: record,
                liveIdentity: live, currentIdentity: initial, corePath: record.corePath,
                coreIdentity: record.coreIdentity, dataRootDigest: record.dataRootDigest), .restore(pid: initial.pid))
            XCTAssertEqual(InstanceRules.decideStop(record: record, liveIdentity: live,
                currentUserID: initial.ownerUserID, requestedExecutablePath: nil,
                requestedCorePath: record.corePath), .terminate(pid: initial.pid))
        }
    }

    func testNativeStartIdentityRejectsInvalidAndExitedProcesses() throws {
        XCTAssertNil(NativeProcess.processStartIdentity(pid: 0))
        XCTAssertNil(NativeProcess.processStartIdentity(pid: -1))
        let child = Process()
        child.executableURL = URL(fileURLWithPath: "/bin/sleep")
        child.arguments = ["30"]
        try child.run()
        defer { if child.isRunning { child.terminate(); child.waitUntilExit() } }
        let pid = child.processIdentifier
        XCTAssertNotNil(NativeProcess.processStartIdentity(pid: pid))
        child.terminate()
        child.waitUntilExit()
        XCTAssertNil(NativeProcess.processStartIdentity(pid: pid))
    }

    func testReadOnlyStatusVerifiesProcessAndRetainsRecords() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        XCTAssertEqual(PetInstanceRuntime.status(productRoot: root.path), "stopped")
        XCTAssertFalse(FileManager.default.fileExists(atPath: root.path))
        let pet = root.appendingPathComponent("pet")
        try FileManager.default.createDirectory(at: pet, withIntermediateDirectories: true)
        let live = try XCTUnwrap(NativeProcess.currentIdentity())
        let record = makeRecord(pid: Int(live.pid), processStartIdentity: live.startIdentity, executablePath: live.executablePath)
        let path = pet.appendingPathComponent("runtime.json")
        let bytes = try record.encoded()
        try bytes.write(to: path)
        XCTAssertEqual(PetInstanceRuntime.status(productRoot: root.path), "running")
        XCTAssertEqual(try Data(contentsOf: path), bytes)
        let stale = makeRecord(pid: Int(live.pid), processStartIdentity: "recycled", executablePath: live.executablePath)
        try stale.encoded().write(to: path)
        XCTAssertEqual(PetInstanceRuntime.status(productRoot: root.path), "stopped")
        try Data("broken".utf8).write(to: path)
        XCTAssertEqual(PetInstanceRuntime.status(productRoot: root.path), "unknown")
        XCTAssertEqual(try String(contentsOf: path), "broken")
        let arguments = try LaunchArgumentParser.parse(["status", "--product-root", root.path])
        XCTAssertEqual(arguments.mode, .status)
        XCTAssertThrowsError(try LaunchArgumentParser.parse(["status", "--product-root", root.path, "--core-path", "/core"]))
    }

    // MARK: - Launch arguments

    func testParsesTheRunEntry() throws {
        let arguments = try LaunchArgumentParser.parse([
            "run",
            "--core-path", "/runtime/taskbelay",
            "--data-dir", "/data",
            "--product-root", "/product",
            "--core-identity", TestFixtures.coreIdentity,
            "--data-root-digest", digest,
        ])
        XCTAssertEqual(arguments.mode, .run)
        XCTAssertEqual(arguments.corePath, "/runtime/taskbelay")
        XCTAssertEqual(arguments.dataDirectory, "/data")
        XCTAssertEqual(arguments.productRoot, "/product")
        XCTAssertEqual(arguments.coreIdentity, TestFixtures.coreIdentity)
        XCTAssertEqual(arguments.dataRootDigest, digest)
    }

    func testParsesTheStopEntryWithAnOptionalCoreFilter() throws {
        let plain = try LaunchArgumentParser.parse(["stop", "--product-root", "/product"])
        XCTAssertEqual(plain.mode, .stop)
        XCTAssertNil(plain.corePath)
        XCTAssertNil(plain.dataDirectory)
        XCTAssertNil(plain.coreIdentity)
        XCTAssertNil(plain.dataRootDigest)

        let filtered = try LaunchArgumentParser.parse([
            "stop", "--product-root", "/product", "--core-path", "/runtime/taskbelay",
        ])
        XCTAssertEqual(filtered.corePath, "/runtime/taskbelay")
    }

    func testRejectsAMissingOrUnsupportedEntry() {
        assertArgumentFailure([])
        assertArgumentFailure(["unknown", "--product-root", "/product"])
        assertArgumentFailure(["run"])
    }

    func testRunRequiresEveryVerifiedField() {
        assertArgumentFailure(["run", "--product-root", "/product"])
        assertArgumentFailure([
            "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
            "--data-root-digest", digest,
        ])
        assertArgumentFailure([
            "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
            "--core-identity", TestFixtures.coreIdentity,
        ])
    }

    func testRejectsRelativePathsUnknownAndDuplicateOptions() {
        assertArgumentFailure([
            "run", "--core-path", "runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
            "--core-identity", TestFixtures.coreIdentity, "--data-root-digest", digest,
        ])
        assertArgumentFailure([
            "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
            "--core-identity", TestFixtures.coreIdentity, "--data-root-digest", digest, "--browser", "safari",
        ])
        // The shutdown entry accepts only its own two fields, so a launcher that
        // drifted to the run form fails instead of stopping the wrong instance.
        assertArgumentFailure(["stop", "--product-root", "/product", "--data-dir", "/data"])
        assertArgumentFailure([
            "run", "--core-path", "/runtime/taskbelay", "--core-path", "/other/taskbelay", "--data-dir", "/data",
            "--product-root", "/product", "--core-identity", TestFixtures.coreIdentity, "--data-root-digest", digest,
        ])
        assertArgumentFailure([
            "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
            "--core-identity", TestFixtures.coreIdentity, "--data-root-digest",
        ])
    }

    func testRejectsAnUnverifiedIdentityOrDigestShape() {
        for identity in ["taskbelay/0.4.1", "0.4", "04.1.0", "0.4.1-extra", ""] {
            assertArgumentFailure([
                "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
                "--core-identity", identity, "--data-root-digest", digest,
            ])
        }
        for value in ["AB" + String(repeating: "ab", count: 31), String(repeating: "a", count: 63), ""] {
            assertArgumentFailure([
                "run", "--core-path", "/runtime/taskbelay", "--data-dir", "/data", "--product-root", "/product",
                "--core-identity", TestFixtures.coreIdentity, "--data-root-digest", value,
            ])
        }
    }

    // MARK: - Runtime record

    func testRuntimeRecordRoundTripsAndRejectsIncompleteValues() throws {
        let record = InstanceRecord(
            pid: 4242,
            processStartIdentity: "1788682530:123456",
            executablePath: "/app/TaskBelayPet.app/Contents/MacOS/TaskBelayPet",
            corePath: "/runtime/taskbelay",
            coreIdentity: TestFixtures.coreIdentity,
            dataRootDigest: digest
        )
        XCTAssertEqual(InstanceRecord.decode(try record.encoded()), record)

        let incomplete = InstanceRecord(
            pid: 0,
            processStartIdentity: "1788682530:123456",
            executablePath: "/app/TaskBelayPet",
            corePath: "/runtime/taskbelay",
            coreIdentity: TestFixtures.coreIdentity,
            dataRootDigest: digest
        )
        XCTAssertNil(InstanceRecord.decode(try incomplete.encoded()))
        XCTAssertNil(InstanceRecord.decode(Data("{}".utf8)))
    }

    func testPetPathsPlaceTheRecordsBesideTheDataDirectory() {
        let paths = PetPaths(productRoot: "/product")
        XCTAssertEqual(paths.root, "/product/pet")
        XCTAssertEqual(paths.instanceLock, "/product/pet/instance.lock")
        XCTAssertEqual(paths.runtimeRecord, "/product/pet/runtime.json")
        XCTAssertEqual(paths.settings, "/product/pet/settings.json")
    }

    // MARK: - Single instance

    func testFreeLockStartsANewInstance() {
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: true,
                record: makeRecord(),
                liveIdentity: nil,
                currentIdentity: makeIdentity(),
                corePath: "/runtime/taskbelay",
                coreIdentity: TestFixtures.coreIdentity,
                dataRootDigest: digest
            ),
            .start
        )
    }

    func testSameCoreAndDataDirectoryRestoresTheRunningInstance() {
        let record = makeRecord()
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .restore(pid: 4242)
        )
    }

    func testDifferentCoreOrDataDirectoryConflicts() {
        let record = makeRecord()
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(),
                currentIdentity: makeIdentity(),
                corePath: "/other/taskbelay",
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.coreDiffers)
        )
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: TestFixtures.otherCoreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.coreDiffers)
        )
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: TestFixtures.otherDataRootDigest
            ),
            .conflict(.dataDirectoryDiffers)
        )
    }

    func testRecycledPidOrAnotherUserIsUnverifiable() {
        let record = makeRecord()
        // A recycled PID carries a different start identity, so the record can
        // never be trusted on the number alone.
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(startIdentity: "1788682530:123457"),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.recordUnverifiable)
        )
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(executablePath: "/other/TaskBelayPet"),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.recordUnverifiable)
        )
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: record,
                liveIdentity: makeIdentity(ownerUserID: 502),
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.recordUnverifiable)
        )
        XCTAssertEqual(
            InstanceRules.decideRun(
                lockAcquired: false,
                record: nil,
                liveIdentity: nil,
                currentIdentity: makeIdentity(),
                corePath: record.corePath,
                coreIdentity: record.coreIdentity,
                dataRootDigest: record.dataRootDigest
            ),
            .conflict(.recordUnverifiable)
        )
    }

    // MARK: - Shutdown

    func testStopWithoutARecordSucceeds() {
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: nil,
                liveIdentity: nil,
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: nil
            ),
            .nothingToStop
        )
    }

    func testStopTerminatesTheMatchingCurrentUserInstance() {
        let record = makeRecord()
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(),
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: nil
            ),
            .terminate(pid: 4242)
        )
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(),
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: record.corePath
            ),
            .terminate(pid: 4242)
        )
    }

    func testAdapterMaintenanceOnlyStopsInstancesUsingThatCore() {
        let record = makeRecord()
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(),
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: "/other/taskbelay"
            ),
            .otherCoreInUse
        )
    }

    func testStopDiscardsARecordThatNoLongerMatchesALiveProcess() {
        let record = makeRecord()
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: nil,
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: nil
            ),
            .expiredRecord
        )
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(startIdentity: "1788682530:123457"),
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: nil
            ),
            .expiredRecord
        )
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(ownerUserID: 502),
                currentUserID: 501,
                requestedExecutablePath: nil,
                requestedCorePath: nil
            ),
            .expiredRecord
        )
        XCTAssertEqual(
            InstanceRules.decideStop(
                record: record,
                liveIdentity: makeIdentity(),
                currentUserID: 501,
                requestedExecutablePath: "/app/Other.app/Contents/MacOS/Other",
                requestedCorePath: nil
            ),
            .expiredRecord
        )
    }

    // MARK: - Preferences

    func testStopRejectsAHeldLockWithoutARuntimeRecord() throws {
        let directory = PetTestDirectory()
        let runtime = PetInstanceRuntime(paths: PetPaths(productRoot: directory.root))
        XCTAssertTrue(try runtime.acquireLock())
        XCTAssertEqual(PetStopRequest.run(productRoot: directory.root, corePath: nil), 1)
        runtime.release()
        XCTAssertEqual(PetStopRequest.run(productRoot: directory.root, corePath: nil), 0)
    }

    func testMissingOrInvalidPreferencesYieldCurrentDefaults() throws {
        let directory = PetTestDirectory()

        XCTAssertEqual(PreferenceStore.load(path: directory.settingsPath), .default)
        try Data("not json".utf8).write(to: URL(fileURLWithPath: directory.settingsPath))
        XCTAssertEqual(PreferenceStore.load(path: directory.settingsPath), .default)
    }

    func testPreferencesStorePositionAnimationSwitchAndSelectionPerDataDirectory() throws {
        let directory = PetTestDirectory()
        let store = PreferenceStore(path: directory.settingsPath)

        XCTAssertTrue(store.update { preferences in
            preferences.position = PetPreferences.Position(x: 120, y: 80)
            preferences.animationsEnabled = false
            preferences.idleActivitiesEnabled = false
            preferences.scale = 1.5
            preferences.select(taskID: "task-a", for: TestFixtures.dataRootDigest)
            preferences.select(taskID: "task-b", for: TestFixtures.otherDataRootDigest)
        })
        // An unchanged write does not touch the file again.
        XCTAssertFalse(store.update { _ in })

        let reloaded = PreferenceStore(path: directory.settingsPath)
        XCTAssertEqual(reloaded.current.position, PetPreferences.Position(x: 120, y: 80))
        XCTAssertFalse(reloaded.current.animationsEnabled)
        XCTAssertFalse(reloaded.current.idleActivitiesEnabled)
        XCTAssertEqual(reloaded.current.scale, 1.5)
        XCTAssertEqual(reloaded.current.selectedTask(for: TestFixtures.dataRootDigest), "task-a")
        XCTAssertEqual(reloaded.current.selectedTask(for: TestFixtures.otherDataRootDigest), "task-b")

        reloaded.update { $0.select(taskID: nil, for: TestFixtures.dataRootDigest) }
        XCTAssertNil(PreferenceStore(path: directory.settingsPath).current.selectedTask(for: TestFixtures.dataRootDigest))
        XCTAssertEqual(PreferenceStore(path: directory.settingsPath).current.selectedTask(for: TestFixtures.otherDataRootDigest), "task-b")
    }

    func testPreferencesDoNotStoreLanguageOrTaskState() throws {
        let directory = PetTestDirectory()
        let store = PreferenceStore(path: directory.settingsPath)
        store.update { preferences in
            preferences.position = PetPreferences.Position(x: 1, y: 2)
            preferences.select(taskID: "task-a", for: digest)
        }
        let text = try String(contentsOfFile: directory.settingsPath, encoding: .utf8)
        let keys = try XCTUnwrap(
            (JSONSerialization.jsonObject(with: Data(text.utf8)) as? [String: Any]).map { Set($0.keys) }
        )
        XCTAssertEqual(keys, ["animations_enabled", "idle_activities_enabled", "position", "pinned_tasks", "scale"])
    }

    func testPrivatePermissionsOnThePetDirectoryAndRecords() throws {
        let directory = PetTestDirectory()
        let paths = PetPaths(productRoot: directory.root)
        let store = PreferenceStore(path: paths.settings)
        store.update { $0.animationsEnabled = false }

        let mode = try FileManager.default.attributesOfItem(atPath: paths.settings)[.posixPermissions] as? NSNumber
        XCTAssertEqual(mode?.intValue, 0o600)
        let directoryMode = try FileManager.default.attributesOfItem(atPath: paths.root)[.posixPermissions] as? NSNumber
        XCTAssertEqual(directoryMode?.intValue, 0o700)
    }

    func testLanguageFollowsTheSystemPreferenceAndIsNotStored() {
        XCTAssertEqual(PetLanguage.resolve(preferredLanguages: ["zh-Hans-CN", "en-US"]), .chinese)
        XCTAssertEqual(PetLanguage.resolve(preferredLanguages: ["zh-Hant-TW"]), .chinese)
        XCTAssertEqual(PetLanguage.resolve(preferredLanguages: ["en-US"]), .english)
        XCTAssertEqual(PetLanguage.resolve(preferredLanguages: ["ja-JP"]), .english)
        XCTAssertEqual(PetLanguage.resolve(preferredLanguages: []), .english)
    }

    // MARK: - Position

    func testFirstPlacementIsInsideTheLowerRightOfWorkArea() {
        let visible = CGRect(x: 0, y: 0, width: 1512, height: 900)
        let origin = PositionRules.constrain(position: nil, windowSize: CGSize(width: 220, height: 220), visibleFrame: visible, fallbackInset: 24)
        XCTAssertEqual(origin, CGPoint(x: 1512 - 220 - 24, y: 24))
    }

    func testRememberedPositionIsKeptInsideTheVisibleWorkArea() {
        let visible = CGRect(x: 0, y: 0, width: 1512, height: 900)
        let size = CGSize(width: 220, height: 220)
        XCTAssertEqual(
            PositionRules.constrain(position: PetPreferences.Position(x: 300, y: 200), windowSize: size, visibleFrame: visible, fallbackInset: 24),
            CGPoint(x: 300, y: 200)
        )
        // A removed display leaves a remembered position outside every visible
        // area; the whole window moves back instead of staying unreachable.
        XCTAssertEqual(
            PositionRules.constrain(position: PetPreferences.Position(x: 4000, y: -900), windowSize: size, visibleFrame: visible, fallbackInset: 24),
            CGPoint(x: 1512 - 220, y: 0)
        )
    }

    func testWorkAreaSmallerThanTheWindowFallsBackToItsOrigin() {
        let visible = CGRect(x: 10, y: 20, width: 100, height: 100)
        XCTAssertEqual(
            PositionRules.constrain(position: PetPreferences.Position(x: 500, y: 500), windowSize: CGSize(width: 220, height: 220), visibleFrame: visible, fallbackInset: 24),
            CGPoint(x: 10, y: 20)
        )
    }

    // MARK: - Helpers

    private func assertArgumentFailure(_ rawArguments: [String], file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertThrowsError(try LaunchArgumentParser.parse(rawArguments), file: file, line: line)
    }

    private func makeRecord(
        pid: Int = 4242,
        processStartIdentity: String = "1788682530:123456",
        executablePath: String = "/app/TaskBelayPet.app/Contents/MacOS/TaskBelayPet",
        corePath: String = "/runtime/taskbelay",
        coreIdentity: String = TestFixtures.coreIdentity,
        dataRootDigest: String? = nil
    ) -> InstanceRecord {
        InstanceRecord(
            pid: pid,
            processStartIdentity: processStartIdentity,
            executablePath: executablePath,
            corePath: corePath,
            coreIdentity: coreIdentity,
            dataRootDigest: dataRootDigest ?? digest
        )
    }

    private func makeIdentity(
        pid: Int32 = 4242,
        startIdentity: String = "1788682530:123456",
        executablePath: String = "/app/TaskBelayPet.app/Contents/MacOS/TaskBelayPet",
        ownerUserID: uid_t = 501
    ) -> NativeProcess.Identity {
        NativeProcess.Identity(
            pid: pid,
            startIdentity: startIdentity,
            executablePath: executablePath,
            ownerUserID: ownerUserID
        )
    }
}
