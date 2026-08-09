//go:build windows

package updatex

// replaceExecutable Windows 占位：v0.2.0 实现启动时替换。
func replaceExecutable(current, newFile string) (bool, error) {
	return true, ErrReplaceFailed
}

// bootstrap Windows 占位：v0.2.0 实现 .pending 处理。
func bootstrap(_ string) error {
	return nil
}
