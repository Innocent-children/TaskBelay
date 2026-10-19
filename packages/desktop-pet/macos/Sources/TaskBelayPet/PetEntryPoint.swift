import AppKit
import Foundation

/// The native entry of the bundled desktop component.
///
/// `run` opens the window, or asks an instance that already uses the same Core
/// and data directory to show itself again. `stop` is the internal shutdown entry
/// used by `taskbelay pet stop` and by Adapter maintenance; it never creates a
/// window. Launch confirmation is a single `ready` or `restored` line on stdout,
/// written only after the window and the runtime record exist. Diagnostics
/// afterwards go to the system log, never back to the launcher.
@MainActor
@main
enum PetEntryPoint {
    static func main() {
        NativeProcess.ignorePipeSignal()
        let rawArguments = Array(CommandLine.arguments.dropFirst())
        let strings = PetStrings.forLanguage(PetLanguage.resolve())

        let arguments: LaunchArguments
        do {
            arguments = try LaunchArgumentParser.parse(rawArguments)
        } catch {
            // Double-clicking the bundled application without launch arguments
            // states the product dependency and exits.
            fail(strings.launchFromTaskBelay, code: 2)
        }

        switch arguments.mode {
        case .status:
            print(PetInstanceRuntime.status(productRoot: arguments.productRoot))
        case .stop:
            exit(PetStopRequest.run(productRoot: arguments.productRoot, corePath: arguments.corePath))
        case .run:
            runDesktop(arguments: arguments)
        }
    }

    private static func runDesktop(arguments: LaunchArguments) {
        let paths = PetPaths(productRoot: arguments.productRoot)
        let runtime = PetInstanceRuntime(paths: paths)
        let lockAcquired: Bool
        do {
            lockAcquired = try runtime.acquireLock()
        } catch {
            fail("cannot prepare \(paths.root)", code: 1)
        }
        let record = runtime.readRecord()
        let corePath = arguments.corePath ?? ""
        let coreIdentity = arguments.coreIdentity ?? ""
        let dataRootDigest = arguments.dataRootDigest ?? ""

        switch InstanceRules.decideRun(
            lockAcquired: lockAcquired,
            record: record,
            liveIdentity: record.flatMap { NativeProcess.identity(pid: Int32($0.pid)) },
            currentIdentity: NativeProcess.currentIdentity(),
            corePath: corePath,
            coreIdentity: coreIdentity,
            dataRootDigest: dataRootDigest
        ) {
        case .restore(let pid):
            // The signal carries only the request to become visible.
            guard NativeProcess.send(signal: SIGUSR1, to: pid) else {
                fail("cannot reach the running desktop pet", code: 1)
            }
            confirm("restored")
        case .conflict(let conflict):
            fail("the desktop pet already runs for a different \(conflict.rawValue)", code: 1)
        case .start:
            startDesktop(arguments: arguments, corePath: corePath, coreIdentity: coreIdentity,
                         dataRootDigest: dataRootDigest, paths: paths, runtime: runtime)
        }
    }

    private static func startDesktop(
        arguments: LaunchArguments,
        corePath: String,
        coreIdentity: String,
        dataRootDigest: String,
        paths: PetPaths,
        runtime: PetInstanceRuntime
    ) {
        let application = NSApplication.shared
        application.setActivationPolicy(.accessory)
        let delegate = PetAppDelegate(
            corePath: corePath,
            dataDirectory: arguments.dataDirectory ?? "",
            coreIdentity: coreIdentity,
            dataRootDigest: dataRootDigest,
            paths: paths,
            runtime: runtime
        )
        application.delegate = delegate
        application.run()
    }

    private static func confirm(_ line: String) {
        FileHandle.standardOutput.write(Data("\(line)\n".utf8))
        exit(0)
    }

    private static func fail(_ message: String, code: Int32) -> Never {
        FileHandle.standardError.write(Data("\(message)\n".utf8))
        exit(code)
    }
}

