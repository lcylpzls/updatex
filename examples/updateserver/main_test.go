package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	testx "github.com/lcylpzls/testx"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/updatex/examples/internal/testutil"
	"github.com/lcylpzls/updatex/examples/internal/updateserver"
)

// testLogger 构造写入丢弃目标的日志器。
func testLogger() logx.Logger {
	logger, err := logx.NewBuilder().EnableWriter(io.Discard, logx.InfoLevel).Build()
	if err != nil {
		panic(err)
	}
	return logger
}

// TestHTTP3UpdateServer 验证 HTTP/3 服务端提供清单与资产。
func TestHTTP3UpdateServer(t *testing.T) {
	certFile, keyFile := testutil.WriteTestCert(t)
	asset := []byte("新版本二进制内容")
	var protos []string
	var mu sync.Mutex
	record := func(p string) {
		mu.Lock()
		protos = append(protos, p)
		mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, addr, err := updateserver.StartAndWait(ctx, updateserver.Config{
		Version:   "1.1.0",
		Notes:     "HTTP/3 示例更新",
		Asset:     asset,
		OnRequest: record,
	}, certFile, keyFile, "127.0.0.1:0", testLogger())
	testx.RequireNoError(t, err)

	defer s.Stop(context.Background())

	client, err := httpx.New(
		httpx.WithTimeout(10*time.Second),
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	testx.RequireNoError(t, err)

	base := "https://" + addr

	resp, err := client.Get(ctx, base+"/update.json")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("清单状态码非 200：%d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	testx.RequireNoError(t, err)

	var m updatex.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("清单 JSON 解析失败：%v", err)
	}
	if m.Version != "1.1.0" || m.Notes != "HTTP/3 示例更新" || len(m.Platforms) == 0 {
		t.Fatalf("清单内容不符：%+v", m)
	}

	resp2, err := client.Get(ctx, base+"/download")
	testx.RequireNoError(t, err)

	defer resp2.Body.Close()
	body, err := io.ReadAll(resp2.Body)
	testx.RequireNoError(t, err)

	if string(body) != string(asset) {
		t.Fatalf("资产内容不符：%q", body)
	}

	mu.Lock()
	defer mu.Unlock()
	hitHTTP3 := false
	for _, p := range protos {
		if p == "HTTP/3.0" {
			hitHTTP3 = true
			break
		}
	}
	testx.RequireTrue(t, hitHTTP3)

}
