package updatex

import (
	"errors"
	testx "github.com/lcylpzls/testx"
	"testing"
)

// TestParseVersion 覆盖版本解析分支。
func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want semver
	}{
		{"1.2.3", semver{major: 1, minor: 2, patch: 3}},
		{"0.0.1", semver{patch: 1}},
		{"10.20.30-alpha.1", semver{major: 10, minor: 20, patch: 30, prerelease: "alpha.1"}},
		{"1.0.0+build.5", semver{major: 1, build: "build.5"}},
		{"1.0.0-rc.1+build.2", semver{major: 1, prerelease: "rc.1", build: "build.2"}},
		{"v1.2.3", semver{major: 1, minor: 2, patch: 3}},
		{"v1.2.3-rc.1+build.2", semver{major: 1, minor: 2, patch: 3, prerelease: "rc.1", build: "build.2"}},
	}
	for _, tc := range cases {
		got, err := parseVersion(tc.in)
		testx.RequireNoError(t, err)

		testx.RequireEqual(t, got, tc.want)

	}
	for _, bad := range []string{"", "1.2", "1.2.3.4", "a.b.c", "01.2.3", "1.02.3", "1.2.3-", "1.2.3-01", "1.2.3-..1", "1.2.3-+x", "1.2.3-α", "v", "vv1.2.3", "V1.2.3"} {
		if _, err := parseVersion(bad); !errors.Is(err, ErrInvalidVersion) {
			t.Fatalf("%q 应报版本错误，实际：%v", bad, err)
		}
	}
}

// TestCompareVersion 覆盖版本比较分支。
func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.10", "1.0.9", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.2", -1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0+abc", "1.0.0+xyz", 0},
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3-rc.1", "1.2.3-rc.2", -1},
	}
	for _, tc := range cases {
		av, _ := parseVersion(tc.a)
		bv, _ := parseVersion(tc.b)
		if got := compareVersion(av, bv); got != tc.want {
			t.Fatalf("%q vs %q：got=%d want=%d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestCompareIdent 覆盖预发布数字/字母标识符比较边界。
func TestCompareIdent(t *testing.T) {
	if got := compareIdent("1", "alpha"); got != -1 {
		t.Fatalf("数字段应小于字母段，实际：%d", got)
	}
	if got := compareIdent("alpha", "1"); got != 1 {
		t.Fatalf("字母段应大于数字段，实际：%d", got)
	}
	if got := compareIdent("alpha", "beta"); got != -1 {
		t.Fatalf("字母段应按字典序比较，实际：%d", got)
	}
}

// TestCmpIntEqual 覆盖相等分支。
func TestCmpIntEqual(t *testing.T) {
	if got := cmpInt(5, 5); got != 0 {
		t.Fatalf("相等应返回 0，实际：%d", got)
	}
}
