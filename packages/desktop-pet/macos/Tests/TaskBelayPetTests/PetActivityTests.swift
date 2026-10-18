import Foundation
import XCTest
@testable import TaskBelayPet

final class PetActivityTests: XCTestCase {
    private final class Clock { var time = 0.0 }
    private final class Random { var value = 0.0 }
    private var clock: Clock!
    private var random: Random!
    private var activities: PetActivityController!

    override func setUp() {
        clock = Clock()
        random = Random()
        let clock = clock!, random = random!
        activities = PetActivityController(now: { clock.time }, random: { random.value })
        activities.configure(catalog: catalog())
        activities.geometry = .init(originX: 300, walkingRange: 180...420)
    }

    func testGreetingFinishesAndOrdinaryPollsPreserveTheIdleDeadline() throws {
        update(.noSelection, .idle)
        XCTAssertEqual(activities.currentActivity, .waving)
        XCTAssertEqual(activities.playback?.playback, .repeatThenRest(cycles: 1, restFrame: 0))
        clock.time = 0.2
        activities.playbackFinished(.waving)
        let request = activities.playback
        let deadline = try XCTUnwrap(activities.nextDeadline)
        XCTAssertEqual(deadline, 6.2, accuracy: 0.001)
        clock.time = 3
        update(.noSelection, .idle)
        XCTAssertEqual(activities.playback, request)
        XCTAssertEqual(activities.nextDeadline, deadline)
        clock.time = deadline
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .runningRight)
        XCTAssertEqual(activities.walk?.targetX, 340)
        XCTAssertEqual(activities.walk?.duration, 2)
    }

    func testWalkingCanFinishWithOneWaveAndThenReturnToIdle() throws {
        enterIdleWithoutGreeting()
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .runningRight)
        clock.time += 2
        activities.geometry.originX = 340
        activities.walkingFinished()
        XCTAssertNil(activities.walk)
        XCTAssertEqual(activities.currentActivity, .waving)
        clock.time += 0.2
        activities.playbackFinished(.waving)
        XCTAssertEqual(activities.playback?.clip, .idle)
        XCTAssertNotNil(activities.nextDeadline)
    }

    func testWalkingSpeedTracksVisualScale() throws {
        activities.geometry.scale = 2
        enterIdleWithoutGreeting()
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .runningRight)
        XCTAssertEqual(activities.walk?.targetX, 340)
        XCTAssertEqual(activities.walk?.duration, 1)
    }

    func testHoverStopsWalkingAndWavesOnceWithSharedCooldown() throws {
        enterIdleWithoutGreeting()
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        activities.windowHoverChanged(true)
        XCTAssertNil(activities.walk)
        XCTAssertEqual(activities.playback?.clip, .idle)
        XCTAssertNil(activities.nextDeadline)
        activities.characterHoverChanged(true)
        clock.time += 0.4
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .waving)
        clock.time += 0.2
        activities.playbackFinished(.waving)
        activities.characterHoverChanged(true)
        XCTAssertNil(activities.nextDeadline)
        activities.dropped()
        XCTAssertNil(activities.currentActivity)

        activities.characterHoverChanged(false)
        clock.time += 0.5
        activities.characterHoverChanged(true)
        XCTAssertNil(activities.nextDeadline)
        activities.characterHoverChanged(false)
        clock.time += 21
        activities.characterHoverChanged(true)
        clock.time += 0.4
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .waving)
    }

    func testBubbleHoverPausesRandomActivitiesWithoutWaving() {
        enterIdleWithoutGreeting()
        activities.windowHoverChanged(true)
        clock.time = 100
        activities.tick()
        XCTAssertNil(activities.currentActivity)
        XCTAssertNil(activities.nextDeadline)
        activities.windowHoverChanged(false)
        XCTAssertEqual(activities.nextDeadline, 106)
    }

    func testReviewUsesWholeCyclesAndIdleDoesNotChangeTheTaskPresentation() throws {
        enterIdleWithoutGreeting()
        random.value = 0.99
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .review)
        guard case .repeatThenRest(let cycles, _) = activities.playback?.playback else { return XCTFail("review must finish full cycles") }
        XCTAssertGreaterThanOrEqual(Double(cycles) * 0.2, 3)
        XCTAssertLessThanOrEqual(Double(cycles) * 0.2, 5.2)
        clock.time += Double(cycles) * 0.2
        activities.playbackFinished(.review)
        XCTAssertEqual(activities.playback?.clip, .idle)
    }

    func testUnavailableDirectionAndLastChoiceAreExcluded() throws {
        enterIdleWithoutGreeting()
        activities.geometry = .init(originX: 380, walkingRange: 180...384)
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertEqual(activities.currentActivity, .runningLeft)
        random.value = 0.9
        activities.geometry.originX = try XCTUnwrap(activities.walk?.targetX)
        activities.walkingFinished()
        XCTAssertNil(activities.currentActivity)
        random.value = 0
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertNotEqual(activities.currentActivity, .runningLeft)
    }

    func testTaskAlertsImmediatelyInterruptIdleAndWalking() throws {
        for (phase, clip) in [(DisplayPhase.blocked(node: "ACTIVE"), AnimationClip.blocked), (.disconnected, .disconnected)] {
            enterIdleWithoutGreeting()
            clock.time = try XCTUnwrap(activities.nextDeadline)
            activities.tick()
            XCTAssertNotNil(activities.walk)
            update(phase, clip)
            XCTAssertNil(activities.currentActivity)
            XCTAssertNil(activities.walk)
            XCTAssertNil(activities.nextDeadline)
            XCTAssertEqual(activities.playback?.clip, clip)
        }
    }

    func testCompletionFinishesBeforeTheThreeSecondRestAndDoesNotReplay() throws {
        update(.working(node: "ACTIVE"), .working)
        update(.completed, .complete, intro: true)
        let celebration = activities.playback
        XCTAssertEqual(celebration?.playback, .onceThenRest(lastFrameIndex: 1, restFrame: 0))
        clock.time = 3
        update(.completed, .complete, rest: true)
        XCTAssertEqual(activities.playback, celebration)
        XCTAssertNil(activities.nextDeadline)
        activities.playbackFinished(.complete)
        XCTAssertEqual(activities.nextDeadline, 6)
        clock.time = 4
        update(.completed, .complete, rest: true)
        XCTAssertEqual(activities.nextDeadline, 6)
        clock.time = 6
        activities.tick()
        XCTAssertEqual(activities.playback?.clip, .idle)
        XCTAssertNil(activities.currentActivity)
    }

    func testFirstReadOfDoneNeverCelebratesAndOldTerminalDraggingDoesNotRestartTheRest() throws {
        update(.completed, .complete, rest: true)
        XCTAssertEqual(activities.playback?.playback, .rest(frameIndex: 0))
        clock.time = 3
        activities.tick()
        activities.playbackFinished(.waving)
        clock.time = 30
        update(.completed, .complete, rest: true, controls: .init(dragging: true))
        XCTAssertNil(activities.playback)
        update(.completed, .complete, rest: true)
        activities.dropped()
        XCTAssertEqual(activities.currentActivity, .waving)
    }

    func testMenusAnimationControlsAndVisibilityCancelPendingActivities() throws {
        let suspended: [PetActivityController.Controls] = [.init(interactionBlocked: true), .init(animationsEnabled: false),
            .init(idleActivitiesEnabled: false), .init(reduceMotion: true), .init(visible: false), .init(dragging: true)]
        for controls in suspended {
            enterIdleWithoutGreeting()
            clock.time = try XCTUnwrap(activities.nextDeadline)
            activities.tick()
            update(.noSelection, .idle, controls: controls)
            XCTAssertNil(activities.currentActivity)
            XCTAssertNil(activities.walk)
            XCTAssertNil(activities.nextDeadline)
        }
    }

    func testFiveClipPacksKeepTaskPlaybackWithoutBackgroundIdleTimers() {
        activities.configure(catalog: catalog(additional: false))
        update(.noSelection, .idle)
        XCTAssertEqual(activities.playback?.clip, .idle)
        XCTAssertNil(activities.nextDeadline)
        update(.working(node: "ACTIVE"), .working)
        XCTAssertEqual(activities.playback?.clip, .working)
        activities.configure(catalog: catalog())
        update(.working(node: "ACTIVE"), .working)
        XCTAssertEqual(activities.playback?.clip, .working)
        XCTAssertNil(activities.nextDeadline)
    }

    func testFailedOptionalArtworkStopsMovementAndIsNotSelectedAgain() throws {
        enterIdleWithoutGreeting()
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        activities.playbackFailed(.runningRight)
        XCTAssertNil(activities.walk)
        XCTAssertEqual(activities.playback?.clip, .idle)
        clock.time = try XCTUnwrap(activities.nextDeadline)
        activities.tick()
        XCTAssertNotEqual(activities.currentActivity, .runningRight)
    }

    func testUnreadableCelebrationReleasesTheCompletionWait() {
        update(.working(node: "ACTIVE"), .working)
        update(.completed, .complete, intro: true)
        activities.playbackFailed(.complete)
        XCTAssertEqual(activities.nextDeadline, 3)
        clock.time = 3
        activities.tick()
        XCTAssertEqual(activities.playback?.clip, .idle)
    }

    func testWalkingBoundsStayNearTheManualCenterAndInsideTheScreen() {
        XCTAssertEqual(PositionRules.walkingRange(centerX: 300, windowWidth: 220,
            visibleFrame: CGRect(x: 0, y: 0, width: 620, height: 600)), 180...384)
        XCTAssertEqual(PositionRules.walkingRange(centerX: 0, windowWidth: 220,
            visibleFrame: CGRect(x: 0, y: 0, width: 620, height: 600)), 16...120)
        XCTAssertNil(PositionRules.walkingRange(centerX: 0, windowWidth: 220,
            visibleFrame: CGRect(x: 0, y: 0, width: 200, height: 600)))
    }

    private func enterIdleWithoutGreeting() {
        update(.working(node: "ACTIVE"), .working)
        update(.noSelection, .idle)
    }

    private func update(_ phase: DisplayPhase, _ clip: AnimationClip, intro: Bool = false, rest: Bool = false,
                        controls: PetActivityController.Controls = .init()) {
        let result = PresentationRules.Result(phase: phase, clip: clip, playIntro: intro, useRestFrame: rest,
            summary: nil, isStaleSummary: false, detailReadiness: nil)
        activities.update(result, taskID: phase == .noSelection ? nil : "task", controls: controls, allowPrompt: intro)
    }

    private func catalog(additional: Bool = true) -> AnimationCatalog {
        let names = additional ? AnimationClip.allCases : AnimationCatalog.requiredClips
        let clips = Dictionary(uniqueKeysWithValues: names.map { clip in
            (clip, AnimationCatalog.Clip(frames: ["0.png", "1.png"], fps: 10,
                loopRange: clip == .complete ? nil : 0...1, restFrame: 0))
        })
        return AnimationCatalog(canvas: .init(width: 16, height: 16), anchor: .init(x: 8, y: 16), clips: clips)
    }
}
