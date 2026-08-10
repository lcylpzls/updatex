package updatex

import (
	"io"

	"github.com/lcylpzls/cryptox"
	"github.com/lcylpzls/errx"
)

// countReader 包装读取器并累计已读字节数（校验大小上限用）。
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// verifySHA256 流式校验：边读边哈希，限制读取上限。
func verifySHA256(r io.Reader, expected string, maxBytes int64) (int64, error) {
	if _, err := parseHexSHA256(expected); err != nil {
		return 0, err
	}
	cr := &countReader{r: io.LimitReader(r, maxBytes+1)}
	got, err := cryptox.SHA256Hex(cr)
	if err != nil {
		return cr.n, errx.Wrap(err, errx.KindUnavailable, CodeDownloadFailed, "资产读取失败")
	}
	if cr.n > maxBytes {
		return cr.n, errx.New(errx.KindInvalid, CodeDownloadFailed, "资产超出大小上限")
	}
	if !cryptox.ConstantTimeEquals([]byte(got), []byte(expected)) {
		return cr.n, ErrChecksumMismatch
	}
	return cr.n, nil
}
