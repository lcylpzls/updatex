package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/testx"
)

// TestApplyResolverResolvesAndReturnsResolvedURL 覆盖解析器生效与结果回填。
func TestApplyResolverResolvesAndReturnsResolvedURL(t *testing.T) {
	stubReplace(t, func(_, _, _ string) (bool, error) { return false, nil })
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	calls := 0
	resolver := func(asset Asset) string {
		calls++
		return srv.URL + "/download"
	}
	u, _ := New(Config{
		Source:           &stubSource{manifest: newStubManifest("1.1.0", "https://update.invalid/download", sha)},
		CurrentVersion:   "1.0.0",
		ExecutablePath:   "x",
		AllowHTTP:        true,
		AssetURLResolver: resolver,
	})
	info, err := u.Apply(context.Background())
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, calls, 1)
	testx.RequireEqual(t, info.Asset.URL, srv.URL+"/download")
}

// TestApplyResolverNotCalledWhenSignatureInvalid 覆盖“先验签后解析”顺序。
func TestApplyResolverNotCalledWhenSignatureInvalid(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	testx.RequireNoError(t, err)
	m := newStubManifest("1.1.0", "https://update.invalid/download", strings.Repeat("ab", 32))
	signManifest(t, m, priv)
	tampered := *m
	tampered.Notes = "被篡改"
	calls := 0
	u, _ := New(Config{
		Source:           &stubSource{manifest: &tampered},
		CurrentVersion:   "1.0.0",
		ExecutablePath:   "x",
		VerifyPublicKey:  pub,
		AssetURLResolver: func(Asset) string { calls++; return "https://x/download" },
	})
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("篡改清单应校验失败，实际：%v", err)
	}
	testx.RequireEqual(t, calls, 0)
}

// TestApplyResolverEmpty 覆盖解析结果为空。
func TestApplyResolverEmpty(t *testing.T) {
	u, _ := New(Config{
		Source:           &stubSource{manifest: newStubManifest("1.1.0", "https://update.invalid/download", strings.Repeat("ab", 32))},
		CurrentVersion:   "1.0.0",
		ExecutablePath:   "x",
		AllowHTTP:        true,
		AssetURLResolver: func(Asset) string { return "" },
	})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("空解析结果应报下载失败，实际：%v", err)
	}
}

// TestApplyResolverHTTPSEnforced 覆盖解析后的 HTTPS 校验分支。
func TestApplyResolverHTTPSEnforced(t *testing.T) {
	u, _ := New(Config{
		Source:           &stubSource{manifest: newStubManifest("1.1.0", "https://update.invalid/download", strings.Repeat("ab", 32))},
		CurrentVersion:   "1.0.0",
		ExecutablePath:   "x",
		AssetURLResolver: func(Asset) string { return "http://x/download" },
	})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("明文解析结果应被拒绝，实际：%v", err)
	}

	// 允许明文时，解析后的 http 地址可正常下载。
	stubReplace(t, func(_, _, _ string) (bool, error) { return false, nil })
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u2, _ := New(Config{
		Source:           &stubSource{manifest: newStubManifest("1.1.0", "https://update.invalid/download", sha)},
		CurrentVersion:   "1.0.0",
		ExecutablePath:   "x",
		AllowHTTP:        true,
		AssetURLResolver: func(Asset) string { return srv.URL + "/download" },
	})
	if _, err := u2.Apply(context.Background()); err != nil {
		t.Fatalf("允许明文时解析地址应可下载：%v", err)
	}
}
