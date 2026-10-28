package functionTools

type Mappers interface {
	AddMapping(Name string, In []string, Out []string)
}

func InjectMapper(m Mappers) {
	m.AddMapping(writeFileToolName, []string{"path"}, []string{"wrote"})
	m.AddMapping(readFileToolName, []string{"path"}, []string{"length"})
	m.AddMapping(editFileToolName, []string{"path"}, []string{"diff"})
	// Grep 的结果是数字键（"0"~"N" 的逐条匹配），无法逐字段登记，
	// Out 传 nil 走兜底：数字键自然排序、每条匹配一行
	m.AddMapping(grepToolName, []string{"path", "regex"}, nil)
	m.AddMapping(deleteFileToolName, []string{"path"}, []string{"deleted"})
	m.AddMapping(fileStatToolName, []string{"path"}, []string{"name", "size", "is_dir", "mode", "mod_time"})
	m.AddMapping(diffToolName, []string{"src", "dst"}, []string{"diff"})
	// ls/glob 的值是"JSON 套 JSON"（数组字符串），渲染层会展开成每元素一行
	m.AddMapping(lsToolName, []string{"path"}, []string{"files"})
	m.AddMapping(mkdirToolName, []string{}, []string{"created"})
	m.AddMapping(cpToolName, []string{}, []string{"copied"})
	m.AddMapping(mvToolName, []string{}, []string{"moved", "old_path", "new_path"})
	m.AddMapping(globToolName, []string{"regex", "root", "depth"}, []string{"matches"})
	// 框架内置工具：todo_write 的入参是整份清单（JSON 数组，逐字段提取无意义），
	// 只展示工具返回的 nudge 文案。
	m.AddMapping(todoWriteToolName, []string{}, []string{"message"})
}
