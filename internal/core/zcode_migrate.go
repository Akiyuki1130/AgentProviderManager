package core

import (
	"fmt"
	"strings"

	"agentprovidermanager/internal/zcodeprovider"
)

// LegacyImportPreview 是“旧供应商导入新版”的预览结果。
type LegacyImportPreview struct {
	Providers []ProviderSummary   `json:"providers"`
	Dropped   map[string][]string `json:"dropped"`
}

// LegacyImportReport 是一次导入的执行结果。Backup 为导入前的新版文件备份路径，
// 新版文件原本不存在时为空字符串。
type LegacyImportReport struct {
	Imported []string `json:"imported"`
	Skipped  []string `json:"skipped"`
	Backup   string   `json:"backup"`
}

// isManagedProviderID 报告该 providerId 是否由 ZCode 自带数据或账号体系管理
// （builtin: / account: 前缀）。这类供应商在新版个人文件之外另有来源，
// 不能当成个人供应商导入——FromLegacy 会生成 group standard-personal 的规则，
// 等于制造重复条目，所以预览与执行两处都要跳过。
func isManagedProviderID(id string) bool {
	return strings.HasPrefix(id, "builtin:") || strings.HasPrefix(id, "account:")
}

// PreviewLegacyImport 计算旧版 config.json 中有、而新版 provider_config.json
// 的 providerRules 里还没有的供应商（按 providerId 比对），并给出每个供应商
// 转换后会丢失的字段（zcodeprovider.DroppedFields）。
//
// v2Doc 允许为 nil（等同于一份空的新版配置）。两个入参都不会被修改。
func PreviewLegacyImport(legacyDoc, v2Doc Doc) (LegacyImportPreview, error) {
	preview := LegacyImportPreview{Providers: []ProviderSummary{}, Dropped: map[string][]string{}}
	rawProviders, ok := legacyDoc["provider"]
	if !ok || rawProviders == nil {
		return preview, nil
	}
	providers, ok := rawProviders.(map[string]interface{})
	if !ok {
		return preview, &ConfigParseError{Msg: "legacy 配置中的 provider 字段格式错误（应为对象）"}
	}
	cfg := zcodeprovider.NewConfigFromDoc(v2Doc)
	// BuildProviderSummary 会按 ID 排序并跳过非对象条目。
	for _, summary := range BuildProviderSummary(legacyDoc) {
		if isManagedProviderID(summary.ID) {
			continue
		}
		if cfg.ProviderRule(summary.ID) != nil {
			continue
		}
		preview.Providers = append(preview.Providers, summary)
		raw, ok := providers[summary.ID].(map[string]interface{})
		if !ok {
			continue
		}
		if dropped := zcodeprovider.DroppedFields(raw); len(dropped) > 0 {
			preview.Dropped[summary.ID] = dropped
		}
	}
	return preview, nil
}

// ApplyLegacyImport 把选中的旧供应商导入新版文档：
// 逐个用 zcodeprovider.FromLegacy 语义转换后写入 providerRule 与模型规则，
// 并把 providerId 追加到 providerOrder；已存在同 id 的记为 skipped。
//
// legacyDoc 永不被修改；v2Doc 只作为输入（内部深拷贝后改写）。
// 只要确实有导入项，就会先 BackupConfig 备份 targetPath 指向的新版文件
// （用户手工把目标指到别的 v2 文件时也必须写在那里，不能固定写 canonical 路径），
// 再用 zcodeprovider.Encode 的 canonical 字节落盘（严格校验：未知键必须写不出去）。
// targetPath 为空时回落到 ZCodeProviderConfigPath()。
func ApplyLegacyImport(v2Doc, legacyDoc Doc, selectedIDs []string, targetPath string) (Doc, LegacyImportReport, error) {
	report := LegacyImportReport{Imported: []string{}, Skipped: []string{}}
	cfg := zcodeprovider.NewConfigFromDoc(deepCopyMap(v2Doc))
	legacyProviders, _ := legacyDoc["provider"].(map[string]interface{})

	order := cfg.ProviderOrder()
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, raw := range selectedIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		// ZCode 自带/账号型供应商不属于个人配置：即使用户手工勾选也不导入。
		if isManagedProviderID(id) {
			report.Skipped = append(report.Skipped, id)
			continue
		}
		if cfg.ProviderRule(id) != nil {
			report.Skipped = append(report.Skipped, id)
			continue
		}
		provider, ok := legacyProviders[id].(map[string]interface{})
		if !ok {
			report.Skipped = append(report.Skipped, id)
			continue
		}
		converted, _, err := zcodeprovider.FromLegacy(map[string]interface{}{
			"provider": map[string]interface{}{id: provider},
		})
		if err != nil {
			return nil, report, err
		}
		for _, rule := range converted.ProviderRules() {
			cfg.UpsertProviderRule(rule)
		}
		for _, mr := range converted.ProviderModelRules() {
			cfg.UpsertProviderModelRule(mr)
		}
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
		report.Imported = append(report.Imported, id)
	}
	if len(report.Imported) == 0 {
		return cfg.Doc(), report, nil
	}
	cfg.SetProviderOrder(order)
	newDoc := cfg.Doc()

	path := strings.TrimSpace(targetPath)
	if path == "" {
		path = ZCodeProviderConfigPath()
	}
	fingerprint, _ := FileFingerprint(path)
	snapshot, err := PrepareChange(ChangeContext{Target: path, AgentID: string(AgentZCode), Operation: OpImportLegacy, Note: "旧版供应商导入新版"})
	if err != nil {
		return nil, report, err
	}
	backup := snapshot.BackupPath
	report.Backup = backup
	data, err := zcodeprovider.Encode(cfg)
	if err != nil {
		return nil, report, err
	}
	if err := WriteConfigBytes(path, data, fingerprint); err != nil {
		return nil, report, fmt.Errorf("写入配置文件时出错：%s", ShortText(err.Error(), 300))
	}
	return newDoc, report, nil
}
