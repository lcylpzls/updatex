package updatex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lcylpzls/testx"
)

// TestVerifySHA256 覆盖流式校验分支。
func TestVerifySHA256(t *testing.T) {
	content := []byte("update binary content")
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	n, err := verifySHA256(strings.NewReader(string(content)), want, 1024)
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, n, int64(len(content)))
	_, err = verifySHA256(strings.NewReader(string(content)), strings.Repeat("00", 32), 1024)
	testx.RequireErrorIs(t, err, ErrChecksumMismatch)
	_, err = verifySHA256(strings.NewReader(string(content)), want, 10)
	testx.RequireError(t, err)
	_, err = verifySHA256(strings.NewReader(string(content)), "bad", 1024)
	testx.RequireErrCode(t, err, CodeManifestInvalid)
	_, err = verifySHA256(io.MultiReader(strings.NewReader(string(content)), failingReader{}), want, 1024)
	testx.RequireError(t, err)
}

// failingReader 读取即失败的读取器。
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("读取故障") }
