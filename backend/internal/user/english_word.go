package user

// 英文单词工具 - 用户端领域层骨架
//
// 说明：
// - 这里先只定义结构体和接口，暂不实现具体业务逻辑。
// - 未来可以在此封装与题目生成、判分、会话管理等相关的领域规则。

// Mode 表示一种练习模式（听写、拼写、词义等）。
// 这里是对 domain.PracticeMode 的精简视图，专注于用户端需要的信息。
type Mode struct {
	Code        string
	Name        string
	Description string
	Enabled     bool
}

// Question 表示一次练习中的一道题目。
type Question struct {
	ID         string
	WordID     string
	PromptType string            // audio / text / image
	Prompt     string            // 对应资源的 URL 或文本
	Extra      map[string]any    // 预留扩展字段，如 hint、phonetic 等
}

// Session 表示一次练习会话。
type Session struct {
	ID        string
	Mode      string
	Grade     string
	Level     string
	WordSetID string
	Questions []Question
}

// Answer 表示用户对单个题目的作答。
type Answer struct {
	QuestionID string
	WordID     string
	UserAnswer string
	TimeUsedMs int64
}

// SessionResult 表示一次练习的整体结果。
type SessionResult struct {
	SessionID    string
	Score        int
	CorrectCount int
	WrongCount   int
	Details      []QuestionResult
}

// QuestionResult 表示单个题目的作答结果。
type QuestionResult struct {
	QuestionID    string
	WordID        string
	UserAnswer    string
	CorrectAnswer string
	IsCorrect     bool
	Explanation   string
	WordInfo      map[string]any // 例如 text、meaning、phonetic 等
}

// EnglishWordService 定义英文单词工具在用户端的核心能力接口。
//
// 具体实现可以在本包中通过结构体实现，
// 也可以在将来通过适配层调用 Python/Node 子服务。
type EnglishWordService interface {
	// ListModes 返回当前可用的练习模式。
	ListModes(grade, level string) ([]Mode, error)

	// CreateSession 创建一次新的练习会话。
	CreateSession(mode, grade, level, wordSetID string, questionCount int) (*Session, error)

	// SubmitAnswers 提交作答并返回结果。
	SubmitAnswers(sessionID string, answers []Answer) (*SessionResult, error)

	// GetSessionResult 获取某次练习的结果（用于回顾）。
	GetSessionResult(sessionID string) (*SessionResult, error)
}