/// Completes activation, writes the runtime record, and installs the signal
/// handlers of a running instance.
@MainActor
final class PetAppDelegate: NSObject, NSApplicationDelegate {
    private let corePath: String
    private let dataDirectory: String
    private let coreIdentity: String
    private let dataRootDigest: String
    private let paths: PetPaths
    private let runtime: PetInstanceRuntime

    private var controller: PetController?
    private var signalSources: [DispatchSourceSignal] = []
    private var didFinishLaunching = false

    init(
        corePath: String,
        dataDirectory: String,
        coreIdentity: String,
        dataRootDigest: String,
        paths: PetPaths,
        runtime: PetInstanceRuntime
    ) {
        self.corePath = corePath
        self.dataDirectory = dataDirectory
        self.coreIdentity = coreIdentity
        self.dataRootDigest = dataRootDigest
        self.paths = paths
        self.runtime = runtime
        super.init()
    }

    /// Activation runs after the run loop is spinning so a slow Core command
    /// cannot block the main thread, and the launch confirmation is written only
    /// once the window and the runtime record exist.
    func applicationDidFinishLaunching(_ notification: Notification) {
        guard !didFinishLaunching else { return }
        didFinishLaunching = true

        let strings = PetStrings.forLanguage(PetLanguage.resolve())
        let core = CoreRuntimeClient(
            corePath: corePath,
            dataDirectory: dataDirectory,
            runner: SystemProcessRunner()
        )
        Task { @MainActor in
            let activation = await PetActivation.activate(
                core: core,
                expectedCoreIdentity: coreIdentity,
                expectedDataRootDigest: dataRootDigest
            )
            guard case .activated = activation else {
                FileHandle.standardError.write(
                    Data("\(PetActivation.description(activation, strings: strings))\n".utf8)
                )
                runtime.release()
                exit(1)
            }
            startController(core: core)
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    func applicationWillTerminate(_ notification: Notification) {
        controller?.shutdown()
    }

    private func startController(core: CoreRuntimeClient) {
        let preferences = PreferenceStore(path: paths.settings)
        let library = AssetLibrary.bundleResourceDirectory().flatMap { try? AssetLibrary(resourceDirectory: $0) }
        if library == nil {
            petLog.error("the bundled animation assets could not be read")
        }
        let controller = PetController(
            preferences: preferences,
            core: core,
            expectedCoreIdentity: coreIdentity,
            expectedDataRootDigest: dataRootDigest,
            library: library,
            runtime: runtime
        ) {
            NSApp.terminate(nil)
        }
        self.controller = controller
        do {
            try writeRuntimeRecord()
        } catch {
            FileHandle.standardError.write(Data("cannot register the desktop pet: \(error)\n".utf8))
            runtime.release()
            exit(1)
        }
        installSignalHandlers()
        controller.start()
        FileHandle.standardOutput.write(Data("ready\n".utf8))
        try? FileHandle.standardOutput.close()
        try? FileHandle.standardError.close()
    }

    private enum StartupFailure: Error { case processIdentityUnavailable }

    private func writeRuntimeRecord() throws {
        guard let identity = NativeProcess.currentIdentity() else {
            throw StartupFailure.processIdentityUnavailable
        }
        try runtime.writeRecord(InstanceRecord(
            pid: Int(identity.pid),
            processStartIdentity: identity.startIdentity,
            executablePath: identity.executablePath,
            corePath: corePath,
            coreIdentity: coreIdentity,
            dataRootDigest: dataRootDigest
        ))
    }

    /// `SIGUSR1` asks this instance to show its window again; `SIGTERM` requests
    /// the orderly shutdown. Neither signal carries Task or connection state.
    private func installSignalHandlers() {
        let restore = DispatchSource.makeSignalSource(signal: SIGUSR1, queue: .main)
        restore.setEventHandler { [weak self] in
            MainActor.assumeIsolated { self?.controller?.restoreVisibility() }
        }
        signal(SIGUSR1, SIG_IGN)
        restore.resume()

        let terminate = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
        terminate.setEventHandler { [weak self] in
            MainActor.assumeIsolated { self?.controller?.shutdown() }
        }
        signal(SIGTERM, SIG_IGN)
        terminate.resume()

        signalSources = [restore, terminate]
    }
}
