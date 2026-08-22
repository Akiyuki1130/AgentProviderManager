package update

import (
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
