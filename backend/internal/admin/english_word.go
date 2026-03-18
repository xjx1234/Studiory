package admin

// 英文单词工具 - 管理端领域层骨架
//
// 说明：
// - 这里先只定义结构体和接口，暂不实现具体业务逻辑。
// - 未来可在此处理单词集、单词管理和统计报表等后台功能。

// WordSet 表示一个单词集（如某年级某单元）。
// 未来可以与通用的“资源集合（如题库、知识点集合）”抽象对齐。
type WordSet struct {
	ID        string
	Name      string
	Grade     string
	WordCount int
	Enabled   bool
}

// Word 表示一个具体的单词。
type Word struct {
	ID       string
	Text     string
	Meaning  string
	Phonetic string
	AudioURL string
	Tags     []string
}

// WordSetList 表示分页后的单词集列表。
type WordSetList struct {
	Items []WordSet
	Total int
}

// WordList 表示分页后的单词列表。
type WordList struct {
	Items []Word
	Total int
}

// EnglishWordAdminService 定义英文单词工具在管理端的核心能力接口。
type EnglishWordAdminService interface {
	// ListWordSets 返回单词集列表。
	ListWordSets(grade string, page, pageSize int, keyword string) (*WordSetList, error)

	// SaveWordSet 创建或更新单词集。
	SaveWordSet(set *WordSet) (string, error)

	// ListWords 返回某个单词集下的单词列表。
	ListWords(wordSetID string, page, pageSize int, keyword string) (*WordList, error)

	// SaveWord 创建或更新单词。
	SaveWord(wordSetID string, word *Word) (string, error)
}

