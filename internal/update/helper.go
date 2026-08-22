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
	backupCleanupTries = 40
	backupCleanupDelay = 250 * time.Millisecond
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
	return filepath.Clean(aa) == filepath.Clean(bb), nil
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
	replacements := map[string]string{
		"{parent_pid}":  strconv.Itoa(req.ParentPID),
		"{staged_path}": req.StagedPath,
		"{target_path}": req.TargetPath,
		"{sha256}":      digest,
	}
	out := make([]string, len(args))
	for i, arg := range args {
		for from, to := range replacements {
			arg = strings.ReplaceAll(arg, from, to)
		}
		out[i] = arg
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
	if filepath.Clean(filepath.Dir(stagedPath)) != filepath.Clean(filepath.Dir(targetPath)) {
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
	return InstallStaged(ctx, req)
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
func InstallStaged(ctx context.Context, req InstallRequest) error {
	if err := ValidateInstallRequest(req); err != nil {
		return err
	}
	if err := waitForParentExit(ctx, req.ParentPID); err != nil {
		return err
	}
	if err := verifySHA256(req.StagedPath, req.ExpectedSHA256); err != nil {
		return err
	}

	backup, err := makeBackupPath(req.TargetPath)
	if err != nil {
		return err
	}
	if err := os.Rename(req.TargetPath, backup); err != nil {
		return fmt.Errorf("backup current executable: %w", err)
	}
	staged := req.StagedPath
	replaced := false
	rollback := func() {
		if replaced {
			_ = os.Remove(req.TargetPath)
		}
		_ = os.Rename(backup, req.TargetPath)
		_ = os.Remove(staged)
	}
	if err := os.Rename(staged, req.TargetPath); err != nil {
		_ = os.Rename(backup, req.TargetPath)
		return fmt.Errorf("install staged executable: %w", err)
	}
	replaced = true
	launchArgs := append([]string{CleanupBackupFlag, backup}, req.LaunchArgs...)
	if err := startProcess(req.TargetPath, launchArgs); err != nil {
		rollback()
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
	if filepath.Clean(filepath.Dir(cleanPath)) != filepath.Clean(filepath.Dir(cleanExecutable)) {
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

func verifySHA256(path, expected string) error {
	expected, err := normalizeDigest(expected)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat staged executable: %w", err)
	}
	if info.Size() > maxExecutable {
		return errors.New("staged executable exceeds the configured size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open staged executable: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyN(h, f, maxExecutable+1); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("hash staged executable: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("staged SHA-256 mismatch: got %s", actual)
	}
	return nil
}
