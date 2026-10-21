// ctxbench — 评测上下文管理（摘要阈值 / compaction 预算）对"会话末态信息保真度"
// 与 token 成本的影响，选出最优配置。各实验组共享一份内嵌唯一"事实锚点"的确定性
// 长转录；摘要组走生产路径，保真度按锚点是否存活于末态有效上下文做确定性匹配。
//
// 运行示例：go run ./cmd/ctxbench -config ./.HackerTeam/HackerTeam.yaml [-smoke]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/models"
	hypersess "HackerTeam/service/engine/session"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/tiktoken"
	fwsess "trpc.group/trpc-go/trpc-agent-go/session"
)

// ── 转录生成 ──

// 事实锚点：Value 必须在末态上下文中可被唯一匹配。
type Fact struct {
	Kind    string // narr | tool-head | tool-mid | tool-tail
	Value   string
	Round   int
	InUser  bool // narr：嵌在用户消息(true)还是助手消息(false)
	EventID string
}

type Transcript struct {
	Events    []event.Event
	Facts     []Fact
	Rounds    int
	EstTokens int // tiktoken(cl100k) 估计——与 checker 同一计数空间
	// RoundOfEvent[eventIdx] = round 序号；LastRoundEventStart 等按需计算。
	RoundOfEvent []int
}

var topics = []string{
	"网络分段设计", "日志采集管道", "权限矩阵", "变更管理流程", "容量规划",
	"灾备演练", "告警治理", "资产清点", "密钥轮换", "补丁窗口",
	"流量清洗", "终端准入", "数据分级", "巡检基线", "配置漂移",
}
var units = []string{
	"kafka 分区", "ES 索引", "core 交换机", "堡垒机", "WAF 规则",
	"NAT 池", "VPN 隧道", "镜像仓库", "告警规则", "采集探针",
}

func filler(rnd *rand.Rand, targetChars int) string {
	var b strings.Builder
	n := 0
	for i := 0; n < targetChars; i++ {
		t := topics[rnd.Intn(len(topics))]
		u := units[rnd.Intn(len(units))]
		fmt.Fprintf(&b, "【%s·%s】本批次覆盖 %s，基线编号 BL-%04d，抽样 %d 个实例，异常 %d 项已转工单，"+
			"责任组 oncall-%d。复查窗口 %d 天，关联变更单 CHG-%06d，处置时限 SLA %d 小时。\n",
			t, u, units[rnd.Intn(len(units))], rnd.Intn(10000), 3+rnd.Intn(40),
			rnd.Intn(6), 1+rnd.Intn(9), 1+rnd.Intn(28), rnd.Intn(1000000), 1+rnd.Intn(48))
		n += 128
	}
	return b.String()
}

func factCode(rnd *rand.Rand, prefix string) string {
	const hex = "0123456789ABCDEF"
	b := make([]byte, 4)
	for i := range b {
		b[i] = hex[rnd.Intn(16)]
	}
	return fmt.Sprintf("%s-%s", prefix, string(b))
}

