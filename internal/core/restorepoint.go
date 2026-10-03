package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"agentprovidermanager/internal/zcodeprovider"
)

// 还原点（restore point）在每次改动 Agent 配置文件之前，把目标文件的**原始字节**
// 以及“这份文件是什么格式、在哪、因为哪次操作被备份”等元数据一起存下来。
//
// 与同级的 <文件>.bak_<时间戳> 的关系：
//   - .bak_ 备份是轻量副本，只保留最近 MaxBackups 份，好处是不依赖本程序也能手工恢复；
//   - 还原点是完整历史（含元数据），可以按时间点任选一个回滚，存在 AppData 下，
//     不会往 ~/.zcode 之类的目录里堆文件。
//
// 两者都由 PrepareChange 一次创建：任何写入配置文件的代码都必须先调用它。
//
// 隐私约束：content 是配置文件原文，可能包含 API Key。它只写在本机
// %LOCALAPPDATA%\AgentProviderManager\restorepoints 下，权限 0600，
// meta.json 不含任何密钥或配置内容，也绝不写日志、不上传。

const (
	// RestorePointsDirName 是还原点存储目录名（位于 AppDataDir 下）。
	RestorePointsDirName = "restorepoints"
	// RestorePointMetaName 是每个还原点的元数据文件名。
	RestorePointMetaName = "meta.json"
	// RestorePointContentName 是每个还原点的配置原文文件名。
	RestorePointContentName = "content"

	// MaxRestorePoints 是单个目标路径保留的还原点数量上限。
	MaxRestorePoints = 50
	// MaxRestorePointBytes 是全部还原点内容的总字节上限（超出后按最旧优先清理）。
	MaxRestorePointBytes = 64 * 1024 * 1024
	// orphanPointAge 是“只有目录、没有 meta.json”的残留还原点的清理门槛。
	orphanPointAge = time.Hour
)

// 还原点记录的 operation 取值。
const (
	OpSaveProvider   = "save_provider"
	OpDeleteProvider = "delete_provider"
	OpDeleteModel    = "delete_model"
	OpImportLegacy   = "import_legacy"
	OpImportProvider = "import_provider"
	OpMigrate        = "migrate"
	OpRestore        = "restore"
	OpManual         = "manual"
)

// 还原点记录的 format 取值。
const (
	FormatV2       = "v2"
	FormatLegacy   = "legacy"
	FormatOpencode = "opencode"
	FormatDeepSeek = "deepseek"
	FormatUnknown  = "unknown"
)

