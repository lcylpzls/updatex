package updatex

import (
	"errors"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
)

// TestParseManifest 覆盖清单解析分支。
func TestParseManifest(t *testing.T) {
	good := `{"version":"1.1.0","published_at":"2026-08-09T00:00:00Z",` +
		`"notes":"升级","platforms":{"linux_amd64":{"url":"https://x/a","sha256":"` +
		strings.Repeat("ab", 32) + `","size":10}},"signature":"sig"}`
	m, err := ParseManifest([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.1.0" || m.Notes != "升级" || m.Signature != "sig" ||
		m.PublishedAt.IsZero() || len(m.raw) == 0 {
		t.Fatalf("清单解析不符：%+v", m)
	}
	asset, err := m.AssetFor("linux", "amd64")
	if err != nil || asset.Size != 10 {
		t.Fatalf("资产选择失败：%+v err=%v", asset, err)
	}
	if _, err := m.AssetFor("windows", "amd64"); !errors.Is(err, ErrPlatformUnsupported) {
		t.Fatalf("平台缺失应报错，实际：%v", err)
	}
	for name, data := range map[string][]byte{
		"空":        nil,
		"超大":       make([]byte, 1<<20+1),
		"坏 JSON":   []byte("not-json"),
		"缺版本":      []byte(`{"platforms":{}}`),
		"缺平台":      []byte(`{"version":"1.0.0"}`),
		"坏版本":      []byte(`{"version":"bad","platforms":{"linux_amd64":{"url":"x","sha256":"` + strings.Repeat("ab", 32) + `"}}}`),
		"资产缺 URL":  []byte(`{"version":"1.0.0","platforms":{"linux_amd64":{"sha256":"` + strings.Repeat("ab", 32) + `"}}}`),
		"资产坏校验和":   []byte(`{"version":"1.0.0","platforms":{"linux_amd64":{"url":"x","sha256":"short"}}}`),
		"资产负大小":    []byte(`{"version":"1.0.0","platforms":{"linux_amd64":{"url":"x","sha256":"` + strings.Repeat("ab", 32) + `","size":-1}}}`),
		"校验和含非法字符": []byte(`{"version":"1.0.0","platforms":{"linux_amd64":{"url":"x","sha256":"` + strings.Repeat("zz", 32) + `"}}}`),
	} {
		if _, err := ParseManifest(data); !errx.Is(err, CodeManifestInvalid) &&
			!errx.Is(err, CodeInvalidVersion) {
			t.Fatalf("%s 应报错，实际：%v", name, err)
		}
	}
}

// TestParseHexSHA256 覆盖校验和格式解析。
func TestParseHexSHA256(t *testing.T) {
	b, err := parseHexSHA256(strings.Repeat("ab", 32))
	if err != nil || len(b) != 32 {
		t.Fatalf("合法校验和应解析：%v", err)
	}
	if _, err := parseHexSHA256(strings.Repeat("a", 63)); err == nil {
		t.Fatal("长度不足应报错")
	}
	if _, err := parseHexSHA256(strings.Repeat("g", 64)); err == nil {
		t.Fatal("非法字符应报错")
	}
	if _, err := parseHexSHA256(strings.Repeat("AB", 32)); err != nil {
		t.Fatalf("大写十六进制应解析：%v", err)
	}
}
