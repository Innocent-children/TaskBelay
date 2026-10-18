import Foundation

/// Response models for the existing local WebUI HTTP interface.
///
/// These types only decode what the desktop component displays. Core owns
/// lifecycle, blocker, and terminal classification; the desktop never
/// re-derives them from node identifiers.
enum Readiness: String, Codable, Equatable {
    case ready
    case readOnly = "read_only"
    case incompatible
    case unavailable
    case unknown

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let value = try container.decode(String.self)
        self = Readiness(rawValue: value) ?? .unknown
    }
}

enum TaskLifecycle: String, Codable, Equatable {
    case active
    case blocked
    case done
    case cancelled
    case unknown

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let value = try container.decode(String.self)
        self = TaskLifecycle(rawValue: value) ?? .unknown
    }

    var isTerminal: Bool {
        switch self {
        case .done, .cancelled: return true
        case .active, .blocked, .unknown: return false
        }
    }
}

struct SystemStatusResponse: Decodable, Equatable {
    let readiness: Readiness
    let coreIdentity: String
    let dataRootDigest: String
    let url: String

    enum CodingKeys: String, CodingKey {
        case readiness
        case coreIdentity = "core_identity"
        case dataRootDigest = "data_root_digest"
        case url
    }
}

/// The subset of `TaskSummary` used by the desktop. Unused response fields are
/// deliberately not modelled.
private struct DesktopRepositorySummary: Decodable {
    let key: String
    let repositoryPath: String
    enum CodingKeys: String, CodingKey {
        case key
        case repositoryPath = "repository_path"
    }
}

struct DesktopTaskSummary: Decodable, Equatable {
    let taskID: String
    let requestSummary: String
    let originHost: String
    let currentNode: String
    let lifecycle: TaskLifecycle
    let revision: UInt64
    let updatedAt: Date
    let archived: Bool
    let repositoryKeys: [String]
    let worktreePath: String
    let blocker: String?

    enum CodingKeys: String, CodingKey {
        case taskID = "task_id"
        case requestSummary = "request_summary"
        case originHost = "host"
        case repositories
        case lifecycle
        case revision
        case updatedAt = "updated_at"
        case archived
        case blocker
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        taskID = try container.decode(String.self, forKey: .taskID)
        requestSummary = try container.decode(String.self, forKey: .requestSummary)
        originHost = try container.decode(String.self, forKey: .originHost)
        lifecycle = try container.decode(TaskLifecycle.self, forKey: .lifecycle)
        currentNode = lifecycle.rawValue.uppercased()
        revision = try container.decode(UInt64.self, forKey: .revision)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        archived = try container.decode(Bool.self, forKey: .archived)
        let repositories = try container.decode([DesktopRepositorySummary].self, forKey: .repositories)
        guard let primary = repositories.first else {
            throw DecodingError.dataCorruptedError(forKey: .repositories, in: container, debugDescription: "A Task must retain its bound repositories")
        }
        repositoryKeys = repositories.map(\.key)
        worktreePath = primary.repositoryPath
        blocker = try container.decodeIfPresent(String.self, forKey: .blocker)
    }
}

struct TaskListResponse: Decodable, Equatable {
    let readiness: Readiness
    let page: Int
    let hasNext: Bool
    let items: [DesktopTaskSummary]

    enum CodingKeys: String, CodingKey {
        case readiness
        case page
        case hasNext = "has_next"
        case items
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        readiness = try container.decode(Readiness.self, forKey: .readiness)
        page = try container.decode(Int.self, forKey: .page)
        hasNext = try container.decode(Bool.self, forKey: .hasNext)
        items = try container.decodeIfPresent([DesktopTaskSummary].self, forKey: .items) ?? []
    }
}

struct TaskDetailResponse: Decodable, Equatable {
    let readiness: Readiness
    let summary: DesktopTaskSummary
}

/// Decodes the RFC 3339 timestamps emitted by Go `time.Time`, including the
/// nanosecond fraction that `ISO8601DateFormatter` does not accept directly.
enum RFC3339Timestamp {
    private static let pattern = #"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$"#
    private static let withFraction: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()
    private static let withoutFraction: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    static func date(from value: String) -> Date? {
        guard let expression = try? NSRegularExpression(pattern: pattern),
              let match = expression.firstMatch(in: value, range: NSRange(value.startIndex..., in: value)),
              let secondsRange = Range(match.range(at: 1), in: value),
              let zoneRange = Range(match.range(at: 3), in: value) else {
            return nil
        }
        let base = String(value[secondsRange])
        let zone = String(value[zoneRange])
        let fractionRange = match.range(at: 2)
        guard fractionRange.location != NSNotFound, let range = Range(fractionRange, in: value) else {
            return withoutFraction.date(from: base + zone)
        }
        let digits = String(value[range]).prefix(3)
        let normalized = digits.padding(toLength: 3, withPad: "0", startingAt: 0)
        return withFraction.date(from: "\(base).\(normalized)\(zone)")
            ?? withoutFraction.date(from: base + zone)
    }
}

enum WebUIDecoding {
    /// A decoder matching the current Core JSON output. Unknown members are
    /// ignored because the desktop models only the fields it displays.
    static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            guard let date = RFC3339Timestamp.date(from: value) else {
                throw DecodingError.dataCorruptedError(in: container, debugDescription: "timestamp is not RFC 3339")
            }
            return date
        }
        return decoder
    }

    static func decodeSystemStatus(_ data: Data) throws -> SystemStatusResponse {
        try makeDecoder().decode(SystemStatusResponse.self, from: data)
    }

    static func decodeTaskList(_ data: Data) throws -> TaskListResponse {
        try makeDecoder().decode(TaskListResponse.self, from: data)
    }

    static func decodeTaskDetail(_ data: Data) throws -> TaskDetailResponse {
        try makeDecoder().decode(TaskDetailResponse.self, from: data)
    }
}

/// The runtime identity reported by `taskbelay webui status --json`.
///
/// The Core writes these members at the top level of a single JSON object. `pid`
/// is absent or zero when no service process is recorded, so it stays optional.
struct CoreRuntimeStatus: Equatable {
    let readiness: Readiness
    let coreIdentity: String
    let dataRootDigest: String
    let url: String
    let pid: Int?

    private struct Payload: Decodable {
        let readiness: Readiness
        let coreIdentity: String
        let dataRootDigest: String
        let url: String
        let pid: Int?

        enum CodingKeys: String, CodingKey {
            case readiness
            case coreIdentity = "core_identity"
            case dataRootDigest = "data_root_digest"
            case url
            case pid
        }
    }

    static func decode(_ data: Data) throws -> CoreRuntimeStatus {
        let payload = try WebUIDecoding.makeDecoder().decode(Payload.self, from: data)
        guard !payload.coreIdentity.isEmpty, !payload.dataRootDigest.isEmpty else {
            throw DecodingError.dataCorrupted(
                DecodingError.Context(codingPath: [], debugDescription: "Core runtime status is incomplete")
            )
        }
        return CoreRuntimeStatus(
            readiness: payload.readiness,
            coreIdentity: payload.coreIdentity,
            dataRootDigest: payload.dataRootDigest,
            url: payload.url,
            pid: payload.pid
        )
    }
}
