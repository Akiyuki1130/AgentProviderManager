package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHelperArgsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, ".apm-update-1234.exe")
	target := filepath.Join(dir, "current.exe")
	for _, path := range []string{staged, target} {
		if err := os.WriteFile(path, []byte("placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash := sha256.Sum256([]byte("placeholder"))
	req := InstallRequest{ParentPID: 12345, StagedPath: staged, TargetPath: target, ExpectedSHA256: hex.EncodeToString(hash[:]), LaunchArgs: []string{"--safe", "value with spaces", "$(not-a-shell-command)"}}
	args, err := BuildHelperArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseHelperArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Fatalf("round trip mismatch:\n got=%+v\nwant=%+v", got, req)
	}
}

func TestHelperArgsRejectMalformedInput(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{HelperFlag},
		{HelperFlag, parentPIDFlag, "1", stagedFlag, "relative.exe", targetFlag, "target.exe", sha256Flag, "bad"},
		{HelperFlag, parentPIDFlag, "1", stagedFlag, "x.exe", targetFlag, "y.exe", sha256Flag, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "not-separator"},
	} {
		if _, err := ParseHelperArgs(args); err == nil {
			t.Errorf("accepted malformed args %v", args)
		}
	}
}

func TestBackupCleanupRemovesOnlyUpdaterBackup(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "AgentProviderManager.exe")
	backup := filepath.Join(dir, ".apm-backup-123.exe")
	if err := os.WriteFile(executable, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunBackupCleanup(context.Background(), []string{CleanupBackupFlag, backup}, executable); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
	if err := RunBackupCleanup(context.Background(), []string{CleanupBackupFlag, backup}, executable); err != nil {
		t.Fatalf("cleanup should be idempotent: %v", err)
	}
}

func TestBackupCleanupRejectsArbitraryPath(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "AgentProviderManager.exe")
	if err := os.WriteFile(executable, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(dir, "other.exe"),
		filepath.Join(t.TempDir(), ".apm-backup-other.exe"),
		filepath.Join(dir, ".apm-backup-bad.txt"),
	} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RunBackupCleanup(context.Background(), []string{CleanupBackupFlag, path}, executable); err == nil {
			t.Errorf("accepted arbitrary cleanup path %q", path)
		}
	}
}

