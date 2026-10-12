package dshprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const nl = "\n"

func TestSetCredentialRefCreatesVersionedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	if err := SetCredentialRef(path, "ACME_API_KEY", "sk-placeholder"); err != nil {
		t.Fatalf("SetCredentialRef: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Fatalf("credentials must carry version 1 for DSH 0.2: %s", data)
	}
	refs, err := LoadCredentialRefs(path)
	if err != nil {
		t.Fatalf("LoadCredentialRefs: %v", err)
	}
	if refs["ACME_API_KEY"] != "sk-placeholder" {
		t.Fatalf("refs = %v", refs)
	}
}

func TestSetCredentialRefMigratesFlatLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	flat := "# keep me" + nl + "OLD_KEY: sk-old" + nl
	if err := os.WriteFile(path, []byte(flat), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := SetCredentialRef(path, "NEW_KEY", "sk-new"); err != nil {
		t.Fatalf("SetCredentialRef: %v", err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "version: 1") || !strings.Contains(text, "refs:") {
		t.Fatalf("flat layout not upgraded: %s", text)
	}
	if !strings.Contains(text, "OLD_KEY: sk-old") {
		t.Fatalf("existing ref lost: %s", text)
	}
	refs, err := LoadCredentialRefs(path)
	if err != nil {
		t.Fatalf("LoadCredentialRefs: %v", err)
	}
	if refs["NEW_KEY"] != "sk-new" || refs["OLD_KEY"] != "sk-old" {
		t.Fatalf("refs = %v", refs)
	}
}

func TestCredentialWritePreservesRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	doc := "version: 1" + nl + "refs:" + nl + "  KEEP_ME: sk-keep" + nl +
		"records:" + nl + "  llm-pi-ai/openai-codex:" + nl + "    kind: grant" + nl +
		"    payload:" + nl + "      type: oauth" + nl + "      access: token-placeholder" + nl
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := SetCredentialRef(path, "ADDED", "sk-added"); err != nil {
		t.Fatalf("SetCredentialRef: %v", err)
	}
	data, _ := os.ReadFile(path)
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	records, _ := parsed["records"].(map[string]interface{})
	if records == nil || records["llm-pi-ai/openai-codex"] == nil {
		t.Fatalf("records lost: %s", data)
	}
	refs, _ := parsed["refs"].(map[string]interface{})
	if refs["KEEP_ME"] != "sk-keep" || refs["ADDED"] != "sk-added" {
		t.Fatalf("refs = %v", refs)
	}
}

func TestRemoveCredentialRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	if err := SetCredentialRef(path, "A_KEY", "sk-a"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := SetCredentialRef(path, "B_KEY", "sk-b"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := RemoveCredentialRef(path, "A_KEY"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	refs, err := LoadCredentialRefs(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := refs["A_KEY"]; ok {
		t.Fatalf("A_KEY still present: %v", refs)
	}
	if refs["B_KEY"] != "sk-b" {
		t.Fatalf("B_KEY lost: %v", refs)
	}
}

func TestCredentialVersionMismatchIsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	doc := "version: 2" + nl + "refs:" + nl + "  A: sk-a" + nl
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := SetCredentialRef(path, "B", "sk-b"); err == nil {
		t.Fatal("expected version mismatch error")
	}
}

func TestSetCredentialRefRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	if err := SetCredentialRef(path, "bad name", "sk-x"); err == nil {
		t.Fatal("expected invalid ref name error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid name must not create the credentials file")
	}
}
