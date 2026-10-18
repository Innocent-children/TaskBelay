import Foundation

/// Runtime language selection and every user-visible string.
///
/// Language follows the system preference and is never stored: Simplified
/// Chinese when the preferred language is Chinese, English otherwise. Node
/// names match the labels the WebUI already shows for the same Core values.
enum PetLanguage: String, Equatable {
    case chinese = "zh-Hans"
    case english = "en"

    static func resolve(preferredLanguages: [String] = Locale.preferredLanguages) -> PetLanguage {
        guard let first = preferredLanguages.first?.lowercased() else { return .english }
        return first.hasPrefix("zh") ? .chinese : .english
    }
}

struct PetStrings: Equatable {
    var petPin: String { self == .chinese ? "固定关注" : "Pin task" }
    var petAuto: String { self == .chinese ? "恢复自动" : "Follow automatically" }
    var petTasks: String { self == .chinese ? "个未完成" : "unfinished" }
    var petBlocked: String { self == .chinese ? "个受阻" : "blocked" }
    var petBlockedStatus: String { self == .chinese ? "受阻" : "Blocked" }
    // Bubble
    let chooseTask: String
    let disconnected: String
    let disconnectedDetail: String
    let lastRecordMark: String
    let taskUpdatedPrefix: String
    let lastSyncPrefix: String
    let blockedFallback: String
    let archived: String
    let taskUnavailable: String
    let cancelled: String
    let completed: String
    let readOnlyHint: String
    let neverSynced: String

    // Menu
    let menuChooseTask: String
    let menuOpenTaskList: String
    let menuRetryConnection: String
    let menuAnimations: String
    let menuIdleActivities: String
    let menuSize: String
    let sizeSaveFailed: String
    let menuHide: String
    let menuShow: String
    let menuQuit: String
    let reduceMotionNote: String

    // Appearance import and selection
    let menuChooseAppearance: String
    let bundledAppearance: String
    let importAppearance: String
    let importingAppearance: String
    let appearanceImportInstructions: String
    let appearanceFailed: String
    let appearanceRestoreFailed: String
    let dismiss: String

    // Task picker
    let pickerTitle: String
    let pickerEmpty: String
    let pickerLoadMore: String
    let pickerLoading: String
    let pickerUnavailable: String
    let pickerOriginPrefix: String
    let pickerSelectedMark: String

    // Exit and launch advice
    let launchFromTaskBelay: String
    let exitCoreMissing: String
    let exitCoreIdentityChanged: String
    let exitDataRootChanged: String
    /// Diagnostic shown only when the bundled frame assets cannot be read; the
    /// build check refuses to assemble an application without them.
    let assetsUnavailable: String

    // Nodes, matching the WebUI labels for the same Core node identifiers
    let nodes: [String: String]

    static let chinese = PetStrings(
        chooseTask: "选择一个任务",
        disconnected: "未连接",
        disconnectedDetail: "无法连接本机 TaskBelay 服务",
        lastRecordMark: "上次记录",
        taskUpdatedPrefix: "任务更新于",
        lastSyncPrefix: "最近同步",
        blockedFallback: "任务暂时受阻",
        archived: "已归档",
        taskUnavailable: "任务已不可用",
        cancelled: "已取消",
        completed: "已完成",
        readOnlyHint: "当前仅可查看",
        neverSynced: "尚未同步",
        menuChooseTask: "选择任务",
        menuOpenTaskList: "打开任务列表",
        menuRetryConnection: "重试连接",
        menuAnimations: "动画",
        menuIdleActivities: "待机活动",
        menuSize: "宠物大小",
        sizeSaveFailed: "无法保存宠物大小，请检查设置目录。",
        menuHide: "隐藏",
        menuShow: "显示",
        menuQuit: "退出",
        reduceMotionNote: "动画受系统“减少动态效果”限制",
        menuChooseAppearance: "选择形象",
        bundledAppearance: "内置形象",
        importAppearance: "导入形象…",
        importingAppearance: "正在导入形象…",
        appearanceImportInstructions: "选择包含 pet.json 的文件夹，支持 TaskBelay 图片/动画包和 Codex 宠物包。",
        appearanceFailed: "无法使用这个形象",
        appearanceRestoreFailed: "保存的形象不可用，已使用内置形象。可以从菜单重新导入。",
        dismiss: "好",
        pickerTitle: "选择任务",
        pickerEmpty: "当前没有任务",
        pickerLoadMore: "加载下一页",
        pickerLoading: "正在读取任务",
        pickerUnavailable: "任务列表不可用",
        pickerOriginPrefix: "来源",
        pickerSelectedMark: "当前关注",
        launchFromTaskBelay: "请通过 taskbelay pet start 开启桌面宠物",
        exitCoreMissing: "Core 可执行文件已不存在，请从 TaskBelay 入口重新开启",
        exitCoreIdentityChanged: "Core 身份已变化，请从 TaskBelay 入口重新开启",
        exitDataRootChanged: "数据目录身份已变化，请从 TaskBelay 入口重新开启",
        assetsUnavailable: "动画素材不可用",
        nodes: [
            "ACTIVE": "开发与验证",
            "DONE": "已完成",
            "BLOCKED": "暂时受阻",
            "CANCELLED": "已取消",
        ]
    )

