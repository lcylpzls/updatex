//go:build !windows

package updatex

import (
	"os"

	"github.com/lcylpzls/errx"
)

// replaceExecutable Unix 原子替换：rename 覆盖目标。
// 返回 restart=false（Unix 替换即时生效）。
func replaceExecutable(current, newFile string) (bool, error) {
	if err := os.Chmod(newFile, 0o755); err != nil {
		return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "设置执行权限失败")
	}
	if err := os.Rename(newFile, current); err != nil {
		return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "原子替换失败")
	}
	return false, nil
}

// bootstrap Windows 延迟替换在 Unix 无操作。
func bootstrap(_ string) error {
	return nil
}
