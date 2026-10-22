package collector

// Paths 可注入的 proc/sys 根路径，便于单测用 fixture 替代真实 /proc、/sys。
type Paths struct {
	Proc string // 默认 "/proc"
	Sys  string // 默认 "/sys"
}

func defaultPaths() Paths {
	return Paths{Proc: "/proc", Sys: "/sys"}
}