// makeTranscript 生成确定性转录，直到 tiktoken 估计达到 targetEstTokens。
func makeTranscript(targetEstTokens int, counter model.TokenCounter) *Transcript {
	rnd := rand.New(rand.NewSource(20261019))
	tr := &Transcript{}
	base := time.Date(2026, 10, 19, 9, 0, 0, 0, time.Local)
	evIdx := 0
	addEvent := func(e event.Event, round int) {
		e.ID = fmt.Sprintf("ev%05d", evIdx)
		e.RequestID = fmt.Sprintf("req%04d", round)
		tr.Events = append(tr.Events, e)
		tr.RoundOfEvent = append(tr.RoundOfEvent, round)
		evIdx++
	}

	round := 0
	for tr.EstTokens < targetEstTokens {
		// 每轮：用户消息(~700t) + 助手文本(~700t) + 工具调用 + 工具结果(~2100t)；
		// 叙事事实每 3 轮 1 个（用户/助手交替），工具事实每 4 轮 1 个。
		wantNarr := round%3 == 0
		wantTool := round%4 == 0
		pos := []string{"tool-head", "tool-mid", "tool-tail"}[(round/4)%3]

		userText := filler(rnd, 900)
		if wantNarr {
			v := factCode(rnd, "GD")
			tr.Facts = append(tr.Facts, Fact{Kind: "narr", Value: v, Round: round, InUser: true, EventID: fmt.Sprintf("ev%05d", evIdx)})
			userText += fmt.Sprintf("顺带确认：巡检工单编号 %s 已同步至值守看板，备注归属组为安全运营二组。\n", v)
		}
		addEvent(event.Event{
			Timestamp: base.Add(time.Duration(round)*time.Minute + 0*time.Second),
			Author:    "user",
			Response:  &model.Response{Choices: []model.Choice{{Message: model.Message{Role: model.RoleUser, Content: userText}}}},
		}, round)

		asstText := filler(rnd, 900)
		if round%3 == 1 {
			v := factCode(rnd, "VX")
			tr.Facts = append(tr.Facts, Fact{Kind: "narr", Value: v, Round: round, InUser: false, EventID: fmt.Sprintf("ev%05d", evIdx)})
			asstText += fmt.Sprintf("已登记：本次发布版本号 %s，灰度范围 10%%，回滚预案见知识库 KB-2026-%d。\n", v, 1000+round)
		}
		addEvent(event.Event{
			Timestamp: base.Add(time.Duration(round)*time.Minute + 10*time.Second),
			Author:    "assistant",
			Response:  &model.Response{Choices: []model.Choice{{Message: model.Message{Role: model.RoleAssistant, Content: asstText}}}},
		}, round)

		callID := fmt.Sprintf("call_%05d", round)
		addEvent(event.Event{
			Timestamp: base.Add(time.Duration(round)*time.Minute + 20*time.Second),
			Author:    "assistant",
			Response: &model.Response{Choices: []model.Choice{{Message: model.Message{
				Role: model.RoleAssistant,
				ToolCalls: []model.ToolCall{{
					ID:   callID,
					Type: "function",
					Function: model.FunctionDefinitionParam{
						Name:      "ReadFile",
						Arguments: []byte(fmt.Sprintf(`{"path":"/data/report/round%03d.txt"}`, round)),
					},
				}},
			}}},
			}}, round)

		// 工具结果 ~2100t，事实嵌在首/中/尾
		bodyChars := 2800
		body := filler(rnd, bodyChars)
		if wantTool {
			v := factCode(rnd, "TF")
			line := fmt.Sprintf("清单登记号 %s（登记人：值班工程师，登记时间：当日 14:22，复核周期 7 天）。\n", v)
			switch pos {
			case "tool-head":
				body = line + body
			case "tool-mid":
				mid := len(body) / 2
				body = body[:mid] + "\n" + line + body[mid:]
			case "tool-tail":
				body = body + "\n" + line
			}
			tr.Facts = append(tr.Facts, Fact{Kind: pos, Value: v, Round: round, EventID: fmt.Sprintf("ev%05d", evIdx)})
		}
		addEvent(event.Event{
			Timestamp: base.Add(time.Duration(round)*time.Minute + 30*time.Second),
			Author:    "assistant",
			Response: &model.Response{Choices: []model.Choice{{Message: model.Message{
				Role:     model.RoleTool,
				ToolID:   callID,
				ToolName: "ReadFile",
				Content:  body,
			}}}},
		}, round)

		// 周期性 tiktoken 计数（每 5 轮），达标即停
		if round%5 == 4 {
			if n, err := countTokens(context.Background(), counter, tr.Events); err == nil {
				tr.EstTokens = n
			}
		}
		round++
	}
	if n, err := countTokens(context.Background(), counter, tr.Events); err == nil {
		tr.EstTokens = n
	}
	tr.Rounds = round
	return tr
}

