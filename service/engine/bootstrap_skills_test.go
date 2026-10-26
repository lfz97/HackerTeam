package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// 首次启动：Skills 目录一次性装齐嵌入模板（5 个技能 + 1 个工具清单占位）。
func TestCheckSkillsFolderInstallsAllTemplates(t *testing.T) {
	env := &BootEnv{ConfigFolderPath: t.TempDir()}
	if err := env.checkSkillsFolder(); err != nil {
		t.Fatalf("checkSkillsFolder: %v", err)
	}

	skillsDir := filepath.Join(env.ConfigFolderPath, skillsFolder)
	if env.SkillsFolderPath != skillsDir {
		t.Fatalf("SkillsFolderPath = %q, want %q", env.SkillsFolderPath, skillsDir)
	}
	for _, name := range []string{
		"hacktricks-recon-methodology",
		"hacktricks-service-scanning",
		"hacktricks-web-exploitation",
		"hacktricks-privesc-postex",
		"poc-scripting",
	} {
		if _, err := os.Stat(filepath.Join(skillsDir, name, "SKILL.md")); err != nil {
			t.Errorf("skill %s missing: %v", name, err)
		}
	}
	// pentest-tools 只是可编辑的占位模板，不是 skill
	if _, err := os.Stat(filepath.Join(skillsDir, "pentest-tools", "SKILL.md.template")); err != nil {
		t.Errorf("pentest-tools placeholder missing: %v", err)
	}
}

// 目录已存在时不做任何补写：用户删掉的技能不该在下次启动时复活。
func TestCheckSkillsFolderExistingIsNoop(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, skillsFolder)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := &BootEnv{ConfigFolderPath: dir}
	if err := env.checkSkillsFolder(); err != nil {
		t.Fatalf("checkSkillsFolder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "hacktricks-recon-methodology")); !os.IsNotExist(err) {
		t.Fatalf("existing Skills folder must not be repopulated, stat err = %v", err)
	}
}
