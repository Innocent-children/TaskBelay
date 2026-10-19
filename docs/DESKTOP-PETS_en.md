# Desktop pet usage and appearance guide

[中文](DESKTOP-PETS.md) | [English](DESKTOP-PETS_en.md)

The desktop pet shows the saved state of one selected TaskBelay Task and opens its WebUI on click. Appearance packs supply artwork;
the local scheduler runs idle activities, while Core owns Task state. This guide covers obtaining and starting the app, selecting tasks,
updating the program and artwork, animation rules, pack creation, and troubleshooting.

## Environment and delivery

The desktop component targets macOS arm64 (Apple Silicon) and Windows 10/11 x64 and requires at least one installed and configured Codex, DeepSeek, Claude Code or ZCode Adapter to provide Core.
The Swift Package and app metadata target macOS 14; actual minimum-system operation, Developer ID signing, and Apple notarization have not completed
formal distribution verification. See the [support matrix](SUPPORT-MATRIX_en.md) for the verified scope.

Below, `productRoot` means the product directory, which defaults to `~/.taskbelay` on macOS; settings and appearances live in its `pet/` subdirectory.

The formal `taskbelay` npm package includes `runtime/darwin-arm64/TaskBelayPet.app` and
`runtime/win32-x64/TaskBelayPet`, each with nine default actions and 57 PNG frames.
Running the built application requires no compiler or Electron development environment. An independently installed Adapter provides Core.
Formal preparation on macOS arm64 compiles Swift, assembles the locked Windows x64 Electron runtime,
and checks versions, architectures, artwork and the extracted package files. macOS uses ad-hoc signing; Windows distribution signing remains unverified.

## npm installation and startup

```bash
npm install -g taskbelay@latest
taskbelay install
taskbelay pet start
```

Configure at least one Adapter through the unified entry; existing configuration can be reused. The launcher supplies Core and data-directory arguments.
It prefers the installed user-directory app over the bundled app. `pet start` reuses an existing application; use maintenance commands below to update its copy.

## Local build and installation

Configure at least one Adapter through the [unified installation entry](COMMANDS_en.md); existing configurations can be reused.
Run the following from the repository root, with an absolute output directory outside the repository:

```bash
node scripts/build-desktop-pet.mjs --output "/absolute/pet-build"
```

The script compiles Swift, assembles artwork and language resources, signs ad hoc, and creates a `.tgz` plus `desktop-pet-build.json`.
It neither rebuilds Core nor publishes a package. If you obtain a local package produced by this script, proceed directly to installation.
Replace `<local-package>.tgz` below with the actual filename reported by the build result's `tarball` field:

```bash
npm install -g "/absolute/pet-build/<local-package>.tgz"
taskbelay pet start
```

Local packages use the same platform application assembly as formal preparation. macOS build machines require Node.js >=24, Xcode >=27 and macOS SDK >=27; `xcrun swift` uses the selected Xcode. Builds check and report toolchain versions before compilation and include stdout and stderr in failure diagnostics.

