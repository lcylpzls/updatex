package core

import (
	"strconv"
	"strings"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/validx"
)

// semver 轻量语义化版本（主.次.补丁，可选预发布与构建元数据）。
type semver struct {
	major      int64
	minor      int64
	patch      int64
	prerelease string
	build      string
}

// parseVersion 解析语义化版本；接受可选小写 v 前缀（如 v1.2.3），
// 与 git tag 惯例及主流 semver 库行为一致。
func parseVersion(s string) (semver, error) {
	var v semver
	if s == "" {
		return v, ErrInvalidVersion
	}
	// 格式判定复用 validx 内置 semver 规则（支持可选小写 v 前缀）。
	if err := validx.ValidateField(s, "semver"); err != nil {
		return v, errInvalidVersion(s)
	}
	s = strings.TrimPrefix(s, "v")
	core := s
	if i := strings.IndexByte(s, '+'); i >= 0 {
		v.build = s[i+1:]
		core = s[:i]
	}
	if i := strings.IndexByte(core, '-'); i >= 0 {
		v.prerelease = core[i+1:]
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	nums := []*int64{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return v, errInvalidVersion(s)
		}
		*nums[i] = n
	}
	// validx semver 允许数字段前导零，updatex 语义更严：预发布数字段拒绝前导零。
	if v.prerelease != "" {
		for _, part := range strings.Split(v.prerelease, ".") {
			if len(part) > 1 && part[0] == '0' {
				return v, errInvalidVersion(s)
			}
		}
	}
	return v, nil
}

// compare 比较版本：a<b 返回 -1，a==b 返回 0，a>b 返回 1。
func compareVersion(a, b semver) int {
	switch {
	case a.major != b.major:
		return cmpInt(a.major, b.major)
	case a.minor != b.minor:
		return cmpInt(a.minor, b.minor)
	case a.patch != b.patch:
		return cmpInt(a.patch, b.patch)
	}
	// 无预发布 > 有预发布。
	switch {
	case a.prerelease == "" && b.prerelease == "":
		return 0
	case a.prerelease == "":
		return 1
	case b.prerelease == "":
		return -1
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

// comparePrerelease 按点分段比较预发布标识符。
func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := compareIdent(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmpInt(int64(len(as)), int64(len(bs)))
}

// compareIdent 比较单个预发布标识符（数字段数值，字母段字典序）。
func compareIdent(a, b string) int {
	an, aerr := strconv.ParseInt(a, 10, 64)
	bn, berr := strconv.ParseInt(b, 10, 64)
	switch {
	case aerr == nil && berr == nil:
		return cmpInt(an, bn)
	case aerr == nil:
		return -1 // 数字段 < 字母段。
	case berr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// cmpInt 比较整数。
func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// errInvalidVersion 构造版本错误。
func errInvalidVersion(s string) error {
	return errx.Wrap(ErrInvalidVersion, errx.KindInvalid, CodeInvalidVersion, "非法版本号："+s)
}
