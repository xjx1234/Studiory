// Package i18n 封装 go-i18n/v2 的初始化与辅助函数，
// 供 middleware 和 resp 包共用，避免循环依赖。
package i18n

import (
	"backend/locales"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// LocalizerKey 是注入到 Gin Context 中的 Key。
const LocalizerKey = "i18n:localizer"

// bundle 是全局翻译包，在 init 时初始化，进程内只初始化一次。
var bundle *i18n.Bundle

func init() {
	bundle = i18n.NewBundle(language.Chinese)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// 从 embed.FS 加载语言文件（文件名决定语言标签）
	mustLoad("active.zh.toml")
	mustLoad("active.en.toml")
}

func mustLoad(filename string) {
	if _, err := bundle.LoadMessageFileFS(locales.FS, filename); err != nil {
		panic("i18n: failed to load locale file " + filename + ": " + err.Error())
	}
}

// NewLocalizer 根据语言偏好字符串（Accept-Language 或 lang query param）创建 Localizer。
// 若未匹配到支持的语言则退回到简体中文。
func NewLocalizer(langs ...string) *i18n.Localizer {
	return i18n.NewLocalizer(bundle, append(langs, language.Chinese.String())...)
}

// GetLocalizer 从 Gin Context 取出 Localizer；若不存在则返回默认中文 Localizer。
func GetLocalizer(c *gin.Context) *i18n.Localizer {
	if v, exists := c.Get(LocalizerKey); exists {
		if l, ok := v.(*i18n.Localizer); ok {
			return l
		}
	}
	return NewLocalizer()
}

// Localize 翻译指定消息 ID，翻译失败时直接返回 msgID（保证不 panic）。
func Localize(c *gin.Context, msgID string) string {
	l := GetLocalizer(c)
	msg, err := l.Localize(&i18n.LocalizeConfig{MessageID: msgID})
	if err != nil {
		return msgID
	}
	return msg
}

// LocalizeWithData 支持模板变量的翻译，如 "Hello, {{.Name}}"。
func LocalizeWithData(c *gin.Context, msgID string, data map[string]any) string {
	l := GetLocalizer(c)
	msg, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:    msgID,
		TemplateData: data,
	})
	if err != nil {
		return msgID
	}
	return msg
}
