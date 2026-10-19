import AppKit
import Darwin
import Foundation
import os

/// `PROC_PIDPATHINFO_MAXSIZE` is four times `MAXPATHLEN` and is not exported to
/// Swift, so the desktop component states the buffer bound itself.
private let executablePathBufferLimit = 4 * 1024

/// Diagnostics go to the system log after the launch confirmation is written.
/// The launch channel is used once and never again, so a closed pipe cannot
/// affect a running desktop component.
let petLog = Logger(subsystem: "com.imotong.taskbelay.pet", category: "desktop")

/// Every operating-system-specific process and file operation used by the
/// desktop component lives here. Other modules express product decisions and
/// stay free of Darwin calls.
enum NativeProcess {
    /// Process facts read from the operating system, never inferred from a PID
    /// or an application name alone.
    struct Identity: Equatable {
        let pid: Int32
        let startIdentity: String
        let executablePath: String
        let ownerUserID: uid_t
    }

    static func currentIdentity() -> Identity? {
        identity(pid: getpid())
    }

    /// Reads live process facts. InstanceRules compares them with the saved
    /// record to detect a recycled PID or a changed executable.
    static func identity(pid: Int32) -> Identity? {
        guard pid > 0 else { return nil }
        guard let startIdentity = processStartIdentity(pid: pid) else { return nil }
        guard let executablePath = executablePath(pid: pid) else { return nil }
        guard let ownerUserID = ownerUserID(pid: pid) else { return nil }
        return Identity(pid: pid, startIdentity: startIdentity, executablePath: executablePath, ownerUserID: ownerUserID)
    }

    /// Kernel creation time as decimal seconds:microseconds, independent of locale and timezone.
    static func processStartIdentity(pid: Int32) -> String? {
        guard pid > 0 else { return nil }
        var info = proc_bsdinfo()
        let size = Int32(MemoryLayout<proc_bsdinfo>.size)
        guard proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, size) == size,
              info.pbi_pid == UInt32(pid), info.pbi_start_tvsec > 0 else { return nil }
        return "\(info.pbi_start_tvsec):\(info.pbi_start_tvusec)"
    }

    static func executablePath(pid: Int32) -> String? {
        var buffer = [CChar](repeating: 0, count: executablePathBufferLimit)
        let length = proc_pidpath(pid, &buffer, UInt32(buffer.count))
        guard length > 0 else { return nil }
        return String(cString: buffer)
    }

    static func ownerUserID(pid: Int32) -> uid_t? {
        var info = kinfo_proc()
        var size = MemoryLayout<kinfo_proc>.stride
        var managementInfo = [CTL_KERN, KERN_PROC, KERN_PROC_PID, pid]
        let result = sysctl(&managementInfo, UInt32(managementInfo.count), &info, &size, nil, 0)
        guard result == 0, size > 0 else { return nil }
        return info.kp_eproc.e_pcred.p_ruid
    }

    /// Delivers a signal to a process. `SIGUSR1` asks an existing instance to
    /// show its window again; `SIGTERM` requests an orderly shutdown. Signals
    /// carry no Task or connection state.
    @discardableResult
    static func send(signal: Int32, to pid: Int32) -> Bool {
        guard pid > 0 else { return false }
        return kill(pid, signal) == 0
    }

    static func currentUserID() -> uid_t {
        getuid()
    }

    /**
     * ESRCH proves a process is gone; permission errors do not.
     */
    static func isGone(pid: Int32) -> Bool {
        pid > 0 && kill(pid, 0) != 0 && errno == ESRCH
    }

    /// Hands a page to the macOS default browser. Only the confirmed loopback
    /// origin is accepted, and a failure leaves the current picture untouched so
    /// the user can click again.
    @discardableResult
    static func openInBrowser(_ value: String) -> Bool {
        guard let url = URL(string: value),
              url.scheme == "http",
              url.host == "127.0.0.1",
              url.port.map({ $0 > 0 && $0 <= 65535 }) ?? false else {
            return false
        }
        return NSWorkspace.shared.open(url)
    }

    /// The system reduce-motion setting, which takes priority over the
    /// animation switch in the menu.
    static func reduceMotionEnabled() -> Bool {
        NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
    }

    /// Suppresses the pipe signal so writing to the closed launch channel can
    /// never terminate a running desktop component.
    static func ignorePipeSignal() {
        signal(SIGPIPE, SIG_IGN)
    }

}

