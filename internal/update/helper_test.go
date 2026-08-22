package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHelperArgsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged.exe")
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
