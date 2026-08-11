package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	testx "github.com/lcylpzls/testx"
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
	testx.RequireNoError(t, err)

	if m.Version != "1.1.0" || m.Notes != "升级" || m.Signature != "sig" ||
		m.PublishedAt.IsZero() {
		t.Fatalf("清单解析不符：%+v", m)
	}
	asset, err := m.AssetFor("linux", "amd64")
	if err != nil || asset.Size != 10 {
		t.Fatalf("资产选择失败：%+v err=%v", asset, err)
	}
	if _, err := m.AssetFor("windows", "amd64"); !errors.Is(err, ErrPlatformUnsupported) {
		t.Fatalf("平台缺失应报错，实际：%v", err)
	}
	// 带 v 前缀的版本应可解析（issue #1）。
	vManifest, err := ParseManifest([]byte(`{"version":"v1.1.0","platforms":{"x_y":` +
		`{"url":"https://x","sha256":"` + strings.Repeat("ab", 32) + `"}}}`))
	if err != nil || vManifest.Version != "v1.1.0" {
		t.Fatalf("带 v 前缀清单应解析成功：%+v err=%v", vManifest, err)
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

// TestVerifySignature 覆盖 Ed25519 签名校验分支。
func TestVerifySignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	testx.RequireNoError(t, err)

	m := &Manifest{Version: "1.1.0", Notes: "说明",
		Platforms: map[string]Asset{"x_y": {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}
	signManifest(t, m, priv)
	if err := m.VerifySignature(pub); err != nil {
		t.Fatalf("合法签名应通过：%v", err)
	}

	// 篡改清单。
	tampered := *m
	tampered.Notes = "被篡改"
	if err := tampered.VerifySignature(pub); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("篡改清单应校验失败，实际：%v", err)
	}

	// 签名缺失。
	noSig := &Manifest{Version: "1.1.0",
		Platforms: map[string]Asset{"x_y": {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}
	if err := noSig.VerifySignature(pub); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("缺签名应校验失败，实际：%v", err)
	}

	// 签名非 base64。
	badSig := *m
	badSig.Signature = "!!!"
	if err := badSig.VerifySignature(pub); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("坏签名应校验失败，实际：%v", err)
	}

	// 错误公钥。
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := m.VerifySignature(otherPub); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("错误公钥应校验失败，实际：%v", err)
	}

	// 公钥长度非法。
	if err := m.VerifySignature([]byte("short")); !errx.Is(err, CodeSignatureInvalid) {
		t.Fatalf("公钥长度非法应报错，实际：%v", err)
	}

	// 签名长度非法。
	shortSig := *m
	shortSig.Signature = base64.StdEncoding.EncodeToString([]byte("short"))
	if err := shortSig.VerifySignature(pub); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("短签名应校验失败，实际：%v", err)
	}

	// 签名载荷序列化失败。
	origMarshal := manifestMarshal
	manifestMarshal = func(any) ([]byte, error) { return nil, errors.New("序列化失败") }
	defer func() { manifestMarshal = origMarshal }()
	if err := m.VerifySignature(pub); !errx.Is(err, CodeSignatureInvalid) {
		t.Fatalf("载荷序列化失败应报错，实际：%v", err)
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
