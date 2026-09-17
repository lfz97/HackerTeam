package localexec

// 框架以工具集名为前缀给工具名加前缀（LocalExec_<tool>）。
const localExecToolSetName = "LocalExec"

const (
	runToolName       = "run"
	statusToolName    = "status"
	outputToolName    = "output"
	interveneToolName = "intervene"
	killToolName      = "kill"
)

// Mappers 由 tools 包实现（*tools.Mappers 满足本接口）；localexec 自定义同形接口，避免跨包依赖。
type Mappers interface {
	AddMapping(Name string, In []string, Out []string)
}

// InjectMapper 注册 localexec 各工具的参数/结果提取字段。工具名为框架加前缀后的实际名。
func InjectMapper(m Mappers) {
	prefix := localExecToolSetName + "_"
	// id 放在 output 前：结果整体截断 200 rune，长输出会把尾部的 id 挤掉
	m.AddMapping(prefix+runToolName, []string{"process", "args"}, []string{"id", "output"})
	m.AddMapping(prefix+statusToolName, []string{"id"}, []string{"status"})
	m.AddMapping(prefix+outputToolName, []string{"id"}, []string{})
	m.AddMapping(prefix+interveneToolName, []string{"id", "signal"}, []string{"msg"})
	m.AddMapping(prefix+killToolName, []string{"id"}, []string{"status"})
}
