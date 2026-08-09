package updatex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
)

// TestVerifySHA256 覆盖流式校验分支。
func TestVerifySHA256(t *testing.T) {
	content := []byte("update binary content")
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	n, err := verifySHA256(strings.NewReader(string(content)), want, 1024)
	if err != nil || n != int64(len(content)) {
		t.Fatalf("校验应成功：n=%d err=%v", n, err)
	}
	if _, err := verifySHA256(strings.NewReader(string(content)), strings.Repeat("00", 32), 1024); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("校验和不匹配应报错，实际：%v", err)
	}
	if _, err := verifySHA256(strings.NewReader(string(content)), want, 10); err == nil {
		t.Fatal("超出大小上限应报错")
	}
	if _, err := verifySHA256(strings.NewReader(string(content)), "bad", 1024); !errx.Is(err, CodeManifestInvalid) {
		t.Fatalf("坏校验和格式应报错，实际：%v", err)
	}
	if _, err := verifySHA256(io.MultiReader(strings.NewReader(string(content)), failingReader{}), want, 1024); err == nil {
		t.Fatal("读取错误应报错")
	}
}

// failingReader 读取即失败的读取器。
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("读取故障") }
