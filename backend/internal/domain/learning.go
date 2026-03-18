package domain

// Subject 表示一个学科（如：英语、语文、数学等）。
type Subject struct {
	Code string // 如 english / chinese / math
	Name string // 展示名：英语 / 语文 / 数学
}

// Tool 表示某个学科下的一类学习工具（如：英语听写工具、语文诗词工具等）。
type Tool struct {
	Code        string // 如 english_word, chinese_poem
	SubjectCode string // 所属学科 code
	Name        string // 工具展示名
	Description string // 简要说明
}

// PracticeMode 表示某个工具下的一种练习模式（如听写、选择题、填空等）。
type PracticeMode struct {
	Code        string
	ToolCode    string // 所属工具 code
	Name        string
	Description string
	Enabled     bool
}

// GradeLevel 表示年级/学段信息（如 G1、G2 或更细粒度）。
type GradeLevel struct {
	Code string // 如 G1, G2, Junior1 等
	Name string // 展示名：一年级 / 二年级 / 初一
}

