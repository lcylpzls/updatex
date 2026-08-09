package updatex

import (
	"encoding/json"
	"time"

	"github.com/lcylpzls/errx"
)

// Asset 单平台更新资产。
type Asset struct {
	// URL 下载地址。
	URL string `json:"url"`
	// SHA256 资产校验和（64 位十六进制）。
	SHA256 string `json:"sha256"`
	// Size 资产字节数。
	Size int64 `json:"size"`
}

// Manifest 发布清单。
type Manifest struct {
	// Version 目标版本（语义化版本）。
	Version string `json:"version"`
	// PublishedAt 发布时间。
	PublishedAt time.Time `json:"published_at"`
	// Notes 变更说明。
	Notes string `json:"notes"`
	// Platforms 平台资产（键为 GOOS_GOARCH）。
	Platforms map[string]Asset `json:"platforms"`
	// Signature 清单签名（Ed25519，可选）。
	Signature string `json:"signature,omitempty"`

	raw []byte
}

// ParseManifest 解析清单并保留原文字节（签名校验用）。
func ParseManifest(data []byte) (*Manifest, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, ErrManifestInvalid
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errx.Wrap(err, errx.KindInvalid, CodeManifestInvalid, "清单 JSON 解析失败")
	}
	if m.Version == "" || len(m.Platforms) == 0 {
		return nil, errInvalidManifest("清单缺少版本或平台资产")
	}
	if _, err := parseVersion(m.Version); err != nil {
		return nil, err
	}
	for name, asset := range m.Platforms {
		if asset.URL == "" || len(asset.SHA256) != 64 || asset.Size < 0 {
			return nil, errInvalidManifest("平台资产字段非法：" + name)
		}
		if _, err := parseHexSHA256(asset.SHA256); err != nil {
			return nil, err
		}
	}
	m.raw = append([]byte(nil), data...)
	return &m, nil
}

// AssetFor 返回指定平台的资产；不存在返回 ErrPlatformUnsupported。
func (m *Manifest) AssetFor(goos, goarch string) (Asset, error) {
	asset, ok := m.Platforms[goos+"_"+goarch]
	if !ok {
		return Asset{}, ErrPlatformUnsupported
	}
	return asset, nil
}

// errInvalidManifest 构造清单错误。
func errInvalidManifest(msg string) error {
	return errx.New(errx.KindInvalid, CodeManifestInvalid, msg)
}

// parseHexSHA256 校验 SHA256 十六进制格式。
func parseHexSHA256(s string) ([]byte, error) {
	if len(s) != 64 {
		return nil, errInvalidManifest("SHA256 必须为 64 位十六进制")
	}
	out := make([]byte, 32)
	for i := 0; i < 32; i++ {
		hi, ok1 := hexVal(s[i*2])
		lo, ok2 := hexVal(s[i*2+1])
		if !ok1 || !ok2 {
			return nil, errInvalidManifest("SHA256 含非法字符")
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

// hexVal 十六进制字符转数值。
func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
