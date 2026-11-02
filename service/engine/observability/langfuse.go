package observability

// langfuse 观测服务：封装框架 telemetry/langfuse 的启停与收尾。
//
// 幂等约束：langfuse.Start 每进程只能调一次——框架对已存在的 SDK provider
// 走 RegisterSpanProcessor 追加分支，二次调用会让所有 span 双份导出。
// 本服务只在 Engine.Init（每进程一次）里构造并 Start，刻意不进 flush
// 重建链；配置改动需重启进程生效。

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	alog "trpc.group/trpc-go/trpc-agent-go/log"
	lf "trpc.group/trpc-go/trpc-agent-go/telemetry/langfuse"
)

// shutdownTimeout 约束 Stop 的尾批 flush 上限：BatchSpanProcessor 收尾是
// 同步上传，Langfuse 不可达时（OTLP client 请求超时 × retry 退避）不能挂住
// /exit。5 秒是"尽力 flush、失败就算"的语义。
const shutdownTimeout = 5 * time.Second

type langfuse struct {
	public   string
	secret   string
	host     string
	insecure bool
	clean    func(context.Context) error
}

// Start 启用 Langfuse trace 导出。非空字段转 Option 覆盖框架内部先读的
// 环境变量配置（LANGFUSE_PUBLIC_KEY/SECRET_KEY/HOST/INSECURE）——
// 即 yaml 优先、env 兜底，无需本层做合并。
func (l *langfuse) Start() error {
	// OTel 默认错误 handler 自建 log.New(os.Stderr)——项目在 bootstrap 里的
	// stdlog.SetOutput 重定向拦不住它，alt-screen 下后台导出持续失败（host
	// 配错/断网）会把 TUI 撕碎。必须导流进框架 log.Default（bootstrap 已把
	// 它重定向到 hyperbot.log），绝不让 OTel 错误碰 stderr。
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		alog.Default.Warnf("langfuse telemetry: %v", err)
	}))

	opts := []lf.Option{}
	if l.host != "" {
		opts = append(opts, lf.WithHost(l.host))
	}
	if l.public != "" {
		opts = append(opts, lf.WithPublicKey(l.public))
	}
	if l.secret != "" {
		opts = append(opts, lf.WithSecretKey(l.secret))
	}
	if l.insecure {
		opts = append(opts, lf.WithInsecure())
	}
	clean, err := lf.Start(context.Background(), opts...)
	if err != nil {
		return err
	}
	l.clean = clean
	return nil
}

// Stop flush 尾批 span 并关闭导出。注意 ctx 必须新鲜：不能传引擎的 turn ctx
// （ESC 中断后是 cancelled 态，尾批 flush 会直接失败丢数据）。
func (l *langfuse) Stop() error {
	if l.clean == nil {
		return nil // Start 失败或未调用：无事可收
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return l.clean(ctx)
}

func NewLangfuseService(public string, secret string, host string, insecure bool) *langfuse {
	return &langfuse{
		public:   public,
		secret:   secret,
		host:     host,
		insecure: insecure,
	}
}
