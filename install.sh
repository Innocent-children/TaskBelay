#!/usr/bin/env bash
set -euo pipefail

main() (
    local source_install=0 version=latest
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --source) source_install=1; shift ;;
            --version)
                [ "$#" -ge 2 ] || { echo '--version requires a release tag' >&2; return 1; }
                version="$2"; shift 2 ;;
            --help|-h)
                echo 'Usage: bash install.sh [--source | --version vX.Y.Z]'
                return ;;
            *) echo "Unknown option: $1" >&2; return 1 ;;
        esac
    done
    if [ "$source_install" -eq 1 ] && [ "$version" != latest ]; then
        echo '--source and --version cannot be combined' >&2
        return 1
    fi
    if [ "$version" != latest ] && ! [[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
        echo 'Invalid release tag' >&2
        return 1
    fi
    local system arch
    case "$(uname -s)" in
        Darwin) system=darwin ;;
        Linux) system=linux ;;
        *) echo 'Use install.ps1 on Windows; WSL uses this Linux installer.' >&2; return 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch=amd64 ;;
        arm64|aarch64) arch=arm64 ;;
        *) echo 'Supported architectures: amd64, arm64' >&2; return 1 ;;
    esac
    local install_dir="$HOME/.local/bin" marker_text='TaskBelay user installation v1'
    local target="$install_dir/taskbelay" marker="$install_dir/.taskbelay-install"
    mkdir -p "$install_dir"
    local install_lock="$install_dir/.taskbelay-install.lock"
    mkdir "$install_lock" || { echo "Installation already running, or stale lock: $install_lock" >&2; return 1; }
    local stage=''
    trap 'test -z "$stage" || { rm -f "$stage/taskbelay" "$stage/SHA256SUMS" "$stage/release-manifest.json" "$stage/marker"; rmdir "$stage"; }; rmdir "$install_lock"' EXIT
    if [ -L "$marker" ]; then
        echo 'Refusing to replace an installation marker symlink.' >&2
        return 1
    fi
    local taskbelay_link=0
    if [ -L "$target" ]; then
        if command -v go >/dev/null && go version -m "$target" | awk '$1 == "path" && $2 == "github.com/Innocent-children/taskbelay/cmd/taskbelay" { found = 1 } END { exit !found }'; then
            taskbelay_link=1
        else
            echo 'Cannot verify the existing link as TaskBelay; it has been preserved.' >&2
            return 1
        fi
    fi
    if { [ -e "$target" ] && [ ! -f "$target" ]; } ||
       { [ -e "$target" ] && [ ! -f "$marker" ] && [ "$taskbelay_link" -eq 0 ]; } ||
       { [ -e "$marker" ] && { [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$marker_text" ]; }; }; then
        echo "Existing files are not managed by TaskBelay: $target" >&2
        return 1
    fi
    stage="$(mktemp -d "$install_dir/.taskbelay-stage.XXXXXX")"
    if [ "$source_install" -eq 1 ]; then
        command -v go >/dev/null || { echo 'Source installation requires Go 1.26 or newer.' >&2; return 1; }
        local source_dir
        source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
        [ -f "$source_dir/go.mod" ] || { echo 'Run --source from a TaskBelay checkout.' >&2; return 1; }
        echo 'Building TaskBelay from source...'
        (cd "$source_dir" && GOWORK=off CGO_ENABLED=0 GOOS="$system" GOARCH="$arch" go build -trimpath -o "$stage/taskbelay" ./cmd/taskbelay)
    else
        command -v curl >/dev/null || { echo 'curl is required.' >&2; return 1; }
        local releases='https://github.com/Innocent-children/TaskBelay/releases' base
        if [ "$version" = latest ]; then base="$releases/latest/download"; else base="$releases/download/$version"; fi
        local asset="taskbelay-$system-$arch" expected actual release_version revision identity
        curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 300 "$base/release-manifest.json" -o "$stage/release-manifest.json"
        # 发布格式只包含三个字段和未转义的 ASCII 标识，不依赖额外的 JSON 工具。
        identity="$(awk -v asset="$asset" '
            function space() { sub(/^[ \t\r\n]+/, "", json) }
            function take(c) { space(); if (substr(json, 1, 1) != c) exit 1; json=substr(json, 2) }
            function string_value( value) {
                space(); if (match(json, /^"[A-Za-z0-9_.-]+"/) != 1) exit 1
                value=substr(json, 2, RLENGTH-2); json=substr(json, RLENGTH+1); return value
            }
            function asset_array( name) {
                take("["); space()
                if (substr(json, 1, 1) == "]") { take("]"); return }
                while (1) {
                    name=string_value()
                    if (assets[name]++ || name !~ /^(taskbelay-(darwin|linux)-(amd64|arm64)|taskbelay-windows-(amd64|arm64)\.exe|install\.sh|install\.ps1|SHA256SUMS)$/) exit 1
                    space(); if (substr(json, 1, 1) == "]") break
                    take(",")
                }
                take("]")
            }
            { json=json $0 "\n" }
            END {
                take("{")
                while (1) {
                    key=string_value(); if (seen[key]++) exit 1
                    take(":")
                    if (key == "version" || key == "revision") values[key]=string_value()
                    else if (key == "assets") asset_array()
                    else exit 1
                    space(); if (substr(json, 1, 1) == "}") break
                    take(",")
                }
                take("}"); space()
                if (json != "" || seen["version"] != 1 || seen["revision"] != 1 || assets[asset] != 1 || assets["SHA256SUMS"] != 1) exit 1
                print values["version"]; print values["revision"]
            }
        ' "$stage/release-manifest.json")" || { echo 'Invalid release metadata or missing platform asset; previous version retained.' >&2; return 1; }
        release_version="${identity%%$'\n'*}"
        revision="${identity#*$'\n'}"
        [[ "$release_version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] && [[ "$revision" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] || { echo 'Invalid release identity; previous version retained.' >&2; return 1; }
        [ "$version" = latest ] || [ "$version" = "$release_version" ] || { echo 'Release tag does not match metadata; previous version retained.' >&2; return 1; }
        version="$release_version"
        base="$releases/download/$release_version"
        echo "Downloading $asset ($version)..."
        curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 300 "$base/SHA256SUMS" -o "$stage/SHA256SUMS"
        expected="$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1 }' "$stage/SHA256SUMS")"
        [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || { echo 'Missing or invalid SHA-256 entry; previous version retained.' >&2; return 1; }
        curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 300 "$base/$asset" -o "$stage/taskbelay"
        if command -v sha256sum >/dev/null; then
            actual="$(sha256sum "$stage/taskbelay" | awk '{print $1}')"
        elif command -v shasum >/dev/null; then
            actual="$(shasum -a 256 "$stage/taskbelay" | awk '{print $1}')"
        else
            echo 'sha256sum or shasum is required; previous version retained.' >&2
            return 1
        fi
        expected="$(printf '%s' "$expected" | tr 'A-F' 'a-f')"
        [ "$actual" = "$expected" ] || { echo 'SHA-256 mismatch; previous version retained.' >&2; return 1; }
    fi
    chmod 755 "$stage/taskbelay"
    local installed_version
    installed_version="$("$stage/taskbelay" --version)"
    case "$installed_version" in TaskBelay\ *) ;; *) echo 'Not a TaskBelay executable; previous version retained.' >&2; return 1 ;; esac
    if [ "$source_install" -eq 0 ]; then
        case "$installed_version" in "TaskBelay $release_version ("*) ;; *) echo 'Executable version does not match release metadata; previous version retained.' >&2; return 1 ;; esac
    fi
    printf '%s\n' "$marker_text" > "$stage/marker"
    mv -f "$stage/taskbelay" "$target"
    mv -f "$stage/marker" "$marker"
    local profile login_profile="$HOME/.profile" path_line='export PATH="$HOME/.local/bin:$PATH"'
    if [ -f "$HOME/.bash_profile" ]; then
        login_profile="$HOME/.bash_profile"
    elif [ -f "$HOME/.bash_login" ]; then
        login_profile="$HOME/.bash_login"
    fi
    for profile in "$login_profile" "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc"; do
        if ! grep -Fqx "$path_line" "$profile" 2>/dev/null; then
            printf '\n# TaskBelay user commands\n%s\n' "$path_line" >> "$profile"
        fi
    done
    local quoted_target=${target//\'/\'\\\'\'}
    printf "\nInstalled %s\nRun now: '%s'\nOr reopen your terminal and run: taskbelay\nChoose Agent integration in the menu.\n" "$installed_version" "$quoted_target"
)

main "$@"
