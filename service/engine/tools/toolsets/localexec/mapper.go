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
	// run：标量字段在前、output 块殿后（渲染层块独立于标量行）；
	// output：工具的本体就是取输出，Out 必须含 output；落盘超阈值时
	// note/outputFile 告知 agent 完整内容的位置
	m.AddMapping(prefix+runToolName, []string{"process", "args"}, []string{"status", "exitCode", "id", "output"})
	m.AddMapping(prefix+statusToolName, []string{"id"}, []string{"status", "exitCode", "id", "pid", "status_all"})
	m.AddMapping(prefix+outputToolName, []string{"id"}, []string{"id", "output", "note", "outputFile"})
	m.AddMapping(prefix+interveneToolName, []string{"id", "signal"}, []string{"msg"})
	m.AddMapping(prefix+killToolName, []string{"id"}, []string{"status"})
}
