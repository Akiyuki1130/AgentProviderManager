package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	HelperFlag         = "--apm-update-helper"
	CleanupBackupFlag  = "--apm-cleanup-backup"
	parentPIDFlag      = "--parent-pid"
	stagedFlag         = "--staged"
	targetFlag         = "--target"
	sha256Flag         = "--sha256"
	backupFilePrefix   = ".apm-backup-"
	updateFilePrefix   = ".apm-update-"
	backupCleanupTries = 40
	backupCleanupDelay = 250 * time.Millisecond
	// stagedStaleAge 是启动清理暂存文件的年龄门槛：仍在下载中的暂存文件（可能由
	// 另一个实例持有）不动，只清超过这个年龄的残留。
	stagedStaleAge = time.Hour
)

// InstallRequest contains all data a separately started update helper needs.
// Paths and the digest are validated before any filesystem operation occurs.
type InstallRequest struct {
	ParentPID      int
	StagedPath     string
	TargetPath     string
	ExpectedSHA256 string
	LaunchArgs     []string
}

// ValidateInstallRequest validates helper input and ensures that paths are
// absolute, local filesystem paths. The helper never accepts URLs or shell
// command strings.
//
// It additionally pins the shape of a legitimate install: the staged file must
// be the updater's own download artifact (.apm-update-*.exe) sitting beside the
// target executable. Without that, the helper flag degenerates into a generic
// "replace this executable and run it" primitive for anyone who can spawn the
// application with arguments.
func ValidateInstallRequest(req InstallRequest) error {
	if req.ParentPID <= 0 {
		return errors.New("parent PID must be positive")
	}
	staged, err := validateExecutablePath(req.StagedPath, "staged executable")
	if err != nil {
		return err
	}
	target, err := validateExecutablePath(req.TargetPath, "target executable")
	if err != nil {
		return err
	}
	if same, err := samePath(staged, target); err != nil {
		return err
	} else if same {
		return errors.New("staged executable must differ from target executable")
	}
	// 目录比较按 Windows 路径语义忽略大小写，与同文件其余同路径判定保持同一口径。
	if !strings.EqualFold(filepath.Clean(filepath.Dir(staged)), filepath.Clean(filepath.Dir(target))) {
		return errors.New("staged executable must be beside the target executable")
	}
	if !strings.HasPrefix(filepath.Base(staged), updateFilePrefix) {
		return fmt.Errorf("staged executable must be the updater's own %s*.exe download", updateFilePrefix)
	}
	if _, err := normalizeDigest(req.ExpectedSHA256); err != nil {
		return fmt.Errorf("expected SHA-256: %w", err)
	}
	for _, arg := range req.LaunchArgs {
		if strings.IndexByte(arg, 0) >= 0 {
			return errors.New("launch argument contains NUL")
		}
	}
	return nil
}

func validateExecutablePath(raw, label string) (string, error) {
	if raw == "" || strings.IndexByte(raw, 0) >= 0 {
		return "", fmt.Errorf("%s path is invalid", label)
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s path must be absolute", label)
	}
	path := filepath.Clean(raw)
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return "", fmt.Errorf("%s must have .exe extension", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", label)
	}
	return path, nil
}

func samePath(a, b string) (bool, error) {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	// Windows 路径大小写不敏感，忽略大小写与 ValidateHelperSelfTarget 同一口径。
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb)), nil
}

// BuildHelperArgs serializes a request as argv without invoking a shell.
func BuildHelperArgs(req InstallRequest) ([]string, error) {
	if err := ValidateInstallRequest(req); err != nil {
		return nil, err
	}
	digest, _ := normalizeDigest(req.ExpectedSHA256)
	args := []string{HelperFlag, parentPIDFlag, strconv.Itoa(req.ParentPID), stagedFlag, req.StagedPath, targetFlag, req.TargetPath, sha256Flag, digest}
	if len(req.LaunchArgs) != 0 {
		args = append(args, "--")
		args = append(args, req.LaunchArgs...)
	}
	return args, nil
}

// ParseHelperArgs parses the exact format emitted by BuildHelperArgs. It does
// not interpret or expand shell syntax.
func ParseHelperArgs(args []string) (InstallRequest, error) {
	if len(args) < 9 || args[0] != HelperFlag {
		return InstallRequest{}, errors.New("invalid update helper arguments")
	}
	if args[1] != parentPIDFlag || args[3] != stagedFlag || args[5] != targetFlag || args[7] != sha256Flag {
		return InstallRequest{}, errors.New("invalid update helper argument flags")
	}
	pid, err := strconv.Atoi(args[2])
	if err != nil {
		return InstallRequest{}, errors.New("invalid parent PID")
	}
	req := InstallRequest{ParentPID: pid, StagedPath: args[4], TargetPath: args[6], ExpectedSHA256: args[8]}
	if len(args) > 9 {
		if args[9] != "--" {
			return InstallRequest{}, errors.New("invalid launch argument separator")
		}
		req.LaunchArgs = append([]string(nil), args[10:]...)
	}
	if err := ValidateInstallRequest(req); err != nil {
		return InstallRequest{}, err
	}
	return req, nil
}

