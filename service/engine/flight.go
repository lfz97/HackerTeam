package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	taskrunruntime "trpc.group/trpc-go/trpc-agent-go/agent/taskrun"
)

const (
	flightMaxRunning  = 3
	flightMaxFinished = 2
)

// injectFlightStatus 把本会话在途/刚结束的 task run 追加到状态栏。
//
// 只做展示，不记账：没有"已读/未读"标记。完成就显示完成，模型自己判断要不要
// 去取结果；某条什么时候从视野里消失只由条数决定。整条链路没有需要跨调用维护
// 的状态，也就没有"读了一半算不算读过"这类坑。
//
// 只显示 task run，不显示 localexec 作业：命令是 Operator 自己起的，id 就在
// CommandRun 的返回值里，CommandStatus 不带 id 时还能列全部——那是纯 pull，
// 不需要引擎替它记。需要提醒的是"你派出去、然后可能忘了的 Worker"。
func (e *Engine) injectFlightStatus(ctx context.Context, status *string) {
	if (*e).taskService == nil {
		return
	}

	// 按当前会话过滤：/new 之后旧 run 不该继续占状态栏。
	filter := taskrunruntime.ListFilter{}
	if inv, ok := agent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		filter.OwnerUserID = inv.Session.UserID
		filter.ParentSessionID = inv.Session.ID
	}
	runs, err := (*e).taskService.List(ctx, filter)
	if err != nil || len(runs) == 0 {
		return
	}

	// List 从 map 出来，顺序不稳定；排序保证同一批 run 每次渲染一致（也便于读）。
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })

	now := time.Now()
	running := make([]string, 0, len(runs))
	finished := make([]string, 0, len(runs))
	for _, run := range runs {
		agentLabel := strings.TrimSpace(run.AgentName)
		if agentLabel == "" {
			agentLabel = "worker"
		}
		if run.Status.IsTerminal() {
			finished = append(finished, fmt.Sprintf("%s %s %s", agentLabel, run.Status, flightAgo(run.FinishedAt, now)))
			continue
		}
		running = append(running, fmt.Sprintf("%s %s", agentLabel, flightElapsed(now, run.StartedAt, run.CreatedAt)))
	}

	parts := make([]string, 0, 2)
	if len(running) > 0 {
		parts = append(parts, fmt.Sprintf("running=%d (%s)", len(running), strings.Join(flightHead(running, flightMaxRunning), " | ")))
	}
	if len(finished) > 0 {
		// finished 按时间正序收集，取最近几条要从未尾倒着拿。
		recent := make([]string, 0, flightMaxFinished)
		for i := len(finished) - 1; i >= 0 && len(recent) < flightMaxFinished; i-- {
			recent = append(recent, finished[i])
		}
		parts = append(parts, fmt.Sprintf("finished=%d (%s)", len(finished), strings.Join(recent, " | ")))
	}
	if len(parts) == 0 {
		return
	}

	if *status != "" {
		*status += "\n"
	}
	*status += "[IN-FLIGHT] task-runs: " + strings.Join(parts, " ")
}

func flightHead(items []string, max int) []string {
	if len(items) <= max {
		return items
	}
	return items[:max]
}

// flightElapsed 优先用 StartedAt，排队阶段回落到 CreatedAt。
func flightElapsed(now time.Time, startedAt *time.Time, createdAt time.Time) string {
	t := createdAt
	if startedAt != nil && !startedAt.IsZero() {
		t = *startedAt
	}
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return shortDuration(d)
}

func flightAgo(t *time.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	d := now.Sub(*t)
	if d < 0 {
		d = 0
	}
	return shortDuration(d) + " ago"
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
