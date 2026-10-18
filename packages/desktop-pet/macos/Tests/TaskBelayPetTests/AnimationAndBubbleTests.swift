import AppKit
import Foundation
import XCTest
@testable import TaskBelayPet

/// Covers the delivered animation manifest, the playback rules, the asset
/// reader, and the bubble text.
///
/// Bundled and imported appearances share the same frame catalog. Static mode
/// covers the animation switch, the system
/// reduce-motion setting, and any first read of a terminal state, so
/// re-enabling animation never replays a prompt the user did not observe.
final class AnimationAndBubbleTests: XCTestCase {
    private var directories: [URL] = []

    override func tearDown() {
        for directory in directories {
            try? FileManager.default.removeItem(at: directory)
        }
        directories.removeAll()
        super.tearDown()
    }

    // MARK: - Manifest

    func testDecodesTheFiveDeliveredClips() throws {
        let catalog = try AnimationCatalog.decode(Self.catalogJSON(clips: Self.deliveredClips))
        XCTAssertEqual(Set(catalog.clips.keys), Set(AnimationCatalog.requiredClips))
        XCTAssertEqual(catalog.canvas, AnimationCatalog.Canvas(width: 512, height: 512))
        XCTAssertEqual(catalog.anchor, AnimationCatalog.Anchor(x: 0.5, y: 0))

        let idle = try XCTUnwrap(catalog.clips[.idle])
        XCTAssertEqual(idle.loopRange, 1...3)
        // Frames before the loop form the attention segment played once.
        XCTAssertEqual(idle.introRange, 0...0)
        XCTAssertEqual(idle.frameDuration, 1.0 / 24.0, accuracy: 0.0001)

        // The celebration is the only one-shot clip, so it carries no loop.
        let complete = try XCTUnwrap(catalog.clips[.complete])
        XCTAssertNil(complete.loopRange)
        XCTAssertNil(complete.introRange)
    }

    func testRejectsAnUnknownOrMissingClip() {
        var extra = Self.deliveredClips
        extra["celebrating"] = Self.clipJSON(frames: ["extra.png"], loopRange: [0, 0], restFrame: 0)
        XCTAssertThrowsError(try AnimationCatalog.decode(Self.catalogJSON(clips: extra)))

        var incomplete = Self.deliveredClips
        incomplete.removeValue(forKey: "blocked")
        XCTAssertThrowsError(try AnimationCatalog.decode(Self.catalogJSON(clips: incomplete)))
    }

    func testAdditionalArtworkClipsRoundTripAndAreValidated() throws {
        var clips = Self.deliveredClips
        for clip in [AnimationClip.runningRight, .runningLeft, .waving, .review] {
            clips[clip.rawValue] = Self.clipJSON(frames: ["\(clip.rawValue)/0.png"], loopRange: [0, 0], restFrame: 0)
        }
        let catalog = try AnimationCatalog.decode(Self.catalogJSON(clips: clips))
        XCTAssertEqual(Set(catalog.clips.keys), Set(AnimationClip.allCases))
        XCTAssertEqual(try AnimationCatalog.decode(JSONEncoder().encode(catalog)), catalog)

        clips["review"] = Self.clipJSON(frames: [], loopRange: [0, 0], restFrame: 0)
        XCTAssertThrowsError(try AnimationCatalog.decode(Self.catalogJSON(clips: clips))) { error in
            XCTAssertEqual(error as? AnimationCatalogError, .emptyFrames(.review))
        }
    }

    func testRejectsFramePathsThatCouldLeaveTheAssetRoot() {
        for frames in [["/etc/passwd"], ["../outside.png"], ["idle/../../secret.png"], [""]] {
            XCTAssertThrowsError(
                try AnimationCatalog.decode(
                    catalogWith(.idle, replacedBy: Self.clipJSON(frames: frames, loopRange: [0, 0], restFrame: 0))
                ),
                "\(frames) was accepted as a frame path"
            )
        }
    }