// ReplaceHelperArgs replaces only updater placeholders in a predeclared argv
// template. Unknown placeholders and shell metacharacters are left untouched;
// callers should pass the result directly to the platform process API.
func ReplaceHelperArgs(args []string, req InstallRequest) ([]string, error) {
	if err := ValidateInstallRequest(req); err != nil {
		return nil, err
	}
	digest, _ := normalizeDigest(req.ExpectedSHA256)
	// 固定顺序的单次替换：既保证结果与 map 迭代顺序无关，也不会把替换进来的
	// 路径文本再当成占位符处理（strings.NewReplacer 不会重新扫描已替换的内容）。
	replacer := strings.NewReplacer(
		"{parent_pid}", strconv.Itoa(req.ParentPID),
		"{staged_path}", req.StagedPath,
		"{target_path}", req.TargetPath,
		"{sha256}", digest,
	)
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = replacer.Replace(arg)
	}
	return out, nil
}

// StartCurrentProcessHelper starts this executable in explicit helper mode.
// It never inherits os.Args: only the fixed updater arguments are passed to the
// helper. The staged executable must be a regular .exe beside the current
// executable, and the current executable itself is always the install target.
func StartCurrentProcessHelper(stagedPath, expectedSHA256 string) error {
	targetPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	stagedPath, err = filepath.Abs(stagedPath)
	if err != nil {
		return fmt.Errorf("resolve staged executable: %w", err)
	}
	stagedInfo, err := os.Lstat(stagedPath)
	if err != nil {
		return fmt.Errorf("staged executable: %w", err)
	}
	if !stagedInfo.Mode().IsRegular() {
		return errors.New("staged executable is not a regular file")
	}
	if !strings.EqualFold(filepath.Ext(stagedPath), ".exe") {
		return errors.New("staged executable must have .exe extension")
	}
	if !strings.EqualFold(filepath.Clean(filepath.Dir(stagedPath)), filepath.Clean(filepath.Dir(targetPath))) {
		return errors.New("staged executable must be beside the current executable")
	}
	req := InstallRequest{
		ParentPID:      os.Getpid(),
		StagedPath:     stagedPath,
		TargetPath:     targetPath,
		ExpectedSHA256: expectedSHA256,
	}
	args, err := BuildHelperArgs(req)
	if err != nil {
		return err
	}
	if err := startProcess(targetPath, args); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}
	return nil
}

// RunHelper parses helper argv and performs installation. It is intended to
// be called only by an explicitly launched helper process; it never runs from
// the normal application startup path.
func RunHelper(ctx context.Context, args []string) error {
	req, err := ParseHelperArgs(args)
	if err != nil {
		return err
	}
	if err := ValidateHelperSelfTarget(req); err != nil {
		return err
	}
	return InstallStaged(ctx, req)
}

// ValidateHelperSelfTarget 确认要替换的可执行文件就是「正在运行的自己」。
// 安装 helper 就是以目标可执行文件本身启动的（见 StartCurrentProcessHelper），
// 因此这条等式成立时，调用方本来就已经能在目标目录里写文件了——那已是完全控制，
// 不再需要这个开关。父进程 PID 与镜像名在 Windows 上无法可靠校验，故不依赖它们。
func ValidateHelperSelfTarget(req InstallRequest) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	self, err = filepath.Abs(self)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	target, err := filepath.Abs(req.TargetPath)
	if err != nil {
		return fmt.Errorf("resolve target executable: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		return errors.New("update helper may only replace the executable it is running from")
	}
	return nil
}

// RunBackupCleanup handles the one-shot cleanup directive passed to the
// restarted application. The path is accepted only when it names a temporary
// updater backup beside the restarted executable.
func RunBackupCleanup(ctx context.Context, args []string, executablePath string) error {
	if len(args) != 2 || args[0] != CleanupBackupFlag {
		return errors.New("invalid backup cleanup arguments")
	}
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate current executable: %w", err)
		}
	}
	return CleanupBackupWithRetry(ctx, args[1], executablePath)
}

