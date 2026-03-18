// Package locales 使用 embed.FS 将翻译文件打包进二进制，
// 避免运行时依赖外部文件路径。
package locales

import "embed"

//go:embed *.toml
var FS embed.FS
