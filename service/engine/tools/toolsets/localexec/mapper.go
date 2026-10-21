package localexec

// 工具名统一 Command* 家族前缀，语义自解释；工具集名为空时框架不再加
// LocalExec_ 前缀，这里登记的名字就是模型看到的名字（MCP 工具集仍带前缀）。
const (
	runToolName       = "CommandRun"
	statusToolName    = "CommandStatus"
	outputToolName    = "CommandOutput"
	interveneToolName = "CommandIntervene"
	killToolName      = "CommandKill"
)

// Mappers 由 tools 包实现（*tools.Mappers 满足本接口）；localexec 自定义同形接口，避免跨包依赖。
type Mappers interface {
	AddMapping(Name string, In []string, Out []string)
}

// InjectMapper 注册 localexec 各工具的参数/结果提取字段。
func InjectMapper(m Mappers) {
	// run：标量字段在前、output 块殿后（渲染层块独立于标量行，旧的
	// "id 放前防截断"考量已不适用）
	m.AddMapping(runToolName, []string{"process", "args"}, []string{"status", "exitCode", "id", "output"})
	m.AddMapping(statusToolName, []string{"id"}, []string{"status", "exitCode", "id", "pid", "status_all"})
	// output：工具的本体就是取输出，Out 必须含 output；落盘超阈值时
	// note/outputFile 告知 agent 完整内容的位置
	m.AddMapping(outputToolName, []string{"id"}, []string{"id", "output", "note", "outputFile"})
	m.AddMapping(interveneToolName, []string{"id", "signal"}, []string{"msg"})
	m.AddMapping(killToolName, []string{"id"}, []string{"status"})
}