// InstallStaged waits for the parent, verifies the staged file, replaces the
// target with rollback protection, and starts the replacement at the same path.
// The replacement receives a cleanup directive for the temporary backup because
// the helper process may still have the old executable image open on Windows.
//
// The staged file is verified before the swap and then verified again *as
// installed*: the file identity must be unchanged (catches a swapped file) and
// its digest must still match (catches an in-place rewrite). Only then is it
// started, so nothing unverified can be launched.
func InstallStaged(ctx context.Context, req InstallRequest) error {
	if err := ValidateInstallRequest(req); err != nil {
		return err
	}
	if err := waitForParentExit(ctx, req.ParentPID); err != nil {
		return err
	}
	stagedInfo, err := verifyStaged(req.StagedPath, req.ExpectedSHA256)
	if err != nil {
		return err
	}
	return replaceVerifiedExecutable(req.StagedPath, stagedInfo, req.TargetPath, req.ExpectedSHA256, req.LaunchArgs)
}

// replaceVerifiedExecutable 备份当前可执行文件、把已校验的暂存文件装到同一路径并启动它，
// 任何一步失败都回滚。拆成独立函数是为了让替换路径能在测试里直接跑到（helper 全流程需要
// 真父进程退出，无法在测试中走完）。
//
// 校验与替换之间必须闭合：先比对文件身份（暂存路径上的文件被换掉会变），再按目标路径复算
// 一次摘要（原地改写会变）。两个检查都不通过就不启动——落到目标路径上的必须是刚校验过的那
// 份内容。残留窗口只剩「复算摘要 → CreateProcess」之间的写入，而能在可执行文件目录里写文件
// 的攻击者本来就能直接替换程序，不值得为此引入平台特定的句柄改名。
func replaceVerifiedExecutable(stagedPath string, stagedInfo os.FileInfo, targetPath, expected string, extraArgs []string) error {
	backup, err := makeBackupPath(targetPath)
	if err != nil {
		return err
	}
	if err := os.Rename(targetPath, backup); err != nil {
		return fmt.Errorf("backup current executable: %w", err)
	}
	replaced := false
	// rollback 把目标路径还原成原可执行文件，返回未能完成的步骤。失败信息必须回传：
	// 回滚失败意味着目标路径上现在没有可用程序，用户需要知道。
	//
	// 删除暂存文件只在 replaced（安装已落盘）之后执行：那时暂存文件已经被改名到目标
	// 路径，删除本就是个 no-op；而「改名失败」分支 replaced 仍为 false，暂存文件还
	// 在原位、是用户刚下载的唯一副本，删掉就只能重新下载，因此必须保留。
	rollback := func() error {
		var problems []string
		if replaced {
			if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
				problems = append(problems, fmt.Sprintf("移除已安装文件失败：%v", err))
			}
			if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
				problems = append(problems, fmt.Sprintf("清理暂存文件失败：%v", err))
			}
		}
		if err := os.Rename(backup, targetPath); err != nil {
			problems = append(problems, fmt.Sprintf("恢复原可执行文件失败：%v", err))
		}
		if len(problems) > 0 {
			return errors.New(strings.Join(problems, "；"))
		}
		return nil
	}
	if err := os.Rename(stagedPath, targetPath); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("install staged executable: %v（回滚失败：%v，请手动恢复 %s）", err, rbErr, backup)
		}
		return fmt.Errorf("install staged executable: %w", err)
	}
	replaced = true
	installedInfo, err := os.Stat(targetPath)
	if err != nil {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("verify installed executable: %v（回滚失败：%v，请手动恢复 %s）", err, rbErr, backup)
		}
		return fmt.Errorf("verify installed executable: %v", err)
	}
	if !os.SameFile(stagedInfo, installedInfo) {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("暂存文件在校验后被替换（回滚失败：%v，请手动恢复 %s）", rbErr, backup)
		}
		return errors.New("staged executable changed after verification; the update was rolled back")
	}
	if err := verifySHA256(targetPath, expected); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("安装后的可执行文件校验失败：%v（回滚失败：%v，请手动恢复 %s）", err, rbErr, backup)
		}
		return fmt.Errorf("installed executable failed verification: %w", err)
	}
	launchArgs := append([]string{CleanupBackupFlag, backup}, extraArgs...)
	if err := startProcess(targetPath, launchArgs); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("start updated executable: %v（回滚失败：%v，请手动恢复 %s）", err, rbErr, backup)
		}
		return fmt.Errorf("start updated executable: %w", err)
	}
	return nil
}

