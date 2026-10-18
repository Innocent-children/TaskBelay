import Foundation

/// The private native entry contract shared with the Node launcher.
///
/// `run` opens the window or restores an existing instance that uses the same
/// Core runtime and data directory. `stop` is the internal shutdown entry used
/// by `taskbelay pet stop` and by Adapter maintenance; it never creates a window.
/// These arguments express a product dependency between the unified launcher
/// and the bundled application. They are not an authentication mechanism
/// between processes of the same user.
struct LaunchArguments: Equatable {
    enum Mode: String, Equatable {
        case run
        case stop
    }

    let mode: Mode
    let productRoot: String
    let corePath: String?
    let dataDirectory: String?
    let coreIdentity: String?
    let dataRootDigest: String?
}

struct LaunchArgumentError: Error, Equatable {
    let message: String

    init(_ message: String) {
        self.message = message
    }
}

enum LaunchArgumentParser {
    private static let coreIdentityPattern = #"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$"#
    private static let digestPattern = "^[0-9a-f]{64}$"
    /// The launcher and this process must agree on exactly these fields. An
    /// option outside the set means the two sides drifted apart, so the entry
    /// refuses to start rather than ignoring a field it does not understand.
    private static let runFields: Set<String> = [
        "core-path", "data-dir", "product-root", "core-identity", "data-root-digest",
    ]
    private static let stopFields: Set<String> = ["core-path", "product-root"]

    /// Parses the private argument array. Every path must already be absolute
    /// because the launcher resolves Adapter, data, and product directories
    /// before starting this process.
    static func parse(_ rawArguments: [String]) throws -> LaunchArguments {
        guard let modeRaw = rawArguments.first else {
            throw LaunchArgumentError("missing native entry; expected run or stop")
        }
        guard let mode = LaunchArguments.Mode(rawValue: modeRaw) else {
            throw LaunchArgumentError("unsupported native entry \(modeRaw)")
        }
        let allowedFields = mode == .run ? runFields : stopFields
        var values: [String: String] = [:]
        var index = 1
        while index < rawArguments.count {
            let option = rawArguments[index]
            guard let field = optionField(option), allowedFields.contains(field) else {
                throw LaunchArgumentError("unknown native option \(option)")
            }
            guard index + 1 < rawArguments.count else {
                throw LaunchArgumentError("\(option) requires a value")
            }
            index += 1
            let value = rawArguments[index]
            index += 1
            guard values[field] == nil else {
                throw LaunchArgumentError("duplicate native option \(option)")
            }
            values[field] = value
        }

        let productRoot = try requiredAbsolutePath(values, field: "product-root", option: "--product-root")
        switch mode {
        case .stop:
            let corePath = try optionalAbsolutePath(values, field: "core-path", option: "--core-path")
            return LaunchArguments(
                mode: mode,
                productRoot: productRoot,
                corePath: corePath,
                dataDirectory: nil,
                coreIdentity: nil,
                dataRootDigest: nil
            )
        case .run:
            let corePath = try requiredAbsolutePath(values, field: "core-path", option: "--core-path")
            let dataDirectory = try requiredAbsolutePath(values, field: "data-dir", option: "--data-dir")
            let coreIdentity = try requiredValue(values, field: "core-identity", option: "--core-identity")
            let dataRootDigest = try requiredValue(values, field: "data-root-digest", option: "--data-root-digest")
            guard coreIdentity.range(of: coreIdentityPattern, options: .regularExpression) != nil else {
                throw LaunchArgumentError("--core-identity must name the verified Core identity")
            }
            guard dataRootDigest.range(of: digestPattern, options: .regularExpression) != nil else {
                throw LaunchArgumentError("--data-root-digest must be a data directory digest")
            }
            return LaunchArguments(
                mode: mode,
                productRoot: productRoot,
                corePath: corePath,
                dataDirectory: dataDirectory,
                coreIdentity: coreIdentity,
                dataRootDigest: dataRootDigest
            )
        }
    }

    private static func optionField(_ option: String) -> String? {
        guard option.hasPrefix("--"), option.count > 2 else { return nil }
        return String(option.dropFirst(2))
    }

    private static func requiredValue(_ values: [String: String], field: String, option: String) throws -> String {
        guard let value = values[field], !value.isEmpty else {
            throw LaunchArgumentError("\(option) is required")
        }
        return value
    }

    private static func requiredAbsolutePath(_ values: [String: String], field: String, option: String) throws -> String {
        let value = try requiredValue(values, field: field, option: option)
        return try absolutePath(value, option: option)
    }

    private static func optionalAbsolutePath(_ values: [String: String], field: String, option: String) throws -> String? {
        guard let value = values[field] else { return nil }
        return try absolutePath(value, option: option)
    }

    private static func absolutePath(_ value: String, option: String) throws -> String {
        guard value.hasPrefix("/"), !value.contains("\0") else {
            throw LaunchArgumentError("\(option) must be an absolute path")
        }
        return value
    }
}