// 安装请求必须被钉成「更新器自己的下载产物 + 与目标同目录」，
// 否则 helper 开关会退化成通用的「替换任意 exe 并运行它」。
func TestValidateInstallRequestPinsUpdaterShape(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	staged := filepath.Join(dir, ".apm-update-1234.exe")
	target := filepath.Join(dir, "current.exe")
	plain := filepath.Join(dir, "downloaded.exe")
	foreign := filepath.Join(other, ".apm-update-5678.exe")
	for _, path := range []string{staged, target, plain, foreign} {
		if err := os.WriteFile(path, []byte("placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash := sha256.Sum256([]byte("placeholder"))
	digest := hex.EncodeToString(hash[:])
	base := func() InstallRequest {
		return InstallRequest{ParentPID: 1234, StagedPath: staged, TargetPath: target, ExpectedSHA256: digest}
	}

	if err := ValidateInstallRequest(base()); err != nil {
		t.Fatalf("合法请求不应被拒：%v", err)
	}
	cases := map[string]func(*InstallRequest){
		"暂存文件缺少更新器前缀": func(r *InstallRequest) { r.StagedPath = plain },
		"暂存文件在别的目录":   func(r *InstallRequest) { r.StagedPath = foreign },
		"目标就是暂存文件自身":  func(r *InstallRequest) { r.TargetPath = staged },
		"摘要不是 64 位十六进制": func(r *InstallRequest) { r.ExpectedSHA256 = "not-a-digest" },
		"父进程 PID 非正":    func(r *InstallRequest) { r.ParentPID = 0 },
	}
	for name, mutate := range cases {
		req := base()
		mutate(&req)
		if err := ValidateInstallRequest(req); err == nil {
			t.Errorf("%s：应被拒绝", name)
		}
	}
}

// helper 只能替换「它自己正在运行的那个可执行文件」：别的目标一律拒绝，且不得动文件。
func TestRunHelperRejectsForeignTarget(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, ".apm-update-1234.exe")
	target := filepath.Join(dir, "current.exe")
	// target 与 staged 内容必须不同，否则「目标未被 staged 覆盖」这条断言恒真、没有区分力。
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("updated"))
	args, err := BuildHelperArgs(InstallRequest{
		ParentPID: os.Getpid(), StagedPath: staged, TargetPath: target, ExpectedSHA256: hex.EncodeToString(hash[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunHelper(context.Background(), args); err == nil {
		t.Fatal("helper 不应替换非自身可执行文件")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("目标文件不应被 staged 覆盖：%v %q", err, data)
	}
	if req := (InstallRequest{TargetPath: target}); ValidateHelperSelfTarget(req) == nil {
		t.Error("非自身路径应被 ValidateHelperSelfTarget 拒绝")
	}
}

// 启动清理只删「过期 + 更新器前缀 + .exe」的暂存残留：正在下载的、别的后缀的、
// 用户自己的下载都不能碰。
func TestCleanupStaleStagedRemovesOnlyOldUpdaterDownloads(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "AgentProviderManager.exe")
	old := filepath.Join(dir, ".apm-update-old.exe")
	fresh := filepath.Join(dir, ".apm-update-fresh.exe")
	oldNotExe := filepath.Join(dir, ".apm-update-old.txt")
	backup := filepath.Join(dir, ".apm-backup-1.exe")
	userFile := filepath.Join(dir, "user-download.exe")
	for _, path := range []string{executable, old, fresh, oldNotExe, backup, userFile} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stale := time.Now().Add(-2 * time.Hour)
	for _, path := range []string{old, oldNotExe} {
		if err := os.Chtimes(path, stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanupStaleStaged(context.Background(), executable); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("过期的暂存文件应被清理：%v", err)
	}
	for _, path := range []string{fresh, oldNotExe, backup, userFile, executable} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s 不应被清理：%v", path, err)
		}
	}
}

// 替换路径必须能真的跑起来：已校验的暂存文件被改名到目标路径，身份复检与按目标路径
// 复算摘要都通过，最终因目标不是可执行文件而启动失败 → 回滚到原文件并清掉暂存文件。
func TestReplaceVerifiedExecutableRollsBackWhenLaunchFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "current.exe")
	staged := filepath.Join(dir, ".apm-update-1234.exe")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("updated"))
	digest := hex.EncodeToString(hash[:])
	info, err := verifyStaged(staged, digest)
	if err != nil {
		t.Fatalf("verifyStaged: %v", err)
	}

	err = replaceVerifiedExecutable(staged, info, target, digest, nil)
	if err == nil {
		t.Fatal("目标是普通文本文件，启动必然失败，应返回错误")
	}
	// 错误必须来自「启动」这一步：若来自改名、身份复检或摘要复算，说明新逻辑的前提不成立。
	if !strings.Contains(err.Error(), "start updated executable") {
		t.Fatalf("错误应来自启动步骤，实际：%v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("回滚后目标应恢复原内容：%v %q", err, data)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Errorf("回滚应清掉暂存文件：%v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), backupFilePrefix) {
			t.Errorf("回滚后不应残留备份文件：%s", entry.Name())
		}
	}
}

// 校验 → 替换之间被换文件必须被拦下：路径上的文件身份与摘要都对不上就不启动。
func TestReplaceVerifiedExecutableRejectsSwappedFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "current.exe")
	staged := filepath.Join(dir, ".apm-update-1234.exe")
	for path, content := range map[string]string{target: "original", staged: "updated"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash := sha256.Sum256([]byte("updated"))
	digest := hex.EncodeToString(hash[:])
	info, err := verifyStaged(staged, digest)
	if err != nil {
		t.Fatalf("verifyStaged: %v", err)
	}
	// 校验之后把暂存路径上的文件换掉（换成另一个文件，身份随之改变）。
	sneaky := filepath.Join(dir, "sneaky.exe")
	if err := os.WriteFile(sneaky, []byte("malicious"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staged, staged+".verified"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sneaky, staged); err != nil {
		t.Fatal(err)
	}

	err = replaceVerifiedExecutable(staged, info, target, digest, nil)
	if err == nil || !strings.Contains(err.Error(), "changed after verification") {
		t.Fatalf("换文件后应被拦下，实际：%v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("被拦下时必须回滚成原文件：%v %q", err, data)
	}
}

func TestCleanupStaleBackupsRemovesLegacyName(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "Renamed Manager.exe")
	legacy := filepath.Join(dir, ".apm-backup-9125725")
	if err := os.WriteFile(executable, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CleanupStaleBackups(context.Background(), executable); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy backup still exists: %v", err)
	}
}
