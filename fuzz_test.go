package updatex

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// FuzzParseVersion 模糊测试版本解析：任意输入不得 panic。
func FuzzParseVersion(f *testing.F) {
	for _, seed := range []string{"", "1.0.0", "1.2.3-alpha.1+build.5", "01.2.3", "bad"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := parseVersion(s)
		if err == nil {
			if v.major < 0 || v.minor < 0 || v.patch < 0 {
				t.Fatalf("版本分量不能为负：%+v", v)
			}
			if got := compareVersion(v, v); got != 0 {
				t.Fatalf("版本应自相等：%+v got=%d", v, got)
			}
		}
	})
}

// FuzzManifest 模糊测试清单解析：任意输入不得 panic。
func FuzzManifest(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"version":"1.0.0","platforms":{"x_y":{"url":"https://x/a","sha256":"` + strings.Repeat("ab", 32) + `"}}}`),
		[]byte("not-json"),
		nil,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := ParseManifest(data)
		if err == nil && m != nil {
			if m.Version == "" || len(m.Platforms) == 0 {
				t.Fatalf("解析成功的清单必须含版本与平台资产")
			}
			asset, err := m.AssetFor("x", "y")
			if err == nil {
				want, ok := m.Platforms["x_y"]
				if !ok || asset != want {
					t.Fatalf("资产选择结果与清单不一致")
				}
			}
		}
	})
}

// FuzzVerifySHA256 模糊测试流式校验：任意输入不得 panic。
func FuzzVerifySHA256(f *testing.F) {
	sum := sha256.Sum256([]byte("update"))
	f.Add([]byte("update"), hex.EncodeToString(sum[:]), int64(1024))
	f.Add([]byte("data"), "bad", int64(-1))
	f.Fuzz(func(t *testing.T, data []byte, sha string, limit int64) {
		_, _ = verifySHA256(strings.NewReader(string(data)), sha, limit)
	})
}

// FuzzVerifySignature 模糊测试签名校验：任意输入不得 panic。
func FuzzVerifySignature(f *testing.F) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(`{"version":"1.0.0","platforms":{"x_y":{"url":"https://x/a","sha256":"`+strings.Repeat("ab", 32)+`"}}}`), []byte(pub), []byte("c2ln"))
	f.Add([]byte(nil), []byte(nil), []byte(nil))
	f.Fuzz(func(t *testing.T, data, key, sig []byte) {
		m, err := ParseManifest(data)
		if err == nil {
			m.Signature = string(sig)
			_ = m.VerifySignature(key)
		}
	})
}
