package functionTools

type Mappers interface {
	AddMapping(Name string, In []string, Out []string)
}

func InjectMapper(m Mappers) {
	m.AddMapping(dateToolName, []string{}, []string{"datetime"})
	m.AddMapping(writeFileToolName, []string{"path"}, []string{"wrote"})
	m.AddMapping(readFileToolName, []string{"path"}, []string{"length"})
	m.AddMapping(editFileToolName, []string{"path"}, []string{"diff"})
	m.AddMapping(searchInFileToolName, []string{"path", "regex"}, []string{})
	m.AddMapping(deleteFileToolName, []string{"path"}, []string{"deleted"})
	m.AddMapping(fileStatToolName, []string{"path"}, []string{"name", "size", "is_dir", "mode", "mod_time"})
	m.AddMapping(diffToolName, []string{"src", "dst"}, []string{"diff"})
	m.AddMapping(pwdToolName, []string{}, []string{"pwd"})
	m.AddMapping(cdToolName, []string{}, []string{"cwd"})
	m.AddMapping(lsToolName, []string{"path"}, []string{})
	m.AddMapping(mkdirToolName, []string{}, []string{"created"})
	m.AddMapping(cpToolName, []string{}, []string{"copied"})
	m.AddMapping(mvToolName, []string{}, []string{"moved", "old_path", "new_path"})
	m.AddMapping(globToolName, []string{"regex", "root", "depth"}, []string{})
}
