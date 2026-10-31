package distribution

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

type releaseManifest struct {
	Version  string
	Revision string
	Assets   map[string]bool
}

var releaseVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var revisionID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
var releaseAsset = regexp.MustCompile(`^(taskbelay-(darwin|linux)-(amd64|arm64)|taskbelay-windows-(amd64|arm64)\.exe|install\.sh|install\.ps1|SHA256SUMS)$`)

func parseRelease(data, asset string) (releaseManifest, error) {
	manifest := releaseManifest{Assets: map[string]bool{}}
	var wire struct {
		Version  string   `json:"version"`
		Revision string   `json:"revision"`
		Assets   []string `json:"assets"`
	}
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return manifest, fmt.Errorf("发布元数据无效: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return manifest, fmt.Errorf("发布元数据包含多余内容")
	}
	if !releaseVersion.MatchString(wire.Version) || !semver.IsValid(wire.Version) || !revisionID.MatchString(wire.Revision) {
		return manifest, fmt.Errorf("发布版本或构建身份无效")
	}
	manifest.Version, manifest.Revision = wire.Version, wire.Revision
	for _, name := range wire.Assets {
		if !releaseAsset.MatchString(name) || manifest.Assets[name] {
			return manifest, fmt.Errorf("发布资产无效或重复: %s", name)
		}
		manifest.Assets[name] = true
	}
	if manifest.Version == "" || manifest.Revision == "" || !manifest.Assets[asset] || !manifest.Assets["SHA256SUMS"] {
		return manifest, fmt.Errorf("发布元数据缺少版本、构建身份或 %s 资产", asset)
	}
	return manifest, nil
}