    static let english = PetStrings(
        chooseTask: "Choose a task",
        disconnected: "Not connected",
        disconnectedDetail: "Cannot reach the local TaskBelay service",
        lastRecordMark: "Last record",
        taskUpdatedPrefix: "Task updated",
        lastSyncPrefix: "Last sync",
        blockedFallback: "The task is blocked",
        archived: "Archived",
        taskUnavailable: "Task is no longer available",
        cancelled: "Cancelled",
        completed: "Done",
        readOnlyHint: "View only",
        neverSynced: "Not synced yet",
        menuChooseTask: "Choose task",
        menuOpenTaskList: "Open task list",
        menuRetryConnection: "Retry connection",
        menuAnimations: "Animations",
        menuIdleActivities: "Idle activities",
        menuSize: "Pet size",
        sizeSaveFailed: "Cannot save pet size. Check the settings directory.",
        menuHide: "Hide",
        menuShow: "Show",
        menuQuit: "Quit",
        reduceMotionNote: "Animation is limited by the system reduce-motion setting",
        menuChooseAppearance: "Choose appearance",
        bundledAppearance: "Bundled appearance",
        importAppearance: "Import appearance…",
        importingAppearance: "Importing appearance…",
        appearanceImportInstructions: "Choose a folder containing pet.json: a TaskBelay image/animation pack or a Codex pet pack.",
        appearanceFailed: "Cannot use this appearance",
        appearanceRestoreFailed: "The saved appearance is unavailable. Using the bundled character; import it again from the menu.",
        dismiss: "OK",
        pickerTitle: "Choose a task",
        pickerEmpty: "No tasks yet",
        pickerLoadMore: "Load next page",
        pickerLoading: "Reading tasks",
        pickerUnavailable: "The task list is unavailable",
        pickerOriginPrefix: "Origin",
        pickerSelectedMark: "Watching",
        launchFromTaskBelay: "Start the desktop pet with taskbelay pet start",
        exitCoreMissing: "The Core executable is missing; open the pet again from TaskBelay",
        exitCoreIdentityChanged: "The Core identity changed; open the pet again from TaskBelay",
        exitDataRootChanged: "The data directory identity changed; open the pet again from TaskBelay",
        assetsUnavailable: "Animation assets are unavailable",
        nodes: [
            "ACTIVE": "Development and verification",
            "DONE": "Done",
            "BLOCKED": "Blocked",
            "CANCELLED": "Cancelled",
        ]
    )

    static func forLanguage(_ language: PetLanguage) -> PetStrings {
        language == .chinese ? chinese : english
    }

    func nodeName(_ nodeID: String) -> String {
        nodes[nodeID] ?? nodeID
    }
}

/// Formats the two timestamps the bubble distinguishes: when Core last updated
/// the Task, and when the desktop last synchronized with the local service.
enum PetTimeFormatter {
    static func format(_ date: Date?, language: PetLanguage) -> String {
        guard let date else {
            return language == .chinese ? PetStrings.chinese.neverSynced : PetStrings.english.neverSynced
        }
        let formatter = DateFormatter()
        formatter.dateStyle = .medium
        formatter.timeStyle = .medium
        formatter.locale = Locale(identifier: language.rawValue)
        return formatter.string(from: date)
    }
}
