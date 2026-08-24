package localexec

// 框架以工具集名为前缀给工具名加前缀（LocalExec_<tool>）。
const localExecToolSetName = "LocalExec"

const (
	submitCommandToolName    = "submit_command"
	getStatusToolName        = "get_status"
	getOutputToolName        = "get_output"
	interveneCommandToolName = "intervene_command"
	killCommandToolName      = "kill_command"
)

// Mappers 由 tools 包实现（*tools.Mappers 满足本接口）；localexec 自定义同形接口，避免跨包依赖。
type Mappers interface {
	AddMapping(Name string, In []string, Out []string)
}

// InjectMapper 注册 localexec 各工具的参数/结果提取字段。工具名为框架加前缀后的实际名。
func InjectMapper(m Mappers) {
	prefix := localExecToolSetName + "_"
	m.AddMapping(prefix+submitCommandToolName, []string{"process", "args"}, []string{"id"})
	m.AddMapping(prefix+getStatusToolName, []string{"id"}, []string{"status"})
	m.AddMapping(prefix+getOutputToolName, []string{"id"}, []string{})
	m.AddMapping(prefix+interveneCommandToolName, []string{"id", "signal"}, []string{"msg"})
	m.AddMapping(prefix+killCommandToolName, []string{"id"}, []string{"status"})
}
