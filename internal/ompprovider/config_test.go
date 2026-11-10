package ompprovider

import (
	"errors"
	"strings"
	"testing"
)

func mustLoadConfig(t *testing.T, data string) *ConfigFile {
	t.Helper()
	file, err := LoadConfig([]byte(data))
	if err != nil {
		t.Fatalf("LoadConfig 失败：%v", err)
	}
	return file
}

func TestLoadConfigRobustness(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr error
	}{
		{name: "空内容", data: ""},
		{name: "纯注释", data: "# 注释\n"},
		{name: "null 根", data: "null\n"},
		{name: "BOM 前缀", data: "\xef\xbb\xbfmodelRoles:\n  default: p1/m1\n"},
		{name: "CRLF 行尾", data: "modelRoles:\r\n  default: p1/m1\r\n"},
		{name: "多文档报错", data: "modelRoles: {}\n---\nmodelRoles: {}\n", wantErr: ErrMultiDocument},
		{name: "重复键报错", data: "modelRoles:\n  default: a\n  default: b\n", wantErr: ErrDuplicateKey},
		{name: "根不是映射", data: "- a\n", wantErr: ErrRootNotMapping},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := LoadConfig([]byte(test.data))
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("期望 %v，实际 %v", test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错：%v", err)
			}
			if file == nil {
				t.Fatal("应返回空文档")
			}
		})
	}
}

func TestModelRoles(t *testing.T) {
	t.Run("缺失时返回空映射", func(t *testing.T) {
		roles, err := mustLoadConfig(t, "").ModelRoles()
		if err != nil || len(roles) != 0 {
			t.Fatalf("期望空映射：%v %v", roles, err)
		}
	})
	t.Run("null 时返回空映射", func(t *testing.T) {
		roles, err := mustLoadConfig(t, "modelRoles: null\n").ModelRoles()
		if err != nil || len(roles) != 0 {
			t.Fatalf("期望空映射：%v %v", roles, err)
		}
	})
	t.Run("正常读取", func(t *testing.T) {
		roles, err := mustLoadConfig(t, "modelRoles:\n  default: p1/m1\n  fast: p2/m2:high\n").ModelRoles()
		if err != nil {
			t.Fatalf("不应报错：%v", err)
		}
		if roles["default"] != "p1/m1" || roles["fast"] != "p2/m2:high" {
			t.Fatalf("内容不对：%#v", roles)
		}
	})
	t.Run("非映射报错", func(t *testing.T) {
		if _, err := mustLoadConfig(t, "modelRoles: nope\n").ModelRoles(); err == nil {
			t.Fatal("非映射应报错")
		}
	})
	t.Run("值非字符串报错", func(t *testing.T) {
		if _, err := mustLoadConfig(t, "modelRoles:\n  default: 42\n").ModelRoles(); err == nil {
			t.Fatal("非字符串值应报错")
		}
	})
}

func TestSetModelRole(t *testing.T) {
	original := "# 文件头注释\nmodelRoles:\n  # default 注释\n  default: p1/m1 # 行内注释\notherKey: keep-me\n"
	t.Run("写入并保留注释与未知键", func(t *testing.T) {
		file := mustLoadConfig(t, original)
		if err := file.SetModelRole("default", "p2/m2"); err != nil {
			t.Fatalf("SetModelRole 失败：%v", err)
		}
		out, err := file.Bytes()
		if err != nil {
			t.Fatalf("Bytes 失败：%v", err)
		}
		text := string(out)
		for _, want := range []string{"# 文件头注释", "# default 注释", "# 行内注释", "otherKey: keep-me", "default: p2/m2"} {
			if !strings.Contains(text, want) {
				t.Fatalf("输出缺少 %q：\n%s", want, text)
			}
		}
		roles, _ := file.ModelRoles()
		if roles["default"] != "p2/m2" {
			t.Fatalf("写入结果不对：%#v", roles)
		}
	})
	t.Run("空 selector 删除键", func(t *testing.T) {
		file := mustLoadConfig(t, original)
		if err := file.SetModelRole("default", ""); err != nil {
			t.Fatalf("SetModelRole 失败：%v", err)
		}
		roles, _ := file.ModelRoles()
		if _, ok := roles["default"]; ok {
			t.Fatalf("键应被删除：%#v", roles)
		}
	})
	t.Run("空文档新建 modelRoles", func(t *testing.T) {
		file := mustLoadConfig(t, "")
		if err := file.SetModelRole("default", "p1/m1"); err != nil {
			t.Fatalf("SetModelRole 失败：%v", err)
		}
		roles, _ := file.ModelRoles()
		if roles["default"] != "p1/m1" {
			t.Fatalf("写入结果不对：%#v", roles)
		}
	})
	t.Run("拒绝非法输入", func(t *testing.T) {
		tests := []struct {
			role     string
			selector string
		}{
			{role: "", selector: "p1/m1"},
			{role: "default", selector: "p1/*"},
			{role: "default", selector: "@fast"},
			{role: "default", selector: "p1"},
			{role: "default", selector: "/m1"},
			{role: "default", selector: "p1/"},
		}
		for _, test := range tests {
			file := mustLoadConfig(t, "")
			if err := file.SetModelRole(test.role, test.selector); err == nil {
				t.Fatalf("role=%q selector=%q 应被拒绝", test.role, test.selector)
			}
		}
	})
	t.Run("重复设置值不变则保留节点", func(t *testing.T) {
		file := mustLoadConfig(t, "modelRoles:\n  default: p1/m1 # 保留注释\n")
		if err := file.SetModelRole("default", "p1/m1"); err != nil {
			t.Fatalf("SetModelRole 失败：%v", err)
		}
		out, _ := file.Bytes()
		if !strings.Contains(string(out), "# 保留注释") {
			t.Fatalf("行内注释应保留：\n%s", out)
		}
	})
}

func TestConfigBytesEmpty(t *testing.T) {
	file := NewConfigFile()
	out, err := file.Bytes()
	if err != nil {
		t.Fatalf("Bytes 失败：%v", err)
	}
	if len(out) != 0 {
		t.Fatalf("空文档应为空输出：%q", out)
	}
}