func countTokens(ctx context.Context, counter model.TokenCounter, events []event.Event) (int, error) {
	total := 0
	for _, e := range events {
		if e.Response == nil {
			continue
		}
		for _, ch := range e.Response.Choices {
			n, err := counter.CountTokens(ctx, ch.Message)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	return total, nil
}

// ── 计数装饰器：包装真实模型客户端，统计每次摘要调用的 usage ──

type callLog struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	InputChars       int     `json:"input_chars"`
	DurationSec      float64 `json:"duration_sec"`
	Err              string  `json:"err,omitempty"`
}

type countingModel struct {
	inner model.Model
	mu    sync.Mutex
	calls []callLog
}

func (c *countingModel) Info() model.Info { return c.inner.Info() }

func (c *countingModel) GenerateContent(ctx context.Context, req *model.Request) (<-chan *model.Response, error) {
	inputChars := 0
	for _, m := range req.Messages {
		inputChars += len(m.Content)
	}
	start := time.Now()
	ch, err := c.inner.GenerateContent(ctx, req)
	if err != nil {
		c.record(callLog{Err: err.Error(), DurationSec: time.Since(start).Seconds(), InputChars: inputChars})
		return nil, err
	}
	out := make(chan *model.Response, 16)
	go func() {
		defer close(out)
		var cl callLog
		cl.InputChars = inputChars
		defer func() { cl.DurationSec = time.Since(start).Seconds(); c.record(cl) }()
		for r := range ch {
			if r != nil && r.Usage != nil {
				cl.PromptTokens += r.Usage.PromptTokens
				cl.CompletionTokens += r.Usage.CompletionTokens
			}
			out <- r
		}
	}()
	return out, nil
}

func (c *countingModel) record(cl callLog) {
	c.mu.Lock()
	c.calls = append(c.calls, cl)
	c.mu.Unlock()
}

func (c *countingModel) stats() (calls []callLog, prompt, completion int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cl := range c.calls {
		prompt += cl.PromptTokens
		completion += cl.CompletionTokens
	}
	return append([]callLog(nil), c.calls...), prompt, completion
}

// ── 实验组 ──

type ArmResult struct {
	Arm            string  `json:"arm"`
	FactSurvival   float64 `json:"fact_survival"`
	NarrSurvival   float64 `json:"narr_survival"`
	ToolSurvival   float64 `json:"tool_survival"`
	ToolHeadSurv   float64 `json:"tool_head_survival"`
	ToolMidSurv    float64 `json:"tool_mid_survival"`
	ToolTailSurv   float64 `json:"tool_tail_survival"`
	SummaryCalls   int     `json:"summary_calls"`
	PromptTokens   int     `json:"prompt_tokens"`
	CompletionTok  int     `json:"completion_tokens"`
	FinalCtxTokens int     `json:"final_ctx_tokens"`
	Notes          string  `json:"notes,omitempty"`
}

func scoreFacts(facts []Fact, effCtx string) ArmResult {
	r := ArmResult{}
	nN, nH, nM, nTl := 0, 0, 0, 0
	for _, f := range facts {
		ok := strings.Contains(effCtx, f.Value)
		switch {
		case f.Kind == "narr":
			nN++
			if ok {
				r.NarrSurvival++
			}
		case f.Kind == "tool-head":
			nH++
			if ok {
				r.ToolHeadSurv++
				r.ToolSurvival++
			}
		case f.Kind == "tool-mid":
			nM++
			if ok {
				r.ToolMidSurv++
				r.ToolSurvival++
			}
		case f.Kind == "tool-tail":
			nTl++
			if ok {
				r.ToolTailSurv++
				r.ToolSurvival++
			}
		}
	}
	if nN > 0 {
		r.NarrSurvival /= float64(nN)
	}
	if nH+nM+nTl > 0 {
		r.ToolSurvival /= float64(nH + nM + nTl)
	}
	if nH > 0 {
		r.ToolHeadSurv /= float64(nH)
	}
	if nM > 0 {
		r.ToolMidSurv /= float64(nM)
	}
	if nTl > 0 {
		r.ToolTailSurv /= float64(nTl)
	}
	if nN+nH+nM+nTl > 0 {
		r.FactSurvival = (r.NarrSurvival*float64(nN) + r.ToolSurvival*float64(nH+nM+nTl)) / float64(nN+nH+nM+nTl)
	}
	return r
}

// effectiveContext 组装末态有效上下文的文本（摘要 + 边界后尾部事件内容）。
func effectiveContext(summaryText string, events []event.Event, counter model.TokenCounter) (string, int) {
	var b strings.Builder
	b.WriteString(summaryText)
	tokens := 0
	if summaryText != "" {
		if n, err := counter.CountTokens(context.Background(), model.Message{Content: summaryText}); err == nil {
			tokens += n
		}
	}
	for _, e := range events {
		if e.Response == nil {
			continue
		}
		for _, ch := range e.Response.Choices {
			m := ch.Message
			if m.ToolID != "" {
				b.WriteString("\n" + m.Content)
			} else if strings.TrimSpace(m.Content) != "" {
				b.WriteString("\n" + m.Content)
			}
			if n, err := counter.CountTokens(context.Background(), m); err == nil {
				tokens += n
			}
		}
	}
	return b.String(), tokens
}

// runSummaryArm 摘要阈值组：逐轮推进，ShouldSummarize → Summarize（生产路径）。
func runSummaryArm(cfg config.Model, tr *Transcript, counter model.TokenCounter, thresholdPct float64) ArmResult {
	r := ArmResult{Arm: fmt.Sprintf("summary@%.2f", thresholdPct)}
	summr := hypersess.NewSummarizerWithThreshold(cfg, nil, thresholdPct)
	cm := &countingModel{inner: models.Openai(cfg)}
	summr.SetModel(cm)

	sess := &fwsess.Session{
		ID:      "ctxbench-summary",
		AppName: "ctxbench",
		UserID:  "bench",
	}
	ctx := context.Background()

	// 按轮推进（每轮 4 个事件；生产按事件粒度检查，轮粒度是保守近似）
	type summLog struct {
		atEvent   int
		estTokens int
		text      string
	}
	var summaries []summLog
	checkFrom := 0
	for i := 0; i < len(tr.Events); i++ {
		sess.Events = tr.Events[:i+1]
		// 只在轮末（每轮最后一个事件）检查
		isRoundEnd := i+1 == len(tr.Events) || tr.RoundOfEvent[i+1] != tr.RoundOfEvent[i]
		if !isRoundEnd {
			continue
		}
		if summr.ShouldSummarize(sess) {
			fmt.Printf("    [summary@%.2f] 触发 @事件%d/%d (轮%d)\n", thresholdPct, i+1, len(tr.Events), tr.RoundOfEvent[i])
			before := len(sess.Events)
			start := time.Now()
			text, err := summr.Summarize(ctx, sess)
			if err != nil {
				fmt.Printf("    [summary@%.2f] Summarize 失败: %v\n", thresholdPct, err)
				r.Notes += fmt.Sprintf("summarize-error@ev%d: %v; ", i+1, err)
			} else {
				_ = before
				summaries = append(summaries, summLog{atEvent: i + 1, text: text})
				fmt.Printf("    [summary@%.2f] 完成 (%.0fs, %d 字符)\n", thresholdPct, time.Since(start).Seconds(), len([]rune(text)))
				checkFrom = i + 1
			}
		}
	}
	_ = checkFrom

	// 末态有效上下文 = 最后一次摘要 + 边界之后的尾部事件
	lastSummary := ""
	if len(summaries) > 0 {
		lastSummary = summaries[len(summaries)-1].text
	}
	boundary := boundaryEventIndex(sess, tr)
	tail := tr.Events[boundary:]
	effCtx, tokens := effectiveContext(lastSummary, tail, counter)
	sr := scoreFacts(tr.Facts, effCtx)
	r.FactSurvival, r.NarrSurvival = sr.FactSurvival, sr.NarrSurvival
	r.ToolSurvival, r.ToolHeadSurv, r.ToolMidSurv, r.ToolTailSurv = sr.ToolSurvival, sr.ToolHeadSurv, sr.ToolMidSurv, sr.ToolTailSurv
	r.SummaryCalls = len(summaries)
	r.FinalCtxTokens = tokens
	r.Notes += fmt.Sprintf("boundary=ev%d, tail=%d events, facts_in_tail=%d; ", boundary, len(tail), countFactsIn(tr.Facts, boundary))
	calls, pt, ct := cm.stats()
	r.PromptTokens, r.CompletionTok = pt, ct
	_ = calls
	return r
}

// boundaryEventIndex 通过会话状态里的边界事件 ID 找到尾部起点。
func boundaryEventIndex(sess *fwsess.Session, tr *Transcript) int {
	raw, ok := sess.GetState(fwsess.SummaryLastIncludedEventIDStateKey)
	if !ok || len(raw) == 0 {
		return 0
	}
	id := string(raw)
	for i, e := range tr.Events {
		if e.ID == id {
			return i + 1
		}
	}
	// 未找到（可能只剩时间戳边界）→ 保守返回 0（全部事件保留）
	return 0
}

func countFactsIn(facts []Fact, fromEventIdx int) int {
	// 事实按 EventID 前缀 ev%05d 编码
	n := 0
	for _, f := range facts {
		var idx int
		if _, err := fmt.Sscanf(f.EventID, "ev%05d", &idx); err == nil && idx >= fromEventIdx {
			n++
		}
	}
	return n
}

// runCompactionArm compaction 预算组（无 API 成本）：模拟 v1.11.1 Pass1+Pass2 语义。
// Pass1：非最近请求（KeepRecentRequests=1）中 token 超过 budget 的工具结果 → placeholder。
// Pass2：任何超过 oversized(8192) 的工具结果 → 首尾保留截断。
func runCompactionArm(tr *Transcript, counter model.TokenCounter, budget int) ArmResult {
	arm := fmt.Sprintf("compact@%d", budget)
	if budget == 0 {
		arm = "compact@off"
	}
	r := ArmResult{Arm: arm}
	var b strings.Builder
	ctx := context.Background()
	lastRound := tr.Rounds - 1
	oversized := 8192
	for i, e := range tr.Events {
		for _, ch := range e.Response.Choices {
			m := ch.Message
			if m.ToolID == "" {
				b.WriteString("\n" + m.Content)
				continue
			}
			tokens, _ := counter.CountTokens(ctx, m)
			content := m.Content
			inLastRound := tr.RoundOfEvent[i] >= lastRound
			// Pass 2（与生产一致，作用于所有请求含当前）
			if tokens > oversized {
				runes := []rune(content)
				keep := oversized / 2
				if keep > len(runes)/2 {
					keep = len(runes) / 2
				}
				content = string(runes[:keep]) + "\n[...output truncated...]\n" + string(runes[len(runes)-keep:])
				tokens, _ = counter.CountTokens(ctx, model.Message{Content: content})
			}
			// Pass 1：历史（非最近请求）且超过预算 → placeholder
			if budget > 0 && !inLastRound && tokens > budget {
				content = "[Tool result omitted from raw history; use session_load to recover]"
			}
			b.WriteString("\n" + content)
		}
	}
	effCtx := b.String()
	sr := scoreFacts(tr.Facts, effCtx)
	r.FactSurvival, r.NarrSurvival = sr.FactSurvival, sr.NarrSurvival
	r.ToolSurvival, r.ToolHeadSurv, r.ToolMidSurv, r.ToolTailSurv = sr.ToolSurvival, sr.ToolHeadSurv, sr.ToolMidSurv, sr.ToolTailSurv
	if n, err := counter.CountTokens(ctx, model.Message{Content: effCtx}); err == nil {
		r.FinalCtxTokens = n
	}
	return r
}

// runBaselineArm 无干预基线：末态上下文 = 全量事件（信息无损上限参考）。
func runBaselineArm(tr *Transcript, counter model.TokenCounter) ArmResult {
	r := ArmResult{Arm: "raw-baseline"}
	effCtx, tokens := effectiveContext("", tr.Events, counter)
	sr := scoreFacts(tr.Facts, effCtx)
	r.FactSurvival, r.NarrSurvival = sr.FactSurvival, sr.NarrSurvival
	r.ToolSurvival, r.ToolHeadSurv, r.ToolMidSurv, r.ToolTailSurv = sr.ToolSurvival, sr.ToolHeadSurv, sr.ToolMidSurv, sr.ToolTailSurv
	r.FinalCtxTokens = tokens
	return r
}

func pct(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }

func main() {
	configPath := flag.String("config", "./.HackerTeam/HackerTeam.yaml", "HackerTeam yaml 配置路径")
	modelName := flag.String("model", "", "覆盖模型名（默认取配置）")
	target := flag.Int("target", 450000, "转录目标 token 数（tiktoken 估计空间）")
	thresholds := flag.String("thresholds", "0.15,0.25,0.40", "摘要阈值实验组（逗号分隔，相对 contextwindow）")
	budgets := flag.String("budgets", "1024,4096,16384,0", "compaction 预算实验组（0=关闭）")
	combos := flag.String("combos", "0.25:4096,0.25:16384", "组合实验组 threshold:budget（空串跳过）")
	outDir := flag.String("out", "/tmp/opencode/ctxbench", "结果输出目录")
	smoke := flag.Bool("smoke", false, "冒烟模式：小转录、单阈值，验证机制")
	flag.Parse()

	if *smoke {
		*target = 12000
		*thresholds = "0.004"
		*budgets = "1024"
		*combos = ""
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	if *modelName != "" {
		cfg.Model.Model = *modelName
	}
	fmt.Printf("模型 %s @ %s, contextwindow=%d, maxtokens=%d\n", cfg.Model.Model, cfg.Model.BaseURL, cfg.Model.ContextWindow, cfg.Model.MaxTokens)

	counter, err := tiktoken.New(cfg.Model.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiktoken 初始化失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("生成转录（目标 ~%d est tokens）...\n", *target)
	tr := makeTranscript(*target, counter)
	nNarr, nTool := 0, 0
	for _, f := range tr.Facts {
		if f.Kind == "narr" {
			nNarr++
		} else {
			nTool++
		}
	}
	fmt.Printf("转录就绪：%d 轮 / %d 事件 / %d est tokens；事实锚点：叙事 %d、工具 %d（首/中/尾）\n",
		tr.Rounds, len(tr.Events), tr.EstTokens, nNarr, nTool)

	os.MkdirAll(*outDir, 0o755)
	var results []ArmResult

	// 每组完成立即落盘（JSONL 逐行追加）：进程被杀时已完成组的数据不丢。
	checkpoint := func(r ArmResult) {
		results = append(results, r)
		line, _ := json.Marshal(r)
		path := filepath.Join(*outDir, "results.jsonl")
		if fp, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fp.Write(append(line, '\n'))
			fp.Close()
		}
		printTable(results)
	}

	// 基线
	checkpoint(runBaselineArm(tr, counter))

	// compaction 组（本地，无成本）
	for _, b := range parseIntList(*budgets) {
		fmt.Printf("== compaction budget %d ==\n", b)
		checkpoint(runCompactionArm(tr, counter, b))
	}

	// 摘要阈值组（真实 API）
	for _, t := range parseFloatList(*thresholds) {
		fmt.Printf("== summary threshold %.2f ==\n", t)
		checkpoint(runSummaryArm(cfg.Model, tr, counter, t))
	}

	// 组合组（真实 API）：摘要 + 尾部 compaction
	for _, c := range parseComboList(*combos) {
		fmt.Printf("== combo %.2f + compact %d ==\n", c.t, c.b)
		checkpoint(runComboWithCompaction(cfg.Model, tr, counter, c.t, c.b))
	}

	// 结尾汇总（兼容旧格式；增量数据在 results.jsonl）
	f, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(*outDir, "results.json"), f, 0o644)
	fmt.Printf("结果已写入 %s\n", filepath.Join(*outDir, "results.json"))
}

// runComboWithCompaction 与 runSummaryArm 相同驱动，但末态尾部再叠加 compaction 语义。
func runComboWithCompaction(cfg config.Model, tr *Transcript, counter model.TokenCounter, thresholdPct float64, budget int) ArmResult {
	r := ArmResult{Arm: fmt.Sprintf("combo %.2f+compact@%d", thresholdPct, budget)}
	summr := hypersess.NewSummarizerWithThreshold(cfg, nil, thresholdPct)
	cm := &countingModel{inner: models.Openai(cfg)}
	summr.SetModel(cm)
	sess := &fwsess.Session{ID: "ctxbench-combo", AppName: "ctxbench", UserID: "bench"}
	ctx := context.Background()
	var lastSummary string
	nSummaries := 0
	for i := 0; i < len(tr.Events); i++ {
		sess.Events = tr.Events[:i+1]
		isRoundEnd := i+1 == len(tr.Events) || tr.RoundOfEvent[i+1] != tr.RoundOfEvent[i]
		if !isRoundEnd {
			continue
		}
		if summr.ShouldSummarize(sess) {
			fmt.Printf("    [combo %.2f+%d] 触发 @事件%d/%d\n", thresholdPct, budget, i+1, len(tr.Events))
			start := time.Now()
			text, err := summr.Summarize(ctx, sess)
			if err != nil {
				fmt.Printf("    [combo %.2f+%d] 失败: %v\n", thresholdPct, budget, err)
				r.Notes += fmt.Sprintf("summarize-error@ev%d: %v; ", i+1, err)
				continue
			}
			fmt.Printf("    [combo %.2f+%d] 完成 (%.0fs, %d 字符)\n", thresholdPct, budget, time.Since(start).Seconds(), len([]rune(text)))
			lastSummary = text
			nSummaries++
		}
	}
	boundary := boundaryEventIndex(sess, tr)
	tail := tr.Events[boundary:]
	// 叠加 compaction：尾部非最近轮且 >budget 的工具结果 → placeholder（模拟请求侧 Pass1）
	var b strings.Builder
	b.WriteString(lastSummary)
	tokens := 0
	lastRound := tr.Rounds - 1
	for i, e := range tail {
		for _, ch := range e.Response.Choices {
			m := ch.Message
			content := m.Content
			if m.ToolID != "" {
				n, _ := counter.CountTokens(ctx, m)
				if budget > 0 && tr.RoundOfEvent[boundary+i] < lastRound && n > budget {
					content = "[Tool result omitted from raw history; use session_load to recover]"
				}
			}
			b.WriteString("\n" + content)
			if n, err := counter.CountTokens(ctx, model.Message{Content: content}); err == nil {
				tokens += n
			}
		}
	}
	effCtx := b.String()
	sr := scoreFacts(tr.Facts, effCtx)
	r.FactSurvival, r.NarrSurvival = sr.FactSurvival, sr.NarrSurvival
	r.ToolSurvival, r.ToolHeadSurv, r.ToolMidSurv, r.ToolTailSurv = sr.ToolSurvival, sr.ToolHeadSurv, sr.ToolMidSurv, sr.ToolTailSurv
	r.SummaryCalls = nSummaries
	r.FinalCtxTokens = tokens
	r.Notes += fmt.Sprintf("boundary=ev%d; ", boundary)
	_, pt, ct := cm.stats()
	r.PromptTokens, r.CompletionTok = pt, ct
	return r
}

func parseIntList(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		var v int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &v); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func parseFloatList(s string) []float64 {
	var out []float64
	for _, p := range strings.Split(s, ",") {
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%f", &v); err == nil {
			out = append(out, v)
		}
	}
	return out
}

type comboCfg struct {
	t float64
	b int
}

func parseComboList(s string) []comboCfg {
	var out []comboCfg
	for _, p := range strings.Split(s, ",") {
		parts := strings.Split(strings.TrimSpace(p), ":")
		if len(parts) != 2 {
			continue
		}
		var t float64
		var b int
		if _, err := fmt.Sscanf(parts[0], "%f", &t); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &b); err != nil {
			continue
		}
		out = append(out, comboCfg{t, b})
	}
	return out
}

func printTable(results []ArmResult) {
	fmt.Println()
	fmt.Printf("%-22s %6s %6s %6s %6s %6s %6s %5s %10s %8s\n",
		"arm", "总", "叙事", "工具", "工首", "工中", "工尾", "调用", "输入tok", "末态tok")
	for _, r := range results {
		fmt.Printf("%-22s %6s %6s %6s %6s %6s %6s %5d %10d %8d\n",
			r.Arm, pct(r.FactSurvival), pct(r.NarrSurvival), pct(r.ToolSurvival),
			pct(r.ToolHeadSurv), pct(r.ToolMidSurv), pct(r.ToolTailSurv),
			r.SummaryCalls, r.PromptTokens, r.FinalCtxTokens)
	}
}
