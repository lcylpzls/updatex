package updatex

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/lcylpzls/logx"
)

// signManifest 使用私钥对清单签名（测试辅助）。
func signManifest(t *testing.T, m *Manifest, priv ed25519.PrivateKey) {
	t.Helper()
	payload, err := m.signedPayload()
	testx.RequireNoError(t, err)

	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload))
}

// testLogger 构造写入丢弃目标的日志器。
func testLogger() logx.Logger {
	logger, err := logx.NewBuilder().EnableWriter(io.Discard, logx.InfoLevel).Build()
	if err != nil {
		panic(err)
	}
	return logger
}

// stubSource 可配置的发布源测试桩。
type stubSource struct {
	manifest *Manifest
	err      error
}

func (s *stubSource) Latest(context.Context) (*Manifest, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.manifest, nil
}

// newStubManifest 构造当前平台可用的清单。
func newStubManifest(version, assetURL, assetSHA string) *Manifest {
	return &Manifest{
		Version: version,
		Platforms: map[string]Asset{
			runtime.GOOS + "_" + runtime.GOARCH: {
				URL:    assetURL,
				SHA256: assetSHA,
				Size:   3,
			},
		},
	}
}

// assetServer 提供下载与错误路径的资产服务。
func assetServer(t interface {
	Helper()
}, content string) (*httptest.Server, string) {
	sum := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download":
			_, _ = w.Write([]byte(content))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	return srv, sha
}