func makeBackupPath(target string) (string, error) {
	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, backupFilePrefix+"*.exe")
	if err != nil {
		return "", fmt.Errorf("create backup name: %w", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

// ValidateBackupCleanupPath accepts only a temporary updater backup beside the
// current executable. It prevents cleanup arguments from becoming arbitrary
// file deletion commands.
func ValidateBackupCleanupPath(path, executablePath string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return "", errors.New("backup path is invalid")
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve backup path: %w", err)
	}
	cleanExecutable, err := filepath.Abs(executablePath)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(filepath.Dir(cleanPath)), filepath.Clean(filepath.Dir(cleanExecutable))) {
		return "", errors.New("backup path must be beside the executable")
	}
	base := filepath.Base(cleanPath)
	ext := filepath.Ext(cleanPath)
	legacyHiddenName := ext == base
	if !strings.HasPrefix(base, backupFilePrefix) || (!legacyHiddenName && ext != "" && !strings.EqualFold(ext, ".exe")) {
		return "", errors.New("backup path is not an updater backup")
	}
	info, err := os.Lstat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cleanPath, nil
		}
		return "", fmt.Errorf("stat backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("backup path is not a regular file")
	}
	return cleanPath, nil
}

// CleanupBackup removes a helper-created backup. Missing files are already
// clean; callers can retry when Windows still has the old image open.
func CleanupBackup(path, executablePath string) error {
	cleanPath, err := ValidateBackupCleanupPath(path, executablePath)
	if err != nil {
		return err
	}
	if same, err := samePath(cleanPath, executablePath); err != nil {
		return err
	} else if same {
		return errors.New("backup path must differ from the executable")
	}
	if err := os.Remove(cleanPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CleanupBackupWithRetry tolerates the short period in which Windows releases
// the old executable after the update helper starts the replacement.
func CleanupBackupWithRetry(ctx context.Context, path, executablePath string) error {
	cleanPath, err := ValidateBackupCleanupPath(path, executablePath)
	if err != nil {
		return err
	}
	if same, err := samePath(cleanPath, executablePath); err != nil {
		return err
	} else if same {
		return errors.New("backup path must differ from the executable")
	}
	var lastErr error
	for attempt := 0; attempt < backupCleanupTries; attempt++ {
		if err := os.Remove(cleanPath); err == nil || os.IsNotExist(err) {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backupCleanupDelay):
		}
	}
	return lastErr
}

// CleanupStaleBackups removes updater backups left by an older helper version.
// It is intentionally limited to the current executable directory and the
// updater's reserved filename prefix.
func CleanupStaleBackups(ctx context.Context, executablePath string) error {
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate current executable: %w", err)
		}
	}
	executablePath, err := filepath.Abs(executablePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	entries, err := os.ReadDir(filepath.Dir(executablePath))
	if err != nil {
		return fmt.Errorf("read executable directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), backupFilePrefix) {
			continue
		}
		path := filepath.Join(filepath.Dir(executablePath), entry.Name())
		if err := CleanupBackupWithRetry(ctx, path, executablePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale updater backup %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// CleanupStaleStaged removes staged update downloads abandoned by a crashed or
// closed download. Only the updater's own reserved prefix beside the current
// executable is touched, and only files older than stagedStaleAge, so a download
// still in flight (or one just handed to a helper by another instance) survives.
func CleanupStaleStaged(ctx context.Context, executablePath string) error {
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate current executable: %w", err)
		}
	}
	executablePath, err := filepath.Abs(executablePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	dir := filepath.Dir(executablePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read executable directory: %w", err)
	}
	cutoff := time.Now().Add(-stagedStaleAge)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), updateFilePrefix) {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".exe") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale staged update %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// verifyStaged 校验暂存文件并返回它当时的身份（卷序列号 + 文件索引），用于替换后确认
// 「落到目标路径上的就是校验过的那个文件」。
//
// 身份必须取自**打开句柄**的 Stat：路径式 os.Stat 不带身份，而 os.SameFile 是按记住的
// 路径重新打开文件去补身份的——暂存路径在改名之后不复存在，那条路会直接判为「不同文件」。
// 句柄不能在改名期间一直持有：Go 的 os.Open 在 Windows 上只共享 READ|WRITE，持有它会让
// 紧接着的改名失败（这两条平台行为都由测试锁住）。
func verifyStaged(path, expected string) (os.FileInfo, error) {
	expected, err := normalizeDigest(expected)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open staged executable: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat staged executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("staged executable is not a regular file")
	}
	if info.Size() > maxExecutable {
		return nil, errors.New("staged executable exceeds the configured size limit")
	}
	actual, err := hashFileContents(f)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(actual, expected) {
		return nil, fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	return info, nil
}

func verifySHA256(path, expected string) error {
	expected, err := normalizeDigest(expected)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}
	if info.Size() > maxExecutable {
		return errors.New("executable exceeds the configured size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open executable: %w", err)
	}
	defer f.Close()
	actual, err := hashFileContents(f)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	return nil
}

// hashFileContents 计算打开句柄从头部的 SHA-256，读取量封顶在 maxExecutable+1
// （超过上限的文件必然与期望摘要不符，不必读完）。
func hashFileContents(f *os.File) (string, error) {
	h := sha256.New()
	if _, err := io.CopyN(h, f, maxExecutable+1); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("hash executable: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