// RestorePoint 描述一次变更前的配置快照。
type RestorePoint struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	AgentID     string `json:"agent_id"`
	AgentLabel  string `json:"agent_label"`
	TargetPath  string `json:"target_path"`
	Format      string `json:"format"`
	Existed     bool   `json:"existed"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	Fingerprint string `json:"fingerprint"`
	AppVersion  string `json:"app_version"`
	Operation   string `json:"operation"`
	ProviderID  string `json:"provider_id,omitempty"`
	ModelID     string `json:"model_id,omitempty"`
	Note        string `json:"note,omitempty"`
}

// ChangeContext 描述“即将发生的变更”，用于生成还原点。
// Format / AgentLabel 留空时由 PrepareChange 自动推断。
type ChangeContext struct {
	Target     string
	AgentID    string
	AgentLabel string
	Format     string
	Operation  string
	ProviderID string
	ModelID    string
	Note       string
}

// ChangeSnapshot 是一次 PrepareChange 的结果。
type ChangeSnapshot struct {
	// BackupPath 是同级 .bak_<时间戳> 备份路径，目标文件不存在时为空。
	BackupPath string `json:"backup"`
	// RestorePoint 是本次建立的还原点（始终非空，除非返回错误）。
	RestorePoint *RestorePoint `json:"restore_point"`
}

// RestorePointsDir 返回还原点存储根目录。
func RestorePointsDir() string {
	return filepath.Join(AppDataDir(), RestorePointsDirName)
}

// ConfigFormatAt 推断某个目标路径上的配置格式。
//
// 文件存在时按内容判定；文件不存在时按已知约定推断：ZCode 新版文件名
// provider_config.json 视为 v2，其余 ZCode 路径视为 legacy。
func ConfigFormatAt(path, agentID string) string {
	switch AgentID(strings.ToLower(strings.TrimSpace(agentID))) {
	case AgentOpenCode:
		return FormatOpencode
	case AgentDeepSeek:
		return FormatDeepSeek
	}
	if _, err := os.Stat(path); err == nil {
		switch DetectZCodeFormatAt(path) {
		case zcodeprovider.FormatV2:
			return FormatV2
		case zcodeprovider.FormatLegacy:
			return FormatLegacy
		default:
			return FormatUnknown
		}
	}
	if strings.EqualFold(filepath.Base(path), filepath.Base(ZCodeProviderConfigPath())) {
		return FormatV2
	}
	return FormatLegacy
}

// PrepareChange 是“写入配置前的唯一入口”：先做同级 .bak_ 备份，再建立还原点。
// 任何新增的写配置路径都必须走这里，否则会破坏“每次变更都有还原点”的保证。
func PrepareChange(ctx ChangeContext) (ChangeSnapshot, error) {
	var snap ChangeSnapshot
	backupPath, err := BackupConfig(ctx.Target)
	if err != nil {
		return snap, fmt.Errorf("创建配置备份失败：%w", err)
	}
	snap.BackupPath = backupPath
	point, err := CreateRestorePoint(ctx)
	if err != nil {
		return snap, fmt.Errorf("创建还原点失败：%w", err)
	}
	snap.RestorePoint = point
	return snap, nil
}

// CreateRestorePoint 为“即将发生的变更”建立还原点（不会写入目标文件）。
//
// 目录直接以最终名字创建（meta.json 最后写，作为“完成标记”）：
// 目录存在但没有 meta.json 的条目在列表与清理里都会被忽略。
func CreateRestorePoint(ctx ChangeContext) (*RestorePoint, error) {
	if strings.TrimSpace(ctx.Target) == "" {
		return nil, fmt.Errorf("目标配置文件路径为空")
	}
	root := RestorePointsDir()
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	now := time.Now()
	point := &RestorePoint{
		CreatedAt:  now.Format(time.RFC3339),
		AgentID:    strings.ToLower(strings.TrimSpace(ctx.AgentID)),
		AgentLabel: strings.TrimSpace(ctx.AgentLabel),
		TargetPath: ctx.Target,
		Format:     strings.TrimSpace(ctx.Format),
		AppVersion: AppVersion,
		Operation:  strings.TrimSpace(ctx.Operation),
		ProviderID: strings.TrimSpace(ctx.ProviderID),
		ModelID:    strings.TrimSpace(ctx.ModelID),
		Note:       strings.TrimSpace(ctx.Note),
	}
	if point.AgentLabel == "" {
		point.AgentLabel = agentLabelFor(point.AgentID)
	}
	if point.Format == "" {
		point.Format = ConfigFormatAt(ctx.Target, point.AgentID)
	}
	if point.Operation == "" {
		point.Operation = OpManual
	}

	pointDir, err := reservePointDir(root, now.Format("20060102_150405_000000"))
	if err != nil {
		return nil, err
	}
	point.ID = filepath.Base(pointDir)
	abort := func(err error) (*RestorePoint, error) {
		_ = os.RemoveAll(pointDir)
		return nil, err
	}

	if info, statErr := os.Stat(ctx.Target); statErr == nil && info.Mode().IsRegular() {
		sum, size, err := copyFileToPoint(ctx.Target, filepath.Join(pointDir, RestorePointContentName))
		if err != nil {
			return abort(err)
		}
		fp, _ := FileFingerprint(ctx.Target)
		point.Existed = true
		point.SizeBytes = size
		point.SHA256 = sum
		point.Fingerprint = fp
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return abort(statErr)
	}

	meta, err := json.MarshalIndent(point, "", "  ")
	if err != nil {
		return abort(err)
	}
	meta = append(meta, '\n')
	if err := writeFileAtomic(filepath.Join(pointDir, RestorePointMetaName), meta, 0600); err != nil {
		return abort(err)
	}
	PruneRestorePoints()
	return point, nil
}

// agentLabelFor 返回 Agent 的展示名，未知 Agent 原样返回 ID。
func agentLabelFor(agentID string) string {
	for _, def := range AllAgents() {
		if strings.EqualFold(string(def.ID), agentID) {
			return def.Label
		}
	}
	return agentID
}

// reservePointDir 为还原点抢占一个唯一目录名（同秒多次调用时追加序号）。
func reservePointDir(root, base string) (string, error) {
	for seq := 0; seq < 10000; seq++ {
		name := base
		if seq > 0 {
			name = fmt.Sprintf("%s-%d", base, seq)
		}
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		return dir, nil
	}
	return "", fmt.Errorf("还原点目录名冲突过多")
}

// copyFileToPoint 把 src 的原始字节复制到 dst（0600），返回 sha256 与字节数。
func copyFileToPoint(src, dst string) (string, int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hash), in)
	if err != nil {
		out.Close()
		_ = os.Remove(dst)
		return "", 0, err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return "", 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if perm != 0 {
		_ = os.Chmod(tmpName, perm)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

var pointIDRe = regexp.MustCompile(`^\d{8}_\d{6}_\d{6}(-\d+)?$`)

// ValidRestorePointID 校验还原点 ID，避免路径穿越。
func ValidRestorePointID(id string) bool {
	return pointIDRe.MatchString(strings.TrimSpace(id))
}

func restorePointDir(id string) (string, error) {
	if !ValidRestorePointID(id) {
		return "", fmt.Errorf("还原点 ID 非法")
	}
	return filepath.Join(RestorePointsDir(), strings.TrimSpace(id)), nil
}

// RestorePointFilter 用于筛选还原点。零值表示不过滤。
type RestorePointFilter struct {
	TargetPath string
	AgentID    string
}

func (f RestorePointFilter) match(p *RestorePoint) bool {
	if f.TargetPath != "" && !strings.EqualFold(filepath.Clean(f.TargetPath), filepath.Clean(p.TargetPath)) {
		return false
	}
	if f.AgentID != "" && !strings.EqualFold(f.AgentID, p.AgentID) {
		return false
	}
	return true
}

// ListRestorePoints 返回符合筛选条件的还原点，按时间倒序（新的在前）。
// meta.json 缺失或损坏的条目会被跳过，不影响整体列表。
func ListRestorePoints(filter RestorePointFilter) []RestorePoint {
	root := RestorePointsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := make([]RestorePoint, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !ValidRestorePointID(entry.Name()) {
			continue
		}
		point, err := readRestorePointDir(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		if !filter.match(point) {
			continue
		}
		out = append(out, *point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// ReadRestorePoint 读取单个还原点的元数据。
func ReadRestorePoint(id string) (*RestorePoint, error) {
	dir, err := restorePointDir(id)
	if err != nil {
		return nil, err
	}
	return readRestorePointDir(dir)
}

func readRestorePointDir(dir string) (*RestorePoint, error) {
	data, err := os.ReadFile(filepath.Join(dir, RestorePointMetaName))
	if err != nil {
		return nil, err
	}
	var point RestorePoint
	if err := json.Unmarshal(data, &point); err != nil {
		return nil, err
	}
	if point.ID == "" {
		point.ID = filepath.Base(dir)
	}
	if strings.TrimSpace(point.TargetPath) == "" {
		return nil, fmt.Errorf("还原点 %s 缺少目标路径", point.ID)
	}
	return &point, nil
}

// RestorePointContentPath 返回还原点内的配置原文路径（Existed=false 时该文件不存在）。
func RestorePointContentPath(id string) (string, error) {
	dir, err := restorePointDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, RestorePointContentName), nil
}

// DeleteRestorePoint 删除一个还原点。
func DeleteRestorePoint(id string) error {
	dir, err := restorePointDir(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// PruneRestorePoints 按保留策略清理还原点，返回删除的条目数。
// 策略：每个目标路径最多 MaxRestorePoints 个；全部内容合计不超过 MaxRestorePointBytes，
// 超出时按最旧优先删除；另外清理超过 orphanPointAge 仍没有 meta.json 的残留目录。
func PruneRestorePoints() int {
	removed := cleanupOrphanPointDirs()
	points := ListRestorePoints(RestorePointFilter{})
	perTarget := map[string]int{}
	for _, point := range points {
		key := strings.ToLower(filepath.Clean(point.TargetPath))
		perTarget[key]++
		if perTarget[key] > MaxRestorePoints {
			if err := DeleteRestorePoint(point.ID); err == nil {
				removed++
			}
		}
	}
	points = ListRestorePoints(RestorePointFilter{})
	var total int64
	for _, point := range points {
		total += point.SizeBytes
	}
	for i := len(points) - 1; i >= 0 && total > MaxRestorePointBytes; i-- {
		if err := DeleteRestorePoint(points[i].ID); err == nil {
			total -= points[i].SizeBytes
			removed++
		}
	}
	return removed
}

// cleanupOrphanPointDirs 删除写入中断留下的目录（没有 meta.json 且已超过 orphanPointAge）。
func cleanupOrphanPointDirs() int {
	root := RestorePointsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-orphanPointAge)
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || !ValidRestorePointID(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, RestorePointMetaName)); err == nil {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			removed++
		}
	}
	return removed
}

// RestoreOutcome 描述一次回滚的结果。
type RestoreOutcome struct {
	// ID 是被使用的还原点。
	ID string `json:"id"`
	// TargetPath 是被写回的路径。
	TargetPath string `json:"target_path"`
	// RemovedPath 在“还原到文件不存在”分支下是被移走的当前文件路径。
	RemovedPath string `json:"removed_path,omitempty"`
	// Snapshot 是回滚前为当前状态建立的还原点。
	Snapshot *RestorePoint `json:"snapshot"`
	// Warning 是非致命提示（例如原文含当前 ZCode 会拒绝的键）。
	Warning string `json:"warning,omitempty"`
}

// RestoreRestorePoint 把某个还原点的内容写回它的原始路径。
//
// 回滚本身也会先给“当前状态”建一个还原点，因此可以再回滚回去。
// 当还原点记录的是“当时文件不存在”时，当前文件会被重命名成 <文件>.removed_<时间戳>
// 而不是直接删除（RemovedPath 返回该路径）。
func RestoreRestorePoint(id string) (RestoreOutcome, error) {
	var outcome RestoreOutcome
	point, err := ReadRestorePoint(id)
	if err != nil {
		return outcome, fmt.Errorf("读取还原点失败：%w", err)
	}
	outcome.ID = point.ID
	outcome.TargetPath = point.TargetPath

	// 回滚前先给当前状态留一个还原点，保证回滚可撤销。
	snapshot, err := CreateRestorePoint(ChangeContext{
		Target:    point.TargetPath,
		AgentID:   point.AgentID,
		Format:    ConfigFormatAt(point.TargetPath, point.AgentID),
		Operation: OpRestore,
		Note:      "回滚到还原点 " + point.ID,
	})
	if err != nil {
		return outcome, fmt.Errorf("回滚前创建还原点失败：%w", err)
	}
	outcome.Snapshot = snapshot

	if !point.Existed {
		removed, err := removeRestoredTarget(point.TargetPath)
		if err != nil {
			return outcome, err
		}
		outcome.RemovedPath = removed
		return outcome, nil
	}

	contentPath, err := RestorePointContentPath(point.ID)
	if err != nil {
		return outcome, err
	}
	data, err := os.ReadFile(contentPath)
	if err != nil {
		return outcome, fmt.Errorf("还原点内容缺失或不可读：%w", err)
	}
	if point.SHA256 != "" && sha256Hex(data) != point.SHA256 {
		return outcome, fmt.Errorf("还原点内容校验失败（sha256 不一致），已取消回滚")
	}
	outcome.Warning = validateRestoreContent(data, point)

	if err := writeRestoredContent(point.TargetPath, data); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// removeRestoredTarget 把当前文件移开，实现“回到文件不存在”。文件本来就不存在时返回空串。
func removeRestoredTarget(target string) (string, error) {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102_150405")
	removed := fmt.Sprintf("%s.removed_%s", target, stamp)
	for suffix := 1; ; suffix++ {
		if _, err := os.Stat(removed); os.IsNotExist(err) {
			break
		}
		removed = fmt.Sprintf("%s.removed_%s_%d", target, stamp, suffix)
	}
	if err := os.Rename(target, removed); err != nil {
		return "", fmt.Errorf("移除当前配置文件失败：%w", err)
	}
	return removed, nil
}

// writeRestoredContent 原子写回还原内容，并保证写盘前后目标未被其他程序改动。
func writeRestoredContent(target string, data []byte) error {
	before, _ := FileFingerprint(target)
	dir := filepath.Dir(target)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".restore_*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if cur, _ := FileFingerprint(target); cur != before {
		_ = os.Remove(tmpName)
		return fmt.Errorf("配置文件在回滚期间被其他程序修改，请重试")
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// validateRestoreContent 校验还原内容至少能被解析成该格式的配置，并返回非致命提示。
// 历史文件可能包含当前 ZCode 已不再接受的键，那种情况只提醒、不阻止回滚。
func validateRestoreContent(data []byte, point *RestorePoint) string {
	switch point.Format {
	case FormatV2:
		if _, err := zcodeprovider.Decode(data); err != nil {
			return "还原内容不是当前 ZCode 版本可接受的 provider_config.json：" + err.Error()
		}
	case FormatDeepSeek:
		if isYAMLPath(point.TargetPath) {
			var raw map[string]interface{}
			if err := yaml.Unmarshal(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), &raw); err != nil {
				return "还原内容不是合法的 YAML：" + err.Error()
			}
			return ""
		}
		if _, err := parseJSONCObject(data); err != nil {
			return "还原内容不是合法的 JSON 对象：" + err.Error()
		}
	case FormatOpencode:
		if _, err := parseJSONCObject(data); err != nil {
			return "还原内容不是合法的 JSON 对象：" + err.Error()
		}
	default:
		if _, err := parseJSONCObject(data); err != nil {
			return "还原内容不是合法的 JSON 对象：" + err.Error()
		}
	}
	return ""
}

// parseJSONCObject 按旧版配置的宽松规则（允许注释与尾逗号）解析 JSON 对象。
func parseJSONCObject(data []byte) (map[string]interface{}, error) {
	text := StripJSONCComments(string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	var v interface{}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, err
	}
	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("根节点必须是对象")
	}
	return obj, nil
}