    func testRejectsEmptyFramesAndANonPositiveFrameRate() {
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.working, replacedBy: Self.clipJSON(frames: [], loopRange: [0, 0], restFrame: 0))
        ))
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.working, replacedBy: Self.clipJSON(frames: ["a.png"], fps: 0, loopRange: [0, 0], restFrame: 0))
        ))
    }

    func testRejectsIndexesOutsideTheFrameList() {
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.idle, replacedBy: Self.clipJSON(frames: ["a.png", "b.png"], loopRange: [0, 1], restFrame: 2))
        ))
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.idle, replacedBy: Self.clipJSON(frames: ["a.png", "b.png"], loopRange: [0, 2], restFrame: 0))
        ))
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.idle, replacedBy: Self.clipJSON(frames: ["a.png", "b.png"], loopRange: [1, 0], restFrame: 0))
        ))
    }

    func testEveryClipExceptTheCelebrationMustLoop() {
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.idle, replacedBy: Self.clipJSON(frames: ["a.png"], restFrame: 0))
        ))
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.complete, replacedBy: Self.clipJSON(frames: ["a.png"], loopRange: [0, 0], restFrame: 0))
        ))
    }

    func testRejectsALoopRangeThatIsNotTwoIndexesAndANonPositiveCanvas() {
        XCTAssertThrowsError(try AnimationCatalog.decode(
            catalogWith(.idle, replacedBy: #"{"frames":["a.png"],"fps":24,"loop_range":[1],"rest_frame":0}"#)
        ))
        XCTAssertThrowsError(try AnimationCatalog.decode(
            Self.catalogJSON(canvasWidth: 0, clips: Self.deliveredClips)
        ))
    }

    // MARK: - Playback

    @MainActor
    func testFiniteActivityPlaybackNotifiesAfterItsCompleteCycles() async throws {
        var clips = Self.deliveredClips
        clips["waving"] = Self.clipJSON(frames: ["wave.png"], fps: 120, loopRange: [0, 0], restFrame: 0)
        let directory = try makeResourceDirectory(files: ["animations.json": Self.catalogJSON(clips: clips),
            "Assets/wave.png": try XCTUnwrap(Self.png(width: 8, height: 8))])
        let view = PetCharacterView(frame: .zero)
        view.configure(library: try AssetLibrary(resourceDirectory: directory), strings: .english)
        let window = NSWindow(contentRect: NSRect(x: 100, y: 100, width: 144, height: 144),
            styleMask: [.borderless], backing: .buffered, defer: false)
        window.contentView = view
        window.isReleasedWhenClosed = false
        window.orderFrontRegardless()
        defer { view.stopPlayback(); window.orderOut(nil) }
        let finished = expectation(description: "finite clip finished")
        var completions = 0
        view.onPlaybackFinished = { clip in
            XCTAssertEqual(clip, .waving)
            completions += 1
            finished.fulfill()
        }
        XCTAssertTrue(view.play(clip: .waving, playback: .repeatThenRest(cycles: 2, restFrame: 0), restart: true))
        await fulfillment(of: [finished], timeout: 2)
        XCTAssertEqual(completions, 1)
        XCTAssertEqual(view.currentPlayback, .rest(frameIndex: 0))
    }

    @MainActor
    func testWindowWalkingReachesItsTargetAndCanBeStopped() async {
        let window = PetWindow()
        window.layout(atOrigin: CGPoint(x: 100, y: 100))
        defer { window.stopWalking() }
        let finished = expectation(description: "walking finished")
        window.onWalkingFinished = { finished.fulfill() }
        window.startWalking(toX: 120, duration: 0.05)
        XCTAssertTrue(window.isWalking)
        await fulfillment(of: [finished], timeout: 2)
        XCTAssertEqual(window.frame.minX, 120, accuracy: 0.1)
        XCTAssertEqual(window.frame.minY, 100, accuracy: 0.1)
        XCTAssertFalse(window.isWalking)

        let stopped = expectation(description: "stopped walk stays cancelled")
        stopped.isInverted = true
        window.onWalkingFinished = { stopped.fulfill() }
        window.startWalking(toX: 180, duration: 0.05)
        window.stopWalking()
        await fulfillment(of: [stopped], timeout: 0.15)
        XCTAssertEqual(window.frame.minX, 120, accuracy: 0.1)
        XCTAssertFalse(window.isWalking)
    }

    func testStaticModeAlwaysUsesTheDedicatedRestFrame() {
        let modes: [(animationsEnabled: Bool, reduceMotion: Bool, useRestFrame: Bool)] = [
            (false, false, false),
            (true, true, false),
            (true, false, true),
            (false, true, true),
        ]
        for mode in modes {
            XCTAssertEqual(
                PlaybackRules.playback(
                    clip: loopingClip,
                    playIntro: true,
                    useRestFrame: mode.useRestFrame,
                    animationsEnabled: mode.animationsEnabled,
                    reduceMotion: mode.reduceMotion
                ),
                .rest(frameIndex: 0),
                "\(mode) must not run an animation timer"
            )
        }
    }

    func testANewPromptPlaysTheAttentionSegmentThenTheQuietLoop() {
        XCTAssertEqual(
            PlaybackRules.playback(
                clip: loopingClip, playIntro: true, useRestFrame: false,
                animationsEnabled: true, reduceMotion: false
            ),
            .introThenLoop(intro: 0...0, loop: 1...3)
        )
    }

    func testAContinuingStateLoopsWithoutReplayingThePrompt() {
        XCTAssertEqual(
            PlaybackRules.playback(
                clip: loopingClip, playIntro: false, useRestFrame: false,
                animationsEnabled: true, reduceMotion: false
            ),
            .loop(1...3)
        )
    }

    func testAClipWithoutAnAttentionSegmentLoopsImmediately() {
        // A first read of a blocked Task enters the quiet loop instead of
        // alerting about a state the user did not observe changing.
        XCTAssertEqual(
            PlaybackRules.playback(
                clip: quietFromStartClip, playIntro: true, useRestFrame: false,
                animationsEnabled: true, reduceMotion: false
            ),
            .loop(0...1)
        )
    }

    func testTheCelebrationPlaysOnceAndThenHoldsItsRestFrame() {
        XCTAssertEqual(
            PlaybackRules.playback(
                clip: oneShotClip, playIntro: true, useRestFrame: false,
                animationsEnabled: true, reduceMotion: false
            ),
            .onceThenRest(lastFrameIndex: 2, restFrame: 2)
        )
        // A first read of a finished Task is not an observed change.
        XCTAssertEqual(
            PlaybackRules.playback(
                clip: oneShotClip, playIntro: false, useRestFrame: false,
                animationsEnabled: true, reduceMotion: false
            ),
            .rest(frameIndex: 2)
        )
    }

    // MARK: - Asset reader

    func testBundledAppearanceLoadsEveryFrame() throws {
        let source = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .appendingPathComponent("../../../default-appearance", isDirectory: true).standardizedFileURL
        let library = try AssetLibrary(resourceDirectory: source)
        XCTAssertEqual(library.catalog.canvas, .init(width: 192, height: 208))
        XCTAssertEqual(Set(library.catalog.clips.keys), Set(AnimationClip.allCases))
        XCTAssertEqual(library.catalog.clips.values.reduce(0) { $0 + $1.frames.count }, 57)

        for clip in AnimationClip.allCases {
            let description = try XCTUnwrap(library.catalog.clips[clip])
            let frames = try library.frames(for: clip)
            XCTAssertEqual(frames.images.count, description.frames.count, clip.rawValue)
            for (index, image) in frames.images.enumerated() {
                let rendered = try XCTUnwrap(image.cgImage(forProposedRect: nil, context: nil, hints: nil),
                                            description.frames[index])
                XCTAssertEqual(rendered.width, library.catalog.canvas.width, description.frames[index])
                XCTAssertEqual(rendered.height, library.catalog.canvas.height, description.frames[index])
            }
        }
    }

    @MainActor
    func testScalingResizesCharacterAndKeepsBubbleTypographyAndAnchorGeometry() throws {
        let clips = Dictionary(uniqueKeysWithValues: AnimationCatalog.requiredClips.map {
            ($0, AnimationCatalog.Clip(frames: ["frame.svg"], fps: 24,
                loopRange: $0 == .complete ? nil : 0...0, restFrame: 0))
        })
        let catalog = AnimationCatalog(canvas: .init(width: 256, height: 288), anchor: .init(x: 128, y: 264), clips: clips)
        let library = try AssetLibrary(catalog: catalog, assetRoot: FileManager.default.temporaryDirectory)
        let window = PetWindow()
        window.content.character.configure(library: library, strings: .english)
        let originalSize = window.content.requiredSize
        XCTAssertEqual(window.content.referencePoint(atScale: 1), CGPoint(x: originalSize.width / 2, y: 12))
        window.content.setScale(2)
        window.layout(atOrigin: CGPoint(x: 100, y: 100))
        window.content.layoutSubtreeIfNeeded()
        XCTAssertEqual(window.content.character.bounds.size, CGSize(width: 288, height: 288))
        XCTAssertEqual(window.content.bubble.bounds.width, PetBubbleStackView.bubbleWidth)
        XCTAssertEqual(window.content.requiredSize.height, originalSize.height + 144)
        XCTAssertEqual(window.content.referencePoint(atScale: 2), CGPoint(x: max(PetBubbleStackView.bubbleWidth, 288) / 2, y: 24))
        window.content.setScale(0.5)
        window.layout(atOrigin: CGPoint(x: 100, y: 100))
        window.content.layoutSubtreeIfNeeded()
        XCTAssertEqual(window.content.character.bounds.size, CGSize(width: 72, height: 72))
        XCTAssertEqual(window.content.bubble.bounds.width, PetBubbleStackView.bubbleWidth)
    }

    func testAssetLibraryRequiresADeliveredCatalog() {
        XCTAssertThrowsError(try AssetLibrary(resourceDirectory: makeResourceDirectory(files: [:]))) { error in
            XCTAssertEqual(error as? AssetError, .catalogMissing)
        }
    }

    func testAssetLibraryRejectsAnIncompleteCatalog() throws {
        var incomplete = Self.deliveredClips
        incomplete.removeValue(forKey: "disconnected")
        let directory = try makeResourceDirectory(files: [
            "animations.json": Self.catalogJSON(clips: incomplete),
        ])
        XCTAssertThrowsError(try AssetLibrary(resourceDirectory: directory)) { error in
            guard case .catalogInvalid = error as? AssetError else {
                return XCTFail("an incomplete catalog must be reported as invalid, got \(error)")
            }
        }
    }

    func testAssetLibraryFailsWhenADeliveredFrameIsAbsent() throws {
        let directory = try makeResourceDirectory(files: [
            "animations.json": Self.catalogJSON(clips: Self.singleFrameClips),
        ])
        let library = try AssetLibrary(resourceDirectory: directory)
        XCTAssertThrowsError(try library.frames(for: .idle)) { error in
            XCTAssertEqual(error as? AssetError, .frameMissing(.idle, "idle/00.png"))
        }
    }

    func testAssetLibraryDecodesThePlayingClipAtTheCanvasSize() throws {
        var files: [String: Data] = ["animations.json": Self.catalogJSON(clips: Self.singleFrameClips)]
        for name in AnimationCatalog.requiredClips {
            files["Assets/\(name.rawValue)/00.png"] = try XCTUnwrap(Self.png(width: 8, height: 8))
        }
        let library = try AssetLibrary(resourceDirectory: makeResourceDirectory(files: files))
        XCTAssertEqual(library.canvasSize, CGSize(width: 512, height: 512))

        let idle = try library.frames(for: .idle)
        XCTAssertEqual(idle.clip, .idle)
        XCTAssertEqual(idle.images.count, 1)
        // Frames are drawn at the canvas size, so a delivered pixel size never
        // decides the on-screen character size.
        XCTAssertEqual(idle.image(at: 0)?.size, CGSize(width: 512, height: 512))
        XCTAssertNil(idle.image(at: 1))

        // Only the playing clip is retained, so a desktop that stays open for
        // days does not hold every delivered frame in memory.
        XCTAssertEqual(try library.frames(for: .working).clip, .working)
        library.releaseFrames()
        XCTAssertEqual(try library.frames(for: .idle).clip, .idle)
    }

    // MARK: - Bubble text

    func testTheStageLineCarriesTheNodeLabelCoreReports() {
        let summary = TestFixtures.summary(currentNode: "ACTIVE")
        let chinese = content(phase: .working(node: "ACTIVE"), summary: summary, language: .chinese)
        XCTAssertEqual(chinese.title, "Add the desktop pet")
        XCTAssertEqual(chinese.stage, "开发与验证")
        XCTAssertNil(chinese.blocker)

        let english = content(phase: .working(node: "ACTIVE"), summary: summary, language: .english)
        XCTAssertEqual(english.stage, "Development and verification")
    }

    func testTheWorkingThemeMakesNoClaimAboutTheHost() {
        // The stage line is the node Core reports, so an unverifiable narrative
        // such as "the host is executing" cannot appear in the working theme.
        let summary = TestFixtures.summary(currentNode: "ACTIVE")
        for language in [PetLanguage.chinese, .english] {
            let strings = PetStrings.forLanguage(language)
            XCTAssertEqual(
                content(phase: .working(node: "ACTIVE"), summary: summary, language: language).stage,
                strings.nodeName("ACTIVE")
            )
        }
    }

    func testBlockedShowsTheCoreReasonAndOnlyFallsBackWithoutOne() {
        let blocked = TestFixtures.summary(
            currentNode: "BLOCKED", lifecycle: .blocked, blocker: "waiting for the repository claim"
        )
        let withReason = content(phase: .blocked(node: "BLOCKED"), summary: blocked, language: .english)
        XCTAssertEqual(withReason.stage, "Blocked")
        XCTAssertEqual(withReason.blocker, "waiting for the repository claim")

        // A missing blocker text is reported as a block; the reason is never
        // invented.
        let withoutReason = content(
            phase: .blocked(node: "BLOCKED"),
            summary: TestFixtures.summary(currentNode: "BLOCKED", lifecycle: .blocked),
            language: .english
        )
        XCTAssertEqual(withoutReason.blocker, "The task is blocked")
    }

    func testDisconnectedKeepsTheLastRecordAndMarksIt() {
        let stale = PresentationRules.Result(
            phase: .disconnected,
            clip: .disconnected,
            playIntro: false,
            useRestFrame: false,
            summary: TestFixtures.summary(currentNode: "ACTIVE"),
            isStaleSummary: true,
            detailReadiness: nil
        )
        let content = BubbleRules.content(
            result: stale,
            lastSyncAt: Date(timeIntervalSince1970: 1_788_000_500),
            strings: PetStrings.english,
            language: .english
        )
        // A stored node is marked as the last record instead of reading as live
        // progress.
        XCTAssertEqual(content.stage, "Not connected · Last record")
        XCTAssertEqual(content.blocker, "Cannot reach the local TaskBelay service")
        XCTAssertNotNil(content.taskUpdated)
    }

    func testDisconnectedWithoutARecordOffersTheChooser() {
        let offline = PresentationRules.Result(
            phase: .disconnected,
            clip: .disconnected,
            playIntro: false,
            useRestFrame: false,
            summary: nil,
            isStaleSummary: false,
            detailReadiness: nil
        )
        let content = BubbleRules.content(
            result: offline, lastSyncAt: nil, strings: PetStrings.english, language: .english
        )
        XCTAssertEqual(content.title, "Choose a task")
        XCTAssertEqual(content.stage, "Not connected")
        // Without a record there is nothing stale to show, but the expanded
        // body still explains the failure.
        XCTAssertNil(content.summary)
        XCTAssertNil(content.taskUpdated)
        XCTAssertEqual(content.blocker, "Cannot reach the local TaskBelay service")
    }

    func testReadOnlyIsAnAdditionalMarkAndNeverReplacesTheStage() {
        XCTAssertEqual(
            content(
                phase: .working(node: "ACTIVE"),
                summary: TestFixtures.summary(currentNode: "ACTIVE"),
                detailReadiness: .readOnly,
                language: .english
            ).stage,
            "Implementation · View only"
        )
    }

    func testTerminalArchivedAndMissingPhasesShowTheirOwnLabel() {
        XCTAssertEqual(
            content(
                phase: .completed,
                summary: TestFixtures.summary(currentNode: "DONE", lifecycle: .done),
                language: .english
            ).stage,
            "Done"
        )
        XCTAssertEqual(
            content(
                phase: .cancelled,
                summary: TestFixtures.summary(currentNode: "CANCELLED", lifecycle: .cancelled),
                language: .english
            ).stage,
            "Cancelled"
        )
        XCTAssertEqual(
            content(phase: .archived, summary: TestFixtures.summary(archived: true), language: .english).stage,
            "Archived"
        )

        let missing = content(phase: .taskMissing, summary: nil, language: .chinese)
        XCTAssertEqual(missing.title, "选择一个任务")
        XCTAssertEqual(missing.stage, "任务已不可用")
    }

    func testTheTwoTimestampsStaySeparate() throws {
        let content = self.content(
            phase: .working(node: "ACTIVE"),
            summary: TestFixtures.summary(currentNode: "ACTIVE"),
            lastSyncAt: Date(timeIntervalSince1970: 1_788_000_500),
            language: .english
        )
        let updated = try XCTUnwrap(content.taskUpdated)
        let sync = try XCTUnwrap(content.lastSync)
        XCTAssertTrue(updated.hasPrefix("Task updated "))
        XCTAssertTrue(sync.hasPrefix("Last sync "))
        XCTAssertNotEqual(updated, sync)
        XCTAssertTrue(content.hasExpandedContent)
    }

    func testWithoutASelectionOrASyncThereIsNoSecondLine() {
        let content = self.content(phase: .noSelection, summary: nil, language: .english)
        XCTAssertEqual(content.title, "Choose a task")
        XCTAssertNil(content.stage)
        XCTAssertNil(content.taskUpdated)
        XCTAssertNil(content.lastSync)
        XCTAssertFalse(content.hasExpandedContent)
    }

    func testBothLanguagesCoverTheSameNodeIdentifiers() {
        XCTAssertEqual(Set(PetStrings.chinese.nodes.keys), Set(PetStrings.english.nodes.keys))
        for (identifier, label) in PetStrings.chinese.nodes {
            XCTAssertFalse(label.trimmingCharacters(in: .whitespaces).isEmpty, "\(identifier) has no Chinese label")
        }
        for (identifier, label) in PetStrings.english.nodes {
            XCTAssertFalse(label.trimmingCharacters(in: .whitespaces).isEmpty, "\(identifier) has no English label")
        }
        // An identifier Core adds later is shown as delivered rather than
        // hidden behind a placeholder.
        XCTAssertEqual(PetStrings.english.nodeName("SOMETHING_NEW"), "SOMETHING_NEW")
    }

    // MARK: - Helpers

    @MainActor
    func testAnimationSwitchChangesPlaybackWithinTheSameClip() throws {
        var files = ["animations.json": Self.catalogJSON(clips: Self.singleFrameClips)]
        for clip in AnimationCatalog.requiredClips {
            files["Assets/\(clip.rawValue)/00.png"] = try XCTUnwrap(Self.png(width: 8, height: 8))
        }
        let view = PetCharacterView(frame: .zero)
        view.configure(library: try AssetLibrary(resourceDirectory: makeResourceDirectory(files: files)), strings: .english)
        view.play(clip: .idle, playback: .loop(0...0), restart: false)
        view.play(clip: .idle, playback: .rest(frameIndex: 0), restart: false)
        XCTAssertEqual(view.currentPlayback, .rest(frameIndex: 0))
        view.play(clip: .idle, playback: .loop(0...0), restart: false)
        XCTAssertEqual(view.currentPlayback, .loop(0...0))
        view.pausePlayback()
        XCTAssertNil(view.currentPlayback)
        view.stopPlayback()
    }

    @MainActor
    func testTaskPickerHasAVisibleColumnAndDismissesOnce() throws {
        _ = NSApplication.shared
        let panel = TaskPickerPanel(session: 1)
        defer { panel.close() }
        let scroll = try XCTUnwrap(panel.contentView?.subviews.compactMap { $0 as? NSScrollView }.first)
        let table = try XCTUnwrap(scroll.documentView as? NSTableView)
        XCTAssertEqual(table.numberOfColumns, 1)
        var dismissals = 0
        panel.onDismiss = { dismissals += 1 }
        panel.dismiss()
        panel.close()
        XCTAssertEqual(dismissals, 1)
    }

    @MainActor
    func testLongBubbleContentKeepsABoundedHeight() {
        let view = PetBubbleView(frame: .zero)
        let longText = String(repeating: "long task description ", count: 500)
        view.update(BubbleContent(title: longText, stage: "Development and verification", summary: longText,
                                  taskUpdated: "Task updated", lastSync: "Last sync", blocker: longText))
        view.setExpanded(true)
        XCTAssertLessThan(view.requiredHeight(width: 220), 300)
    }

    @MainActor
    func testBriefHoverCancelsExpansionAndSettledHoverFinishesResize() async {
        let window = PetWindow()
        let tasks = PetTaskCollection()
        tasks.update([TestFixtures.summary(taskID: "hover")], readiness: .ready)
        window.content.bubble.update(cards: tasks.cards, pinned: nil, sync: nil, strings: .english, language: .english)
        window.content.bubble.update(BubbleContent(title: "Hover transition", stage: "Development and verification",
            summary: "Hover transition details", taskUpdated: "Updated", lastSync: "Synced", blocker: nil))
        window.layout(atOrigin: CGPoint(x: 100, y: 100))
        window.orderFrontRegardless()
        defer { window.stopBubbleTransitions(); window.orderOut(nil) }
        window.setBubbleHovered(true)
        XCTAssertFalse(window.content.bubble.isExpanded)
        let scroll = window.content.bubble.subviews.compactMap { $0 as? NSScrollView }.first
        XCTAssertEqual(scroll?.hasVerticalScroller, false)
        window.setBubbleHovered(false)
        try? await Task.sleep(nanoseconds: 350_000_000)
        XCTAssertFalse(window.content.bubble.isExpanded)
        window.setBubbleHovered(true)
        try? await Task.sleep(nanoseconds: 900_000_000)
        XCTAssertTrue(window.content.bubble.isExpanded)
        XCTAssertEqual(scroll?.hasVerticalScroller, false)
        XCTAssertEqual(window.frame.height, window.content.requiredSize.height, accuracy: 1)
        window.setBubbleHovered(false)
        try? await Task.sleep(nanoseconds: 750_000_000)
        XCTAssertFalse(window.content.bubble.isExpanded)
        XCTAssertEqual(window.frame.height, window.content.requiredSize.height, accuracy: 1)
        window.setBubbleHovered(true)
        window.content.bubble.update(cards: [], pinned: nil, sync: nil, strings: .english, language: .english)
        window.relayoutForBubble(animated: true)
        try? await Task.sleep(nanoseconds: 750_000_000)
        XCTAssertTrue(window.content.bubble.isHidden)
        XCTAssertFalse(window.content.bubble.isExpanded, "pending hover cannot restore an empty bubble")
        XCTAssertEqual(window.frame.height, 144, accuracy: 1)
        window.setBubbleHovered(false)
        try? await Task.sleep(nanoseconds: 750_000_000)
        XCTAssertFalse(window.content.bubble.isExpanded)
        XCTAssertEqual(window.frame.height, window.content.requiredSize.height, accuracy: 1)
    }

    @MainActor
    func testCollapsedBubbleLaysOutBothResidentLines() throws {
        let view = PetBubbleView(frame: .zero)
        view.update(BubbleContent(title: "A task", stage: "已取消", summary: nil,
                                  taskUpdated: nil, lastSync: nil, blocker: nil))
        view.setFrameSize(NSSize(width: 220, height: view.requiredHeight(width: 220)))
        view.layoutSubtreeIfNeeded()
        func textFields(in node: NSView) -> [NSTextField] {
            node.subviews.flatMap { child in (child as? NSTextField).map { [$0] } ?? textFields(in: child) }
        }
        let labels = textFields(in: view)
        if #available(macOS 27.0, *) {
            let glass = try XCTUnwrap(view.subviews.compactMap { $0 as? NSGlassEffectView }.first)
            XCTAssertTrue(glass.effectIsInteractive)
            XCTAssertEqual(glass.cornerRadius, 21)
            XCTAssertEqual(glass.style, .regular)
            XCTAssertNil(glass.tintColor)
            XCTAssertEqual(glass.alphaValue, 1)
            XCTAssertFalse(view.subviews.contains { $0 is NSVisualEffectView })
        }
        let stage = try XCTUnwrap(labels.first { $0.stringValue == "已取消" })
        XCTAssertFalse(stage.isHidden)
        XCTAssertGreaterThan(stage.frame.height, 10)
        XCTAssertGreaterThanOrEqual(stage.frame.minY, 0)
    }

    @MainActor
    func testTaskPickerPagesAndSelectsWithReturn() async throws {
        _ = NSApplication.shared
        let panel = TaskPickerPanel(session: 1)
        defer { panel.close() }
        let scroll = try XCTUnwrap(panel.contentView?.subviews.compactMap { $0 as? NSScrollView }.first)
        let table = try XCTUnwrap(scroll.documentView as? TaskPickerTableView)
        var pages: [Int] = []
        var chosen: String?
        panel.onLoadPage = { page in
            pages.append(page)
            let summaries = page == 1
                ? (0..<50).map { TestFixtures.summary(taskID: "task-\($0)") }
                : [TestFixtures.summary(taskID: "task-next")]
            return .value(TestDetails.list(page: page, hasNext: page == 1, summaries: summaries))
        }
        panel.onChoose = { chosen = $0 }
        panel.loadFirstPage()
        for _ in 0..<100 where table.numberOfRows != 51 {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(table.numberOfRows, 51)
        table.selectRowIndexes(IndexSet(integer: 50), byExtendingSelection: false)
        table.onReturnKey?()
        for _ in 0..<100 where pages.count != 2 {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(pages, [1, 2])
        table.selectRowIndexes(IndexSet(integer: 50), byExtendingSelection: false)
        table.onReturnKey?()
        XCTAssertEqual(chosen, "task-next")
    }

    private let loopingClip = AnimationCatalog.Clip(
        frames: ["a", "b", "c", "d"], fps: 24, loopRange: 1...3, restFrame: 0
    )
    private let quietFromStartClip = AnimationCatalog.Clip(
        frames: ["a", "b"], fps: 24, loopRange: 0...1, restFrame: 1
    )
    private let oneShotClip = AnimationCatalog.Clip(
        frames: ["a", "b", "c"], fps: 24, loopRange: nil, restFrame: 2
    )

    private func content(
        phase: DisplayPhase,
        summary: DesktopTaskSummary?,
        detailReadiness: Readiness? = .ready,
        lastSyncAt: Date? = nil,
        language: PetLanguage
    ) -> BubbleContent {
        BubbleRules.content(
            result: PresentationRules.Result(
                phase: phase,
                clip: .idle,
                playIntro: false,
                useRestFrame: false,
                summary: summary,
                isStaleSummary: false,
                detailReadiness: summary == nil ? nil : detailReadiness
            ),
            lastSyncAt: lastSyncAt,
            strings: PetStrings.forLanguage(language),
            language: language
        )
    }

    private func catalogWith(_ clip: AnimationClip, replacedBy json: String) -> Data {
        var clips = Self.deliveredClips
        clips[clip.rawValue] = json
        return Self.catalogJSON(clips: clips)
    }

    private func makeResourceDirectory(files: [String: Data]) throws -> URL {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("taskbelay-pet-assets-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        directories.append(root)
        for (relativePath, data) in files {
            let url = root.appendingPathComponent(relativePath)
            try FileManager.default.createDirectory(
                at: url.deletingLastPathComponent(), withIntermediateDirectories: true
            )
            try data.write(to: url)
        }
        return root
    }

    /// The delivered five-clip set with an attention segment on `idle`.
    private static var deliveredClips: [String: String] {
        [
            "idle": clipJSON(
                frames: ["idle/00.png", "idle/01.png", "idle/02.png", "idle/03.png"],
                loopRange: [1, 3],
                restFrame: 0
            ),
            "working": clipJSON(frames: ["working/00.png", "working/01.png"], loopRange: [0, 1], restFrame: 0),
            "blocked": clipJSON(
                frames: ["blocked/00.png", "blocked/01.png", "blocked/02.png"],
                loopRange: [2, 2],
                restFrame: 0
            ),
            "complete": clipJSON(frames: ["complete/00.png", "complete/01.png"], restFrame: 1),
            "disconnected": clipJSON(
                frames: ["disconnected/00.png", "disconnected/01.png"],
                loopRange: [0, 1],
                restFrame: 0
            ),
        ]
    }

    /// The same five clips with one frame each, used where the check needs the
    /// frames to exist on disk.
    private static var singleFrameClips: [String: String] {
        [
            "idle": clipJSON(frames: ["idle/00.png"], loopRange: [0, 0], restFrame: 0),
            "working": clipJSON(frames: ["working/00.png"], loopRange: [0, 0], restFrame: 0),
            "blocked": clipJSON(frames: ["blocked/00.png"], loopRange: [0, 0], restFrame: 0),
            "complete": clipJSON(frames: ["complete/00.png"], restFrame: 0),
            "disconnected": clipJSON(frames: ["disconnected/00.png"], loopRange: [0, 0], restFrame: 0),
        ]
    }

    private static func clipJSON(
        frames: [String],
        fps: Double = 24,
        loopRange: [Int]? = nil,
        restFrame: Int = 0
    ) -> String {
        let frameList = frames.map { "\"\($0)\"" }.joined(separator: ",")
        let loop = loopRange.map { "[\($0[0]),\($0[1])]" } ?? "null"
        return #"{"frames":[\#(frameList)],"fps":\#(fps),"loop_range":\#(loop),"rest_frame":\#(restFrame)}"#
    }

    private static func catalogJSON(
        canvasWidth: Int = 512,
        canvasHeight: Int = 512,
        clips: [String: String]
    ) -> Data {
        let body = clips.map { "\"\($0.key)\":\($0.value)" }.joined(separator: ",")
        return Data("""
        {"canvas":{"width":\(canvasWidth),"height":\(canvasHeight)},\
        "anchor":{"x":0.5,"y":0.0},"clips":{\(body)}}
        """.utf8)
    }

    private static func png(width: Int, height: Int) -> Data? {
        guard let representation = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: width,
            pixelsHigh: height,
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ) else { return nil }
        return representation.representation(using: .png, properties: [:])
    }
}
