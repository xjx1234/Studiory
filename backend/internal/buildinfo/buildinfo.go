package buildinfo

// 这些变量通过 go build -ldflags 注入。
// 未注入时保留 unknown，方便本地开发和测试。
var (
	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}