/// An advisory lock that keeps exactly one desktop instance per user. The lock
/// is held for the lifetime of the process and released by the operating system
/// when the process ends, which is what makes an expired record detectable.
final class InstanceLock {
    let path: String
    private var descriptor: Int32 = -1

    init(path: String) {
        self.path = path
    }

    var isHeld: Bool { descriptor >= 0 }

    func acquire() -> Bool {
        guard descriptor < 0 else { return true }
        let handle = open(path, O_RDWR | O_CREAT, 0o600)
        guard handle >= 0 else { return false }
        guard flock(handle, LOCK_EX | LOCK_NB) == 0 else {
            close(handle)
            return false
        }
        descriptor = handle
        return true
    }

    func release() {
        guard descriptor >= 0 else { return }
        flock(descriptor, LOCK_UN)
        close(descriptor)
        descriptor = -1
    }

    deinit {
        release()
    }
}

/// Private-permission storage for the desktop component. JSON is written
/// atomically and read with a size bound; a missing file yields `nil` so callers
/// apply current defaults instead of reading a superseded format.
enum OwnedStorage {
    enum StorageError: LocalizedError, Equatable {
        case directoryUnavailable(String)
        case notARegularFile(String)
        case oversized(String)
        case writeFailed(String)

        var errorDescription: String? {
            switch self {
            case .directoryUnavailable(let path): return "Directory unavailable: \(path)"
            case .notARegularFile(let path): return "Expected a regular file inside the selected folder: \(path)"
            case .oversized(let path): return "File exceeds the supported size: \(path)"
            case .writeFailed(let path): return "Could not save file: \(path)"
            }
        }
    }

    static func ensurePrivateDirectory(_ path: String) throws {
        let manager = FileManager.default
        var isDirectory: ObjCBool = false
        if manager.fileExists(atPath: path, isDirectory: &isDirectory) {
            guard isDirectory.boolValue else { throw StorageError.directoryUnavailable(path) }
        } else {
            do {
                try manager.createDirectory(
                    atPath: path,
                    withIntermediateDirectories: true,
                    attributes: [.posixPermissions: 0o700]
                )
            } catch {
                throw StorageError.directoryUnavailable(path)
            }
        }
        try? manager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: path)
    }

    static func readBoundedData(_ path: String, limit: Int = 256 * 1024) throws -> Data? {
        let manager = FileManager.default
        guard manager.fileExists(atPath: path) else { return nil }
        let attributes = try manager.attributesOfItem(atPath: path)
        guard let type = attributes[.type] as? FileAttributeType, type == .typeRegular else {
            throw StorageError.notARegularFile(path)
        }
        guard let size = attributes[.size] as? NSNumber, size.intValue <= limit else {
            throw StorageError.oversized(path)
        }
        return try Data(contentsOf: URL(fileURLWithPath: path))
    }

    /// Reads a regular file inside a user-selected resource directory.
    static func readRelativeData(in directory: URL, path: String, limit: Int) throws -> Data {
        let components = path.split(separator: "/").filter { $0 != "." }
        guard !path.hasPrefix("/"), !path.contains("\\"), !path.contains("\0"), !components.isEmpty,
              components.allSatisfy({ $0 != ".." }) else {
            throw StorageError.notARegularFile(path)
        }
        var url = directory
        for component in components {
            url.appendPathComponent(String(component))
            let values = try url.resourceValues(forKeys: [.isSymbolicLinkKey])
            guard values.isSymbolicLink != true else { throw StorageError.notARegularFile(path) }
        }
        guard let data = try readBoundedData(url.path, limit: limit) else {
            throw StorageError.notARegularFile(path)
        }
        return data
    }

    static func writeAtomically(_ data: Data, to path: String) throws {
        let directory = (path as NSString).deletingLastPathComponent
        try ensurePrivateDirectory(directory)
        let temporary = "\(directory)/.\((path as NSString).lastPathComponent).\(getpid()).\(UUID().uuidString).tmp"
        let manager = FileManager.default
        guard manager.createFile(atPath: temporary, contents: data, attributes: [.posixPermissions: 0o600]) else {
            throw StorageError.writeFailed(path)
        }
        do {
            _ = try manager.replaceItemAt(URL(fileURLWithPath: path), withItemAt: URL(fileURLWithPath: temporary))
        } catch {
            try? manager.removeItem(atPath: temporary)
            throw StorageError.writeFailed(path)
        }
        try? manager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path)
    }

    static func remove(_ path: String) {
        try? FileManager.default.removeItem(atPath: path)
    }
}
