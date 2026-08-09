package updatex

import (
	"crypto/sha256"
	"crypto/subtle"
	"io"

	"github.com/lcylpzls/errx"
)

// verifySHA256 流式校验：边读边哈希，限制读取上限。
func verifySHA256(r io.Reader, expected string, maxBytes int64) (int64, error) {
	want, err := parseHexSHA256(expected)
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	written, err := io.Copy(h, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return written, errx.Wrap(err, errx.KindUnavailable, CodeDownloadFailed, "资产读取失败")
	}
	if written > maxBytes {
		return written, errx.New(errx.KindInvalid, CodeDownloadFailed, "资产超出大小上限")
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), want) != 1 {
		return written, ErrChecksumMismatch
	}
	return written, nil
}
