import Foundation

/// Holds the single-instance lock and this instance's own runtime record.
///
/// The lock is a process-held advisory lock, so the operating system releases it
/// when the process ends. That is what makes an expired record detectable: a
/// record whose process no longer matches, found while the lock is free, is
/// discarded instead of being trusted by PID or application name.
final class PetInstanceRuntime {
    /**
     * Read-only status uses the existing kernel identity check. An unreadable
     * record or process remains unknown; observation never removes the record.
     */
    static func status(productRoot: String) -> String {
        let path = PetPaths(productRoot: productRoot).runtimeRecord
        do {
            guard let data = try OwnedStorage.readBoundedData(path) else { return "stopped" }
            guard let record = InstanceRecord.decode(data), let pid = Int32(exactly: record.pid) else { return "unknown" }
            guard let identity = NativeProcess.identity(pid: pid) else {
                return NativeProcess.isGone(pid: pid) ? "stopped" : "unknown"
            }
            let decision = InstanceRules.decideStop(record: record, liveIdentity: identity,
                currentUserID: NativeProcess.currentUserID(), requestedExecutablePath: nil, requestedCorePath: nil)
            return decision == .terminate(pid: pid) ? "running" : "stopped"
        } catch { return "unknown" }
    }

    let paths: PetPaths

    private let lock: InstanceLock
    private(set) var record: InstanceRecord?
    private(set) var isShutdownStarted = false

    init(paths: PetPaths) {
        self.paths = paths
        lock = InstanceLock(path: paths.instanceLock)
    }

    /// Prepares the private directory and tries to take the single-instance lock.
    /// A failure to prepare the directory is reported instead of running without
    /// single-instance protection.
    func acquireLock() throws -> Bool {
        try OwnedStorage.ensurePrivateDirectory(paths.root)
        return lock.acquire()
    }

    /// Reads the record left by another instance. A record that cannot be decoded
    /// or that is missing a required field yields `nil`, which the caller treats
    /// as unverifiable rather than as an absent instance.
    func readRecord() -> InstanceRecord? {
        guard let data = try? OwnedStorage.readBoundedData(paths.runtimeRecord) else { return nil }
        return InstanceRecord.decode(data)
    }

    /// Writes this instance's record atomically after the window is ready.
    func writeRecord(_ record: InstanceRecord) throws {
        try OwnedStorage.writeAtomically(record.encoded(), to: paths.runtimeRecord)
        self.record = record
    }

    func beginShutdown() {
        isShutdownStarted = true
    }

    /// Removes the record this instance wrote and releases the lock. A record
    /// this instance did not write is left alone, so a stale record can never
    /// remove a concurrent start's own record.
    func release() {
        if let record, record.pid == Int(NativeProcess.currentIdentity()?.pid ?? -1) {
            OwnedStorage.remove(paths.runtimeRecord)
        }
        self.record = nil
        lock.release()
    }

    /// Discards a record that names `pid` once that process is gone. The caller
    /// must already hold the lock, which is what makes the removal safe.
    func discardRecord(naming pid: Int32) {
        guard let record = readRecord(), record.pid == Int(pid) else { return }
        OwnedStorage.remove(paths.runtimeRecord)
    }
}
