package dshprovider

import (
	"os"
	"path/filepath"
	"testing"
)

// home 级 patch（$DSH_HOME/cordis.patch.yml）不在 profiles/ 下，也必须被识别成
// patch；否则空文件会被当成 0.1 settings，用 settings 布局覆写后 DSH 读不出。
func TestDetectHomePatchIsPatchNotLegacy(t *testing.T) {
	home := filepath.Join("C:", "Users", "placeholder", ".dsh")
	path := filepath.Join(home, ProfilePatchFilename)
	cases := []struct {
		name string
		data string
		want Format
	}{
		{"空字节", "", FormatProfileOther},
		{"只有注释", "# just a comment\n", FormatProfileOther},
		{"空数组", "[]", FormatProfileOther},
		{"普通条目", "- id: llm-pi-ai", FormatProfileOther},
		{"settings 映射不冒充 patch", "llm-pi-ai:\n  providers: {}\n", FormatUnknown},
	}
	for _, tc := range cases {
		if got := Detect(path, []byte(tc.data)); got != tc.want {
			t.Errorf("%s: Detect = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestDetectMissingHomePatchIsPatch(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ProfilePatchFilename)
	if got := DetectFile(path); got != FormatProfileOther {
		t.Fatalf("不存在的 home patch 应为 patch 格式，得到 %s", got)
	}
	if err := os.WriteFile(path, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DetectFile(path); got != FormatProfileOther {
		t.Fatalf("只有注释的 home patch 应为 patch 格式，得到 %s", got)
	}
}
