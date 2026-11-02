package observability

// ObserverService 是可观测后端的启停接口。实现注意两条约束：
// ① Start 每进程只调一次（见 langfuse.go 头注释的幂等约束）；
// ② Stop 必须自带超时——收尾是同步网络操作，不能挂住调用方（/exit）。

type ObserverService interface {
	Start() error
	Stop() error
}

type ServiceType string

const (
	Langfuse ServiceType = "langfuse"
)

// NewObservabilityService 按配置的 service 类型构造观测服务。
// 未知类型返回 nil（调用方负责判 nil 并告警，不能 panic——这是 config
// 可触发的输入，boot goroutine 上没有 recover）。
func NewObservabilityService(service ServiceType, public string, secret string, host string, insecure bool) ObserverService {
	switch service {
	case Langfuse:
		return NewLangfuseService(public, secret, host, insecure)
	default:
		return nil
	}
}