To verify a local installation, first update any existing user-directory application as described under “Updating the application and artwork,” then run `taskbelay pet start`, `taskbelay pet stop`, and `taskbelay pet start` in order. The last start should show the pet and continue reading tasks; compilation alone does not establish a connection to Core. The launcher and native application read the current runtime state described in the [command reference](COMMANDS_en.md#installation-and-local-entrypoints) and verify the live loopback service.

## Windows local build and installation

Building the Windows development package requires Go, Node.js and pnpm on PATH at the repository toolchain versions. Windows 10/11 x64 has a separate desktop implementation with task selection, status bubbles, tray menus, nine actions, appearance import, scaling, dragging, hide/restore and independent start/stop. A built app carries its runtime and needs no Electron development tools or compiler; a configured Adapter still supplies Core.

From the repository root, install the locked Windows build dependencies and choose an output directory outside the repository:

~~~powershell
npm ci --prefix packages/desktop-pet/windows
node scripts/build-desktop-pet-windows.mjs --output "C:\pet-build"
npm install -g "C:\pet-build\<local-package>.tgz"
taskbelay install --host all --yes
taskbelay pet start
taskbelay pet stop
~~~

Replace `<local-package>.tgz` with the filename identified by tarball in desktop-pet-build.json. This build assembles the Windows desktop and complete Adapter packages using the existing Core target catalog, binding artifact paths, versions and SHA256 hashes. It copies nine default actions and 57 frames and verifies extracted artwork and executable bytes. Mac Core is cross-compiled only; no Mac program, Mac test or publication is run. Windows uses the system tray in place of the macOS menu bar; artwork formats, task semantics and six scale choices align.

The local distribution's `--host all` includes ZCode. Its `action_required` result still requires plugin installation and enablement in the ZCode UI; see the [ZCode guide](ZCODE_en.md). The pet reading Core does not establish an available ZCode model session.

productRoot defaults to %LOCALAPPDATA%\taskbelay. The installed app directory is productRoot/pet/TaskBelayPet, with TaskBelayPet.exe as its entry; it takes precedence over runtime/win32-x64/TaskBelayPet in the package. settings.json and appearances/ are stored separately. The unified entry refreshes the app copy through taskbelay install, upgrade, repair or reinstall. The launcher stops maintained instances, stages the replacement, and retains settings.json and appearances/. Running pet start alone does not reinstall an existing app. Ordinary quit and uninstall preserve these files; confirmed delete-data or delete-all clears pet preferences and imported appearances while keeping the installed desktop application.

Windows uses a restricted renderer and a local per-user single-instance channel with acknowledgments. It does not stop processes by name or PID alone. Each poll and navigation rechecks the same Core and data directory. Disconnection keeps and marks the last record, with separate task-update and last-sync timestamps. Hiding or sleeping cancels reads and animation; resuming does not replay historical completion prompts.

Windows distribution signing remains unverified. Recorded native environments and results are in the [adaptation report](WINDOWS-ADAPTATION_en.md); supported scope is listed in the [Support Matrix](SUPPORT-MATRIX_en.md).

## Updating the program and artwork

Update the npm package, then refresh the installed application copy through a maintenance command:

```bash
taskbelay pet stop
npm install -g taskbelay@latest
taskbelay repair --host codex --yes
taskbelay pet start
```

Choose the configured Host; DeepSeek uses `--host deepseek --profile <name>`. `install`, `upgrade`, `repair` and `reinstall` compare bundled and installed application files, updating the pet only when files are missing or differ, including local builds with the same version. Identical applications are not stopped or copied merely because a plugin is already current. Launcher self-update installs the desktop application from the new package. The plan lists actual changes before confirmation. Updates stop the pet, stage and replace the application directory; stop/copy failure halts maintenance and staging failure preserves the old app. Settings and appearances stay in separate directories.

Menu runtime status uses platform verification: macOS checks kernel creation identity, executable path and user; Windows verifies a reply over the instance-token pipe. runtime.json alone does not mean running. Unverifiable state stays unknown; observation starts or stops nothing.

Installing the npm package alone updates its bundled application. Refresh an existing user-directory copy with the maintenance command above. Reimport an appearance from its source folder to update artwork; application updates do not invent missing animations.

## Default appearance and external artwork

The pet package includes only Dudu (嘟嘟), the blue sea sprite from `packages/desktop-pet/default-appearance/`, with nine clips and 57 PNG frames. The build copies the artwork and verifies each file against its source. Import other custom appearances as
separate artwork packs: choose Import appearance and select a folder containing `pet.json`. Supported formats
are listed below under Appearance types and available animations.

Imported artwork lives in `productRoot/pet/appearances/<id>` and is preserved across application updates.
Reimport the same source folder to update a custom appearance. Switching appearances keeps the watched Task.

## Task selection and basic controls

The macOS menu bar entry uses TaskBelay’s circular TB monogram as a monochrome 18 pt vector icon, tinted by the system for the menu bar appearance and selection state. The Windows tray uses the same TB monogram on a light yellow-green background, matching the WebUI app icon.

By default, up to three task bubbles overlap: the front bubble shows the focused task, with the outlines of other task cards behind it. The footer shows unfinished and blocked counts, plus a count for additional tasks. Hover or click the count to expand a scrollable list; each bubble opens its own task WebUI.

When there are no task cards to display, the bubbles and count control are hidden, including on hover. Bubbles return automatically when tasks appear. The hidden area lets mouse input pass through, and clicking the character still opens the task list. Completion prompts, pinned tasks, and task records retained during disconnection follow the rules below.

Every observation discovers unfinished tasks. Automatic focus prioritizes blocked tasks and remains stable among tasks with the same priority; a new focus is selected by most recent update. When the focused task completes, its prompt stays for about three seconds before focus moves to unfinished work. Completions observed continuously in this session remain in the expanded list until opened or dismissed. Cancelled, archived, or confirmed unavailable tasks also release automatic focus. With no remaining work, the pet becomes idle.

Pin task saves an explicit choice for the current data directory. A pinned task stays focused after completion while other task bubbles remain visible. Follow automatically clears the pin. Opening a task does not pin it. Restart restores only an explicit pin; completion prompts are limited to the current session.

Observation waits five seconds after each round on macOS and three seconds on Windows; a completion prompt can trigger an earlier refresh. Hiding or sleeping pauses reads. Failed reads preserve the last records and mark them disconnected. Recovery, wake, and first reads of historical completion do not replay celebrations.

Ordinary Codex chats do not automatically become TaskBelay Tasks. A selected task that remains active keeps working or review artwork even when its Host has no new output.

| Operation | Result |
| --- | --- |
| Left-click the pet or bubble | The pet opens the focused task, or the list with no focus; each bubble opens its own Task WebUI. |
| Context menu / menu bar | Pin a task, follow automatically, choose appearances, import packs, adjust pet size, control animations and idle activities, hide, or quit. |
| Hover | Expand the task bubbles; hovering over the character can also trigger a wave when idle conditions are met. |
| Drag and drop | Save the new placement, which becomes the center of subsequent walks. |
| Hide or system sleep | Stop animation, movement, and observation requests; show or wake to read the current state again. |
| Quit / `taskbelay pet stop` | End only the pet, preserving WebUI, Tasks, settings, and artwork. |

Only `taskbelay pet start` and `taskbelay pet stop` are supported pet commands. Output is plain text, with exit codes `0` for success, `1` for runtime failure,
and `2` for invalid arguments. There is no public `pet status` or `pet start --json`; use the error text and troubleshooting below.

## macOS motion

Bubble expansion and collapse use system animations and continue from the current appearance when the pointer changes direction.
Cards and the scroll surface stay in place throughout the transition; expanded tasks remain clickable, pinnable, and scrollable.
macOS 26 and later use system Liquid Glass materials, with native interactive glass feedback on macOS 27.
Earlier supported systems use their corresponding AppKit materials and animations.

Walks ease into motion and slow before arrival, keeping the overall distance and duration described below.
Pointer entry, a press, or opening a menu interrupts walking. Turning off Animations or enabling the system's Reduce Motion
setting applies the target bubble layout immediately and uses a still character frame. Hiding and sleep stop playback.

Bubbles use the transparent headroom shared by all frames of the current clip to sit closer to the character, with a margin for fine details. Frame changes keep the layout stable while preserving character size and foot placement.

Character frames retain their authored durations and update with the display refresh. After a brief main-thread delay,
the player shows the frame for the current elapsed time. This improves timing without creating missing poses;
low-frame-count artwork still has its original visual limits.

## Pet size

Pet size offers 50%, 75%, 100%, 125%, 150%, and 200%, defaulting to 100% and saved as `scale` in `settings.json`. Scaling affects the character while bubble text and width stay unchanged.
Resizing preserves the artwork anchor position before constraining the window to the visible screen. The size and position apply after saving succeeds; a failed save shows a message and keeps the current size. Walking speed follows the scale.

On Windows, resizing ends the current idle activity and its movement, then schedules the next activity after the normal interval. Work animations and completion celebrations retain their playback progress.

## Appearance types and available animations

Rules use animation keys for every appearance, with no behavior tied to a particular name. The artwork actually supplied determines which effects an appearance can show.

| Appearance type | Saved and played content |
| --- | --- |
| Single PNG or SVG | All five task clips share one image, producing a static appearance. |
| Native TaskBelay PNG/SVG animation pack | Five task clips are required and four additional clips are optional; each clip's frame count comes from its own catalog, and SVG retains its vector representation. |
| Standard Codex atlas or a high-resolution extension with the same layout | Extract the fixed nine animation rows: nine clips, 57 frames. Format 2's extra gaze frames are outside the extraction scope. |

The five required clips are `idle`, `working`, `blocked`, `complete`, and `disconnected`, ensuring each task state has artwork.
The four additional clips are `running-right`, `running-left`, `waving`, and `review`, supporting walking, responses, and thinking.
Missing additional clips are removed from the idle candidate set; active Tasks use working artwork.
Inspect `clips` in `productRoot/pet/appearances/<id>/animations.json` to see the installed copy's animations.

## Import and selection

1. Open Choose appearance from the pet context menu or menu bar.
2. Select Import appearance and choose the folder containing `pet.json`.
3. A successful import selects the appearance immediately. Use the menu to select the bundled character or another imported appearance.

Import saves a copy, so the source folder can be moved away. Reimport an edited pack with the same ID to update it.
The complete converted pack must pass the same validation used when loading, inside a temporary directory, before it replaces the installed directory.
Failed validation preserves the installed appearance and selection. Task and appearance choices are stored independently; switching preserves the watched task and does not replay old completion prompts.

User artwork lives in `productRoot/pet/appearances/<id>`; pet settings store `selected_appearance`. Upgrades and ordinary uninstall preserve packs
and selection. Confirmed delete-data or delete-all clears them while retaining the application. A missing or invalid saved pack is reported, and the bundled character
is used until the user imports or selects an appearance again.

## TaskBelay static appearance

A minimal pack contains a manifest and one PNG or SVG. This example uses PNG:

```text
orange-square/
  pet.json
  pet.png
```

`pet.json`:

```json
{
  "id": "orange-square",
  "name": "Orange square",
  "image": "pet.png"
}
```

`id` contains 1–64 lowercase ASCII letters, digits, hyphens, or underscores, starting with a letter or digit. `name` contains 1–100 characters.
`image` names a PNG or SVG relative to this folder; a transparent background is recommended. One image serves all stages, which the bubble text distinguishes.
Copy the repository's [static example](../packages/desktop-pet/examples/orange-square/pet.json) and replace its image and name.

## TaskBelay animated appearance

An animated pack's `pet.json` contains only `id` and `name`. Provide sibling `animations.json` and `Assets/`:

```text
my-pet/
  pet.json
  animations.json
  Assets/
    idle/0.png
    idle/1.png
    working/0.png
    blocked/0.png
    complete/0.png
    complete/1.png
    disconnected/0.png
```

Frames can use PNG or SVG. SVG files follow the file requirements below and preserve their original vector content on import. This example uses 128×128 PNG images. Keep canvas geometry and character placement consistent across frames:

```json
{
  "canvas": { "width": 128, "height": 128 },
  "anchor": { "x": 64, "y": 128 },
  "clips": {
    "idle": { "frames": ["idle/0.png", "idle/1.png"], "fps": 8, "loop_range": [0, 1], "rest_frame": 0 },
    "working": { "frames": ["working/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 },
    "blocked": { "frames": ["blocked/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 },
    "complete": { "frames": ["complete/0.png", "complete/1.png"], "fps": 8, "loop_range": null, "rest_frame": 1 },
    "disconnected": { "frames": ["disconnected/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 }
  }
}
```

Provide all five task clips; they may reuse one image. Catalogs may also include `running-right`, `running-left`, `waving`, and `review`.
Every supplied clip follows the same validation and loading rules. A single-frame clip is static. Multiple frames use `fps`, or an optional
`frame_durations_ms` array with one duration per entry in `frames` that takes precedence over `fps`. `loop_range` includes both bounds. Completion
uses `null` and ends on `rest_frame`. Reduce Motion and the animation switch use static frames. The player scales the full canvas proportionally;
authors align artwork on a consistent canvas, with `anchor` retained as the shared artwork reference point.

## Compatibility with standard Codex pet formats

TaskBelay supports Codex sprite formats 1 and 2. Compatibility covers the local pack's atlas layout, required animation frames, and per-frame timings;
TaskBelay owns task stages, prompts, and navigation. The importer accepts a PNG/WebP atlas and a `pet.json` with these fields:

```json
{
  "id": "my-codex-pet",
  "displayName": "My Codex pet",
  "description": "My companion",
  "spriteVersionNumber": 2,
  "spritesheetPath": "spritesheet.webp"
}
```

Select the individual pet folder, such as `~/.codex/pets/my-codex-pet`, containing `pet.json` and its PNG/WebP atlas.
This importer requires `id`, `displayName`, and `spritesheetPath`: `id` is nonempty with at most 256 characters, `displayName` contains
non-whitespace content with at most 100 characters, and `spritesheetPath` is a relative file path. `description` is optional and does not affect task presentation.
Omitted `spriteVersionNumber` means 1; an explicit value must be 1 or 2, matching the
[official Codex pet version parameter](https://developers.openai.com/codex/reference/commands#pets).

| Standard Codex format | Atlas size | Grid | Cell size |
| --- | --- | --- | --- |
| `spriteVersionNumber: 1` | 1536×1872 | 8 columns, 9 rows | 192×208 |
| `spriteVersionNumber: 2` | 1536×2288 | 8 columns, 11 rows | 192×208 |

Both standard formats share the nine animation rows below. TaskBelay imports all nine, totaling 57 frames.
Format 2 includes additional gaze frames; the current import covers the first nine animation rows.
Here, 1 and 2 identify Codex artwork formats, not TaskBelay product versions.

| Animation | Catalog key | Codex animation row | Frames |
| --- | --- | --- | --- |
| Idle | `idle` | idle, row 0 | 6 |
| Walk right | `running-right` | running-right, row 1 | 8 |
| Walk left | `running-left` | running-left, row 2 | 8 |
| Wave | `waving` | waving, row 3 | 4 |
| Completion | `complete` | jumping, row 4 | 5 |
| Disconnected | `disconnected` | failed, row 5 | 8 |
| Blocked | `blocked` | waiting, row 6 | 6 |
| Active Task | `working` | running, row 7 | 6 |
| Idle thinking | `review` | review, row 8 | 6 |

Import crops PNG frames and preserves standard frame timings, then uses the common player. Walk right, walk left, wave, and review
participate in the idle activities and task animation selection below. Looping artwork can also finish after a specified number of full cycles.
TaskBelay owns task meaning, completion conditions, and navigation; `behavior-map.json` is not used for importing or scheduling animations.
Source files stay intact, and the installed directory ID is derived consistently from the Codex `id`.

## TaskBelay's own high-resolution extension

The same entry also supports TaskBelay's own high-resolution atlas extension. It uses the `pet.json` fields and animation row layout above,
preserving the original cell resolution when converting to common PNG frames. This extension is supported by TaskBelay and is outside the
standard Codex formats. A successful TaskBelay import does not imply that Codex can read the original atlas. To use the artwork in Codex as well,
provide a separate atlas with the standard dimensions matching its `spriteVersionNumber` in the table above.

Extension cropping rules:

- The atlas has eight columns and its width must be divisible by 8: `cell_width = width / 8`.
- Cells keep the 192:208 ratio: `cell_height = cell_width × 208 / 192` must be an integer.
- The height must cover the first nine complete animation rows: `height >= cell_height × 9`. All nine listed clips are extracted.
- `spriteVersionNumber` still accepts only 1 or 2 and defaults to 1. Extension cell dimensions come from the width;
  the field does not require an extension atlas to have a standard format's total height.

For example, an 8-bit 12288×14976 atlas has 1536×1664 cells and nine rows. Even if its field is 2, TaskBelay extracts all nine clips using
the extension rules. It is not a standard Codex format 2 atlas. Its estimated decoded RGBA data is 702 MiB and its largest eight-frame clip
is estimated at 78 MiB, both within the limits below. Import must also satisfy the total size limit for converted PNG files.

## Idle activities and animation triggers

The menu includes an enabled-by-default Idle activities switch, saved as `idle_activities_enabled`. Turning it off retains baseline idle
and task animations while stopping idle rotation, interaction waves, and automatic movement. The Animations switch and the system's
Reduce Motion setting take precedence; disabling animation uses still frames.

Idle activities begin when the service is connected, the window is visible, and no task is focused. Automatic mode follows unfinished work after a completion prompt; pinned completed, cancelled, or archived tasks
retain their corresponding posture for three seconds before idling. An observed completion first plays its full celebration, then waits those
three seconds. A first read of an already-completed task uses its still completion frame without replaying a historical celebration.
Active, blocked, unavailable, or unknown tasks and disconnected services use their corresponding task presentation.

| Animation | Trigger rules |
| --- | --- |
| Baseline idle | Wait a random 6–12 seconds after each short activity before selecting the next one. Ordinary reads of the same state preserve the current animation and deadline. |
| Walk right, walk left | Initial weight: 25% each. With at least 40 pt available in the direction, move horizontally 40–80 pt at an average of about `20 × scale` pt/second. Duration is distance divided by speed, or 2–4 seconds at 100% size. A finished walk has a 30% chance of continuing with one wave. |
| Wave | Random weight: 20%. Idle startup or showing the pet, hovering over the character for at least 0.4 seconds, and dropping it after a drag can also trigger a wave. Play 1–2 cycles with a shared 20-second cooldown. Continuous hover responds once; leaving for at least two seconds rearms it. |
| Review / thinking at the computer | Random weight: 30%. During idle, play enough complete cycles to meet a randomly selected 3–5-second target, then return to idle. Active Tasks use working artwork; review is an idle animation, not a Core stage. |

Exclude unavailable artwork, cooling-down actions, and directions with insufficient room before sampling the remaining weights.
Avoid consecutive identical choices when alternatives exist. Appearances without review artwork omit that idle choice.
Idle thinking preserves the existing bubble text; Core state continues to determine task meaning and navigation.

Automatic walking stays within 120 pt horizontally of the last manual placement, with targets at least 16 pt from the current screen edge
and the full window inside the visible area. Temporary movement stays in memory; only manual dragging updates the saved position.
Screen layout changes cancel the current route and constrain the window again.

Entering the pet window stops movement and pauses random activities. Only hovering over the character triggers a wave; bubble hover still
expands information. Mouse presses, dragging, menus, and selection/import panels suspend idle activities. Left-click navigation and right-click
menus keep their existing behavior. Observed blocked, completed, or disconnected states immediately interrupt idle activities.
Hiding, sleeping, or turning off the relevant switches cancels idle deadlines and movement. Resuming schedules fresh activities without replaying
interrupted ones. The local scheduler, player, and window coordinate activity execution, completion, and idle waiting; ordinary polling does not
restart animations repeatedly.

## File requirements

- Each description file is at most 256 KiB. References must be relative paths to regular files; symlinks and references outside the folder are rejected.
- TaskBelay PNG/SVG animation frame dimensions must match `canvas`, whose width and height are positive integers. Static PNGs and individual PNG frames
  have an estimated decoding limit of 128 MiB each.
- Catalogs contain at most 512 frame references. Referenced PNG/SVG artwork files total at most 128 MiB to accommodate nine high-resolution clips. Each clip's RGBA data, estimated as
  `canvas.width × canvas.height × 4 × frame_count`, is at most 128 MiB; `frame_count` is the number of frames in the clip.
- Each SVG is UTF-8 and at most 1 MiB, with integer root dimensions from 1 to 4096. An optional `viewBox` must be `0 0 width height`. Accepted content is limited to static vector shapes, gradients, and internal references, with at most 4096 elements and 64 nesting levels. Scripts, embedded raster images, external references, document types, and entities are rejected. The current system must be able to render the artwork.
- `fps` is 0.1–120; per-frame durations are 9–60000 milliseconds. Loop and rest indexes must be valid.
- Standard Codex and extended TaskBelay source atlases are at most 512 MiB, with an estimated decoding limit of 1 GiB.
- Before allocating pixels, decoding estimates use image metadata: `width × height × 4 × ceil(bit_depth / 8)`, where `bit_depth` is the bit depth, with four channels
  and supported depths of 1–16 bits. This bounds image pixel data, not the process's peak memory.
- Converted results must also satisfy the PNG total size, clip memory, and catalog limits. Excessive dimensions, potential integer overflow,
  or an oversized converted result return an import error and preserve the installed appearance.

Packs contain presentation data, not executable scripts.

## Troubleshooting

| Symptom | Checks and action |
| --- | --- |
| Installing an Adapter did not provide the pet app | Install `taskbelay@latest`, configure an Adapter with `taskbelay install`, then run `taskbelay pet start`. |
| Updated package still shows old behavior | Run `taskbelay repair` after updating npm to refresh the user-directory copy; reimport artwork separately. |
| Another appearance cannot walk, wave, or think | Check whether its installed `clips` includes those additional animations. A five-clip appearance can show tasks normally but cannot play artwork it does not contain. |
| The source has nine clips but the installed copy has five | Confirm that the running app copy was updated too, then reimport the original folder containing the complete atlas. The app loads frames saved by the latest import; upgrades do not automatically add missing clips. |
| Must every pack contain nine clips and 57 frames? | This fixed count applies only to Codex-layout atlases. Single PNG/SVG images and native animation packs follow their own rules in the table above. |
| The pet stays idle or does not walk | Check task selection, connectivity, Animations, Idle activities, Reduce Motion, and walking artwork. Hovering inside the window or opening a menu/panel pauses idle activity; leave and wait 6–12 seconds. Directions with less than 40 pt of room are excluded. |
| Hovering does not trigger another wave | Idle conditions and the shared 20-second cooldown must allow it. Hover over the character for at least 0.4 seconds. Continuous hover responds once; leave for at least two seconds to rearm. |
| Chat activity does not match the pet's working animation | The pet reads the selected TaskBelay Task's saved state, not ordinary Codex chat activity or keyboard input. Check the watched Task. |
| Selecting a completed task does not trigger a jump | Celebrate once only when continuously observing the same task change from nonterminal to DONE. A first read of an already-completed task uses a still frame, then may enter idle activities. |
| The source atlas fits its limit, but PNG size or memory is rejected | Source atlases and converted frames have separate limits. WebP-to-PNG conversion can increase file size. Check the 512 MiB source limit, 1 GiB decoding estimate, 128 MiB total converted PNG limit, and 128 MiB per-clip limit. |
| Not connected or startup fails | Check the platform and Adapter configuration; a running pet offers Retry connection. Explicit `pet start` can start WebUI when needed; background observation is read-only. Do not use the nonexistent `pet status` command. |
| A sibling behavior-map.json has no effect | The importer reads `pet.json` with an atlas or `animations.json`. `behavior-map.json` controls neither timing nor behavior; use the supported catalog fields. |

## Acceptance checks

Use checks matching the affected component and environment, and retain the artifact identity, command, actual result and any unrun steps separately.

| Check | Environment and expected result |
| --- | --- |
| `swift test --package-path packages/desktop-pet/macos --filter 'PetMotionTests\|PetAnimationTimelineTests\|AnimationAndBubbleTests\|PetTaskCollectionTests\|PetActivityTests'` | macOS; native-window retargeting, interruption, screen constraints, scrolling and button hit testing, plus frame timing and activity rules; window checks use constructed task data |
| `swift test --package-path packages/desktop-pet/macos --filter TaskObserverTests` | macOS; simulated Core/HTTP checks multi-task discovery, pinning, automatic handoff and late-response handling |
| `node --test packages/taskbelay/tests/pet.test.mjs` | Builds a real Core from source and checks stopped-service output, service startup, and launcher decoding; the native window handoff uses a simulated entry. Requires Go. |
| `swift test --package-path packages/desktop-pet/macos --filter WebUIModelTests` | macOS; includes building and calling a real Core to check status decoding and native launch arguments. Requires Go; final application visibility and start/stop behavior are verified separately. |
| `swift test --package-path packages/desktop-pet/macos --filter PetAppearanceTests` | macOS; static/native/Codex import preserves supported frames and timing, and failed imports preserve the installed appearance |
| `node --test packages/desktop-pet/windows/tests/contracts.test.cjs` | Node.js; check Windows artwork loading and import limits for frame counts and timing, and preservation of installed appearances after rejected reimports; does not exercise native Windows UI |
| `node --test packages/desktop-pet/windows/tests/task-selection.test.cjs` | Simulated Electron, preferences and HTTP; verify discovery and selection rules |
| `node --test packages/desktop-pet/windows/tests/renderer.test.cjs` | Simulated DOM, IPC and clock; polling preserves animation, resizing cancels walking and normal scheduling resumes |
| `node --test scripts/desktop-pet-artwork.test.mjs` | Verify default-artwork copying and rejection after content changes |
| Installed local package | On the target OS, check task selection, WebUI navigation, import, scaling, hide/restore, startup reuse and normal stop with retained settings |

Native Windows window checks require Windows x64 Electron and the environment described by `packages/desktop-pet/windows/tests/native.cjs`; simulations do not establish native window behavior. Full Host workflows and minimum-system checks require their own actual environments.

See the [Support Matrix](SUPPORT-MATRIX_en.md) for verified scope and limits, and the [Windows report](WINDOWS-ADAPTATION_en.md) for Windows check results. Historical desktop validation records are available through Git history; component checks do not expand stable support.
