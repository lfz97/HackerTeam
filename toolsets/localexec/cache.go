package localexec

// 创建manager缓存对象
var manager Manager = Manager{
	jobs: map[string]*Job{},
}
