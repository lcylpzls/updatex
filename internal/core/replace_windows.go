//go:build windows

package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lcylpzls/errx"
)

// 以下可替换变量仅用于测试注入，生产环境保持 os 默认实现。
var (
	renameFile = os.Rename
	removeFile = os.Remove
	statFile   = os.Stat
	readFile   = os.ReadFile
	writeFile  = os.WriteFile
	closeFile  = (*os.File).Close
)

// replaceExecutable Windows 延迟替换：把已校验的新版本落位为
// 目标.new，原子写入目标.pending 标记，返回 restart=true。
func replaceExecutable(current, newFile, version string) (bool, error) {
	if current == "" || newFile == "" || version == "" {
		return false, errx.New(errx.KindInvalid, CodeInvalidConfig, "替换路径与版本不能为空")
	}
	newPath := current + ".new"
	if err := renameFile(newFile, newPath); err != nil {
		return false, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "暂存新版本失败")
	}
	if err := writePending(current, version); err != nil {
		removeFile(newPath)
		return false, err
	}
	return true, nil
}

// writePending 原子写入更新标记（临时文件 + rename）。
func writePending(current, version string) error {
	pending := current + ".pending"
	tmp, err := createTempFile(filepath.Dir(current), "updatex-pending-*")
	if err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "创建更新标记临时文件失败")
	}
	defer func() {
		_ = tmp.Close() // 真实关闭，保证 Windows 上可删除
		_ = os.Remove(tmp.Name())
	}()
	if err := closeFile(tmp); err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "关闭更新标记临时文件失败")
	}
	if err := writeFile(tmp.Name(), []byte(version), 0o600); err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "写入更新标记失败")
	}
	if err := renameFile(tmp.Name(), pending); err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "落位更新标记失败")
	}
	return nil
}

// bootstrap 处理 Windows 启动时替换：存在 .pending 则替换 .new 并清理标记；
// 失败时保留标记（下次启动重试）。
func bootstrap(executablePath string) error {
	pending := executablePath + ".pending"
	data, err := readFile(pending)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "读取更新标记失败")
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return errx.Wrap(ErrReplaceFailed, errx.KindUnavailable, CodeReplaceFailed, "更新标记内容为空")
	}
	newPath := executablePath + ".new"
	if _, err := statFile(newPath); err != nil {
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "新版本文件缺失")
	}
	oldPath := executablePath + ".old"
	if _, err := statFile(executablePath); err == nil {
		if err := renameFile(executablePath, oldPath); err != nil {
			return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "备份当前版本失败")
		}
	}
	if err := renameFile(newPath, executablePath); err != nil {
		if _, statErr := statFile(oldPath); statErr == nil {
			if rbErr := renameFile(oldPath, executablePath); rbErr != nil {
				return errx.Wrap(err, errx.KindUnavailable, CodeRollbackFailed, "替换失败且回滚失败")
			}
		}
		return errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "替换新版本失败")
	}
	removeFile(pending)
	removeFile(oldPath)
	return nil
}
