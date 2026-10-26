package engine

// Operator 与 Worker 的同源关系：两者必须挂同一套执行工具，Worker 只少掉
// 记忆 / todo / 调度这类"状态与权限"，不能少掉干活的能力。

import (
	"strings"
	"testing"

	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/tools/toolsets/localexec"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/taskrun"
)

func newAgentTopologyEngine() *Engine {
	m := config.Model{
		APIType:       "openai",
		Model:         "test-model",
		MaxTokens:     1024,
		ContextWindow: 8192,
	}
	e := &Engine{
		Agentname:   "HackerTeam",
		Config_p:    &config.Config{Model: m},
		execToolSet: localexec.LocalExec(),
	}
	taskTools := taskrun.NewTools(nil) // 只验装配，不执行
	e.taskTools = &taskTools
	return e
}

func toolNameSet(tools []tool.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		if decl := t.Declaration(); decl != nil {
			names[decl.Name] = true
		}
	}
	return names
}

func TestOperatorAndWorkerShareExecutionTools(t *testing.T) {
	e := newAgentTopologyEngine()

	operator, err := e.initOperator()
	if err != nil {
		t.Fatalf("initOperator: %v", err)
	}
	worker, err := e.initWorker()
	if err != nil {
		t.Fatalf("initWorker: %v", err)
	}

	if got := operator.Info().Name; got != string(operatorName) {
		t.Fatalf("operator name=%q, want %q", got, operatorName)
	}
	if got := worker.Info().Name; got != string(workerName) {
		t.Fatalf("worker name=%q, want %q", got, workerName)
	}

	opTools := toolNameSet(operator.Tools())
	wkTools := toolNameSet(worker.Tools())

	// 干活的能力：两者都有
	for _, name := range []string{
		"CommandRun", "CommandStatus", "CommandOutput", "CommandIntervene", "CommandKill",
	} {
		if !opTools[name] {
			t.Errorf("operator missing execution tool %q", name)
		}
		if !wkTools[name] {
			t.Errorf("worker missing execution tool %q", name)
		}
	}

	// 状态与权限：只有 Operator 有
	for _, name := range []string{"todo_write", "start_task_run", "list_task_runs", "get_task_run", "wait_task_run", "cancel_task_run"} {
		if !opTools[name] {
			t.Errorf("operator missing %q", name)
		}
		if wkTools[name] {
			t.Errorf("worker must not have %q", name)
		}
	}

	// Worker 多出来的工具 = 0：它是 Operator 去掉状态与调度后的同一份能力
	for name := range wkTools {
		if !opTools[name] {
			t.Errorf("worker has tool %q that operator does not", name)
		}
	}
}

func TestAgentPromptsResolveAllPlaceholders(t *testing.T) {
	e := newAgentTopologyEngine()
	e.EnvPrompt = "ENV"
	e.CommandExecutionPrompt = "CMD"
	e.VulnConsensusPrompt = "VULN"
	e.OutputConsensusPrompt = "OUTPUT"
	e.OutputDir = "/tmp/hackerteam-output"

	for _, path := range []string{"prompts/agents/operator.md", "prompts/agents/worker.md"} {
		got := e.assemblePrompt(path)
		if strings.Contains(got, "{{") {
			t.Errorf("%s still contains an unresolved placeholder", path)
		}
		if !strings.Contains(got, "/tmp/hackerteam-output") {
			t.Errorf("%s: {{OUTPUTDIR}} was not substituted", path)
		}
	}
}
