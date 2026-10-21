package engine

// /resume 会话恢复：列出当前 cwd 会话库里的候选、切换并重放。
// 命令语义与注册表在前端（service/tui/commands.go），后端只暴露 ListResumeItems /
// CheckoutSession 两个能力接口；running 检查兜底并发切换（SessionId 读写无锁）。
// 切换本体只是改 SessionId，框架按新 id 自动恢复上下文；重放事件到 records 纯粹给前端画历史。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

// ListResumeItems /resume 的数据接口：候选列表 JSON（"[]"=无可恢复或读取失败，已附 notice/error）。
func (e *Engine) ListResumeItems() string {
	items, err := e.listResumeItems()
	if err != nil {
		(*e).appendTyped(RecError, "failed to read session list: "+err.Error())
		return "[]"
	}
	if len(items) == 0 {
		(*e).setNotice(NoticeWarning, "no previous sessions in this directory")
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// CheckoutSession 提交弹框选择（前端 enter 选中后调用）。
// running 时拒绝切换：SessionId 读写无锁，并发切换是 data race（对未来输入入口的兜底）；
// 拒绝/失败时弹框保持，用户可重选。
func (e *Engine) CheckoutSession(id string) error {
	if e.isRunning() {
		(*e).setNotice(NoticeWarning, "agent is running, try /resume later")
		return fmt.Errorf("conversation in progress")
	}
	if err := e.switchSession(id); err != nil {
		(*e).appendTyped(RecError, "failed to restore session: "+err.Error())
		return err // 弹框保持，用户可重选
	}
	(*e).setNotice(NoticeSuccess, "resumed session "+shortID(id))
	return nil
}

// listResumeItems 拉全量会话（带事件：主题需要首条用户输入），
// 按最近更新降序，当前会话打标。
func (e *Engine) listResumeItems() ([]ResumeItem, error) {
	sessions, err := (*e).SessionService_p.ListSessions(
		context.Background(),
		session.UserKey{AppName: (*e).Agentname, UserID: (*(*e).Config_p).User.UserID},
	)
	if err != nil {
		return nil, err
	}
	cur := (*e).SessionId
	type entry struct {
		item      ResumeItem
		updatedAt time.Time
	}
	entries := make([]entry, 0, len(sessions))
	for _, s := range sessions {
		entries = append(entries, entry{
			item: ResumeItem{
				ID:        s.ID,
				Title:     firstUserText(s),
				UpdatedAt: s.UpdatedAt.Local().Format("01-02 15:04"),
				Current:   s.ID == cur,
			},
			updatedAt: s.UpdatedAt,
		})
	}
	// 按真实时间排序而非格式化文本：跨年时 "01-02 15:04" 的字典序会乱。
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].updatedAt.After(entries[j].updatedAt)
	})
	items := make([]ResumeItem, len(entries))
	for i, en := range entries {
		items[i] = en.item
	}
	return items, nil
}

// firstUserText 会话内首条用户输入作主题；压平换行、按 rune 截断。
// 流式场景 user 事件内容在 Message，兜底 Delta。
func firstUserText(s *session.Session) string {
	for i := range s.Events {
		ev := &s.Events[i]
		if ev.Author != "user" || ev.Response == nil {
			continue
		}
		for _, ch := range ev.Response.Choices {
			text := ch.Message.Content
			if text == "" {
				text = ch.Delta.Content
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			text = strings.ReplaceAll(text, "\n", " ")
			if runes := []rune(text); len(runes) > 40 {
				text = string(runes[:40]) + "…"
			}
			return text
		}
	}
	return "(no text message)"
}

// switchSession 切到目标会话并重放事件到 records（给前端画历史）。
// RequestId 随 SessionId 一起换新：本项目 RequestId 与 SessionId 成对生成
// （见 randomStartID），经 agent.WithRequestID 盖进每条事件，是一条会话
// lineage 的追踪标签——切会话即换 lineage，沿用旧值会把新事件打成上一条
// 会话的追踪号。框架只把它当标签，无去重/幂等语义。
func (e *Engine) switchSession(id string) error {
	sess, err := (*e).SessionService_p.GetSession(
		context.Background(),
		session.Key{AppName: (*e).Agentname, UserID: (*(*e).Config_p).User.UserID, SessionID: id},
	)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session %s does not exist", id)
	}
	(*e).SessionId = id
	(*e).RequestId = uuid.New().String()
	(*e).clearRecords()
	for i := range sess.Events {
		e.replayEvent(&sess.Events[i])
	}
	return nil
}

// replayEvent 把一条持久化事件转成 records 行，形状与 emitChoice 的定稿
// 路径对齐（user 文本走 RecUser，其余走 wireMsg(RecMessage)）。工具调用/
// 结果按原顺序重放，render.go 的 toolBuf 配对逻辑无需感知来源。
func (e *Engine) replayEvent(ev *event.Event) {
	if ev.Response == nil {
		return
	}
	if ev.Author == "user" {
		for _, ch := range ev.Response.Choices {
			text := ch.Message.Content
			if text == "" {
				text = ch.Delta.Content
			}
			if strings.TrimSpace(text) != "" {
				(*e).appendTyped(RecUser, text)
				return // 用户事件只有一条文本，取到即止
			}
		}
		return
	}
	for _, ch := range ev.Response.Choices {
		m := ch.Message
		// 与 emitChoice 的 message 过滤条件一致：空定稿不入日志
		if strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) == 0 && m.Role != "tool" {
			continue
		}
		e.AppendRecordJSON(wireMsg(RecMessage, m))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
