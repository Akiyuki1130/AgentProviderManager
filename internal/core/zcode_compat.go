package core

import (
	"fmt"
	"os"
	"strings"

	"agentprovidermanager/internal/zcodeprovider"
)

// ZCode 兼容性检查与修复。
//
// 背景：ZCode 的个人 provider_config.json 是严格校验的——任何它不认的键或取值都会让
// 整份配置被当成空，所有供应商在使用端消失。本包只允许“ZCode 也接受”的文档落盘，
// 但用户手上可能已经存在这类文件（旧版 ZCode 写过、手改过），因此提供：
//
//   - InspectZCodeCompat：只读检查，列出问题（不修改文件）；
//   - RepairZCodeCompatAt：把问题修好并写回（写入前照例建立同级备份 + 还原点）。
//
// 修复方向都是“尽量不丢数据”：builtinModelIds 合并进 personalModelIds、
// 家族 group 改成 standard-personal、非规范 logo 删除、重复项按 ZCode 的语义保留后者。

// ZCodeCompatReport 是某个目标文件的兼容性检查结果。
type ZCodeCompatReport struct {
	Path   string                             `json:"path"`
	Format string                             `json:"format"`
	Issues []zcodeprovider.CompatibilityIssue `json:"issues"`
}

// InspectZCodeCompat 检查目标文件里 ZCode 3.14 会拒绝的问题。
// 非 v2 文件、文件不存在或结构本身不可读时返回空报告（不视为错误）。
func InspectZCodeCompat(path string) ZCodeCompatReport {
	report := ZCodeCompatReport{Path: path, Format: ZCodeFormatName(DetectZCodeFormatAt(path))}
	if !IsZCodeV2Path(path) {
		return report
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return report
	}
	_, issues, err := zcodeprovider.DecodeInspect(data)
	if err != nil {
		return report
	}
	report.Issues = issues
	return report
}

// RepairZCodeCompatAt 修复目标文件里的兼容性问题并写回。
//
// 返回修复动作清单、修复后的残留问题与本次写入建立的还原点。
// 没有任何问题时不会写盘（Fixes 为空、Snapshot 为零值）。
func RepairZCodeCompatAt(path, agentID string) (ZCodeCompatReport, []string, ChangeSnapshot, error) {
	var snapshot ChangeSnapshot
	report := ZCodeCompatReport{Path: path, Format: ZCodeFormatName(DetectZCodeFormatAt(path))}
	if strings.TrimSpace(path) == "" {
		return report, nil, snapshot, fmt.Errorf("请先选择目标配置文件")
	}
	if !IsZCodeV2Path(path) {
		return report, nil, snapshot, fmt.Errorf("当前目标不是 ZCode 新版配置（provider_config.json），无需修复")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return report, nil, snapshot, fmt.Errorf("读取配置文件失败：%v", err)
	}
	cfg, issues, err := zcodeprovider.DecodeInspect(data)
	if err != nil {
		return report, nil, snapshot, &ConfigParseError{Msg: err.Error()}
	}
	report.Issues = issues
	if len(issues) == 0 {
		return report, nil, snapshot, nil
	}
	result, err := zcodeprovider.RepairConfig(cfg)
	if err != nil {
		return report, nil, snapshot, &ConfigParseError{Msg: err.Error()}
	}
	if !result.Changed {
		return report, nil, snapshot, fmt.Errorf("检测到 %d 处兼容性问题，但没有可自动修复的项，请手动编辑相关供应商", len(issues))
	}
	encoded, err := zcodeprovider.Encode(cfg)
	if err != nil {
		return report, nil, snapshot, &ConfigParseError{Msg: err.Error()}
	}
	fingerprint, _ := FileFingerprint(path)
	snapshot, err = PrepareChange(ChangeContext{
		Target:    path,
		AgentID:   agentID,
		Operation: OpManual,
		Note:      "修复 ZCode 兼容性问题",
	})
	if err != nil {
		return report, nil, snapshot, err
	}
	if err := WriteConfigBytes(path, encoded, fingerprint); err != nil {
		return report, nil, snapshot, fmt.Errorf("写入配置文件失败：%v", err)
	}
	report.Issues = result.Remaining
	return report, result.Fixes, snapshot, nil
}
