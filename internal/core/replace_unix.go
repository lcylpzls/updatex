//go:build !windows

package core

import (
	"os"

	"github.com/lcylpzls/errx"
)

// 以下可替换变量仅用于测试注入，生产环境保持 os 默认实现。
var (
	renameFile = os.Rename
	removeFile = os.Remove
	statFile   = os.Stat
	chmodFile  = os.Chmod
)

// replaceExecutable Unix 原子替换：备份当前版本后 rename 覆盖目标，
// 失败时自动回滚。返回 restart=false（Unix 替换即时生效）。
func replaceExecutable(current, newFile, _ string) (bool, error) {
	if err := chmodFile(newFile, 0o755); err != nil {
		return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "设置执行权限失败")
	}
	backup := current + ".bak"
	if _, err := statFile(current); err == nil {
		if err := renameFile(current, backup); err != nil {
			return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "备份当前版本失败")
		}
	}
	if err := renameFile(newFile, current); err != nil {
		if _, statErr := statFile(backup); statErr == nil {
			if rbErr := renameFile(backup, current); rbErr != nil {
				return false, errx.Wrap(err, errx.KindUnavailable, CodeRollbackFailed, "替换失败且回滚失败")
			}
		}
		return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "原子替换失败")
	}
	removeFile(backup)
	return false, nil
}

// bootstrap Windows 延迟替换在 Unix 无操作。
func bootstrap(_ string) error {
	return nil
}
