package core

import (
	"context"
	"errors"
	testx "github.com/lcylpzls/testx"
	"sync"
	"testing"
)

// traceCall 记录一次追踪调用。
type traceCall struct {
	name  string
	attrs map[string]string
	err   error
	ended bool
}

// fakeTraceHook 内存追踪钩子。
type fakeTraceHook struct {
	mu    sync.Mutex
	calls []traceCall
}

func (h *fakeTraceHook) Start(ctx context.Context, name string, attrs ...TraceAttr) (context.Context, func(error)) {
	h.mu.Lock()
	h.calls = append(h.calls, traceCall{name: name, attrs: map[string]string{}})
	for _, a := range attrs {
		h.calls[len(h.calls)-1].attrs[a.Key] = a.Value
	}
	h.mu.Unlock()
	return ctx, func(err error) {
		h.mu.Lock()
		h.calls[len(h.calls)-1].err = err
		h.calls[len(h.calls)-1].ended = true
		h.mu.Unlock()
	}
}

func (h *fakeTraceHook) snapshot() []traceCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]traceCall, len(h.calls))
	copy(out, h.calls)
	return out
}

// TestTraceHook 覆盖 Check/Apply 追踪埋点（成功与失败）。
func TestTraceHook(t *testing.T) {
	stubReplace(t, func(_, _, _ string) (bool, error) { return false, nil })
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	hook := &fakeTraceHook{}
	u, err := New(Config{
		Source:         &stubSource{manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0",
		ExecutablePath: "x",
		AllowHTTP:      true,
		TraceHook:      hook,
	})
	testx.RequireNoError(t, err)

	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 失败路径。
	uFail, _ := New(Config{
		Source:         &stubSource{err: ErrFetchFailed},
		CurrentVersion: "1.0.0",
		ExecutablePath: "x",
		TraceHook:      hook,
	})
	if _, err := uFail.Check(context.Background()); err == nil {
		t.Fatal("应返回拉取失败")
	}

	calls := hook.snapshot()
	if len(calls) != 3 {
		t.Fatalf("应调用 3 次追踪钩子，实际：%d", len(calls))
	}
	for i, c := range calls {
		if c.attrs["updatex.current_version"] != "1.0.0" || !c.ended {
			t.Fatalf("第 %d 次追踪调用不符：%+v", i, c)
		}
	}
	if calls[0].name != "updatex.check" || calls[1].name != "updatex.apply" ||
		calls[2].name != "updatex.check" {
		t.Fatalf("span 名不符：%+v", calls)
	}
	if calls[0].err != nil || calls[1].err != nil || !errors.Is(calls[2].err, ErrFetchFailed) {
		t.Fatalf("结束回调错误记录不符：%+v", calls)
	}
}
