// Package version 暴露构建版本信息，供日志与 /api/status 的追加字段使用。
package version

// Version 是 Go 实现的版本标识；发布时由构建参数覆盖。
var Version = "0.3.0-dev"

// String 返回当前版本。
func String() string {
	return Version
}
