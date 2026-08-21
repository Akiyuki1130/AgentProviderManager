package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func keychainStamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func dpapiProtect(data []byte, protect bool) ([]byte, error) {
	if protected, err := dpapiWindows(data, protect); err == nil {
		return protected, nil
	}
	if protect {
		return nil, &KeychainCryptoError{Msg: "Windows DPAPI 保护失败，未保存钥匙串"}
	}
	// Files written by versions before DPAPI was implemented used a reversible
	// base64 payload. Accept that legacy format once so it can be re-encrypted.
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err == nil {
		var tmp interface{}
		if json.Unmarshal(decoded, &tmp) == nil {
			return decoded, nil
		}
	}
	return nil, &KeychainCryptoError{Msg: "钥匙串解密失败"}
}

func protectKeychainEntries(entries []KeychainEntry) (string, error) {
	payload, _ := json.Marshal(entries)
	enc, err := dpapiProtect(payload, true)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

func unprotectKeychainEntries(encoded string) ([]KeychainEntry, error) {
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, &KeychainCryptoError{Msg: "钥匙串密文格式无效或无法解码"}
	}
	payload, err := dpapiProtect(encrypted, false)
	if err != nil {
		return nil, err
	}
	var entries []KeychainEntry
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil, &KeychainCryptoError{Msg: "钥匙串密文格式无效或无法解码"}
	}
	return entries, nil
}

func loadKeychainUnlocked(path string) ([]KeychainEntry, bool, bool) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, true, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, false
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Corrupted: rename to .bak
		bak := path + ".bak"
		if _, err := os.Stat(bak); err == nil {
			bak = fmt.Sprintf("%s.bak_%s", path, time.Now().Format("20060102_150405_000000"))
		}
		_ = os.Rename(path, bak)
		return nil, false, false
	}
	m, isMap := raw.(map[string]interface{})
	if isMap && m["version"] != nil {
		ver, ok := m["version"].(float64)
		if !ok {
			return nil, false, false
		}
		version := int(ver)
		if version > KeychainFormatVer {
			return nil, false, false
		}
		if version == KeychainFormatVer {
			enc, _ := m["encrypted"].(string)
			if enc == "" {
				return nil, false, false
			}
			entries, err := unprotectKeychainEntries(enc)
			if err != nil {
				return nil, false, false
			}
			return validateKeychainEntries(entries)
		}
		if version == 1 {
			entriesRaw, _ := m["entries"].([]interface{})
			return parseLegacyEntries(entriesRaw)
		}
		return nil, false, false
	}
	if isMap {
		if entriesRaw, ok := m["entries"].([]interface{}); ok {
			return parseLegacyEntries(entriesRaw)
		}
		return nil, false, true
	}
	if arr, ok := raw.([]interface{}); ok {
		return parseLegacyEntries(arr)
	}
	return nil, false, true
}

func validateKeychainEntries(entries []KeychainEntry) ([]KeychainEntry, bool, bool) {
	if len(entries) > MaxKeychainEntries {
		return nil, false, false
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.ID == "" || e.Name == "" || len(e.ID) > MaxKeychainIDLen || len(e.Name) > MaxKeychainNameLen || len(e.APIKey) > MaxAPIKeyLength || len(e.Note) > MaxKeychainNoteLen {
			return nil, false, false
		}
		if seen[e.ID] {
			return nil, false, false
		}
		seen[e.ID] = true
	}
	// sort by name
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if strings.ToLower(entries[j].Name) < strings.ToLower(entries[i].Name) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	return entries, true, false
}

func parseLegacyEntries(raw []interface{}) ([]KeychainEntry, bool, bool) {
	var out []KeychainEntry
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, false, true
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		apiKey, _ := m["api_key"].(string)
		if apiKey == "" {
			apiKey, _ = m["apiKey"].(string)
		}
		note, _ := m["note"].(string)
		createdAt, _ := m["created_at"].(string)
		updatedAt, _ := m["updated_at"].(string)
		id = strings.TrimSpace(id)
		name = strings.TrimSpace(name)
		apiKey = strings.TrimSpace(apiKey)
		note = strings.TrimSpace(note)
		if id == "" || name == "" || len(id) > MaxKeychainIDLen || len(name) > MaxKeychainNameLen || len(apiKey) > MaxAPIKeyLength || len(note) > MaxKeychainNoteLen {
			return nil, false, true
		}
		out = append(out, KeychainEntry{ID: id, Name: name, APIKey: apiKey, Note: note, CreatedAt: createdAt, UpdatedAt: updatedAt})
	}
	if len(out) > MaxKeychainEntries {
		return nil, false, true
	}
	seen := map[string]bool{}
	for _, e := range out {
		if seen[e.ID] {
			return nil, false, true
		}
		seen[e.ID] = true
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if strings.ToLower(out[j].Name) < strings.ToLower(out[i].Name) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, true, true
}

func saveKeychainUnlocked(path string, entries []KeychainEntry) bool {
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	d := dir
	if d == "" {
		d = "."
	}
	enc, err := protectKeychainEntries(entries)
	if err != nil {
		return false
	}
	payload := map[string]interface{}{"version": KeychainFormatVer, "encrypted": enc}
	data, _ := json.MarshalIndent(payload, "", "  ")
	tmpFile, err := os.CreateTemp(d, "keychain.tmp_*")
	if err != nil {
		return false
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return false
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return false
	}
	return true
}

func LoadKeychain(path string) ([]KeychainEntry, error) {
	if path == "" {
		path = KeychainPath()
	}
	entries, valid, legacy := loadKeychainUnlocked(path)
	if !valid {
		return nil, &KeychainDataError{Msg: "钥匙串文件结构损坏，请先备份或删除该文件后重试"}
	}
	if legacy {
		if !saveKeychainUnlocked(path, entries) {
			return nil, &KeychainCryptoError{Msg: "旧版钥匙串迁移到 DPAPI 加密存储失败"}
		}
	}
	return entries, nil
}

func ValidateKeychainEntry(entry KeychainEntry) (bool, string) {
	if strings.TrimSpace(entry.Name) == "" {
		return false, "请填写项目名称"
	}
	if len(entry.Name) > MaxKeychainNameLen {
		return false, fmt.Sprintf("项目名称不能超过 %d 个字符", MaxKeychainNameLen)
	}
	if strings.TrimSpace(entry.APIKey) == "" {
		return false, "请填写 API Key"
	}
	if len(entry.APIKey) > MaxAPIKeyLength {
		return false, fmt.Sprintf("API Key 不能超过 %d 个字符", MaxAPIKeyLength)
	}
	if len(entry.Note) > MaxKeychainNoteLen {
		return false, fmt.Sprintf("备注不能超过 %d 个字符", MaxKeychainNoteLen)
	}
	if len(entry.ID) > MaxKeychainIDLen {
		return false, fmt.Sprintf("条目 ID 不能超过 %d 个字符", MaxKeychainIDLen)
	}
	return true, ""
}

func UpsertKeychainEntry(path string, entry KeychainEntry) (bool, string, *KeychainEntry) {
	if ok, msg := ValidateKeychainEntry(entry); !ok {
		return false, msg, nil
	}
	if path == "" {
		path = KeychainPath()
	}
	entries, valid, _ := loadKeychainUnlocked(path)
	if !valid {
		if _, err := os.Stat(path); err == nil {
			return false, "钥匙串文件结构损坏，请先备份或删除该文件后重试", nil
		}
		entries = nil
	}
	now := keychainStamp()
	entry.Name = strings.TrimSpace(entry.Name)
	entry.APIKey = strings.TrimSpace(entry.APIKey)
	entry.Note = strings.TrimSpace(entry.Note)
	entry.ID = strings.TrimSpace(entry.ID)
	var saved *KeychainEntry
	if entry.ID != "" {
		found := false
		for i, e := range entries {
			if e.ID == entry.ID {
				entries[i].Name = entry.Name
				entries[i].APIKey = entry.APIKey
				entries[i].Note = entry.Note
				entries[i].UpdatedAt = now
				saved = &entries[i]
				found = true
				break
			}
		}
		if !found {
			return false, "要更新的钥匙串条目已不存在，请刷新后重试", nil
		}
	} else {
		entry.ID = uuid.NewString()
		entry.CreatedAt = now
		entry.UpdatedAt = now
		entries = append(entries, entry)
		saved = &entries[len(entries)-1]
	}
	if len(entries) > MaxKeychainEntries {
		return false, fmt.Sprintf("钥匙串条目数量超出上限（%d）", MaxKeychainEntries), nil
	}
	if !saveKeychainUnlocked(path, entries) {
		return false, "钥匙串保存失败", nil
	}
	return true, "", saved
}

func DeleteKeychainEntries(path string, ids []string) (bool, string, []string, []KeychainEntry) {
	if path == "" {
		path = KeychainPath()
	}
	entries, valid, _ := loadKeychainUnlocked(path)
	if !valid {
		if _, err := os.Stat(path); err == nil {
			return false, "钥匙串文件结构损坏，请先备份或删除该文件后重试", nil, nil
		}
		return true, "", nil, nil
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	var kept []KeychainEntry
	var deleted []string
	for _, e := range entries {
		if idSet[e.ID] {
			deleted = append(deleted, e.ID)
		} else {
			kept = append(kept, e)
		}
	}
	if len(deleted) == 0 {
		return true, "", nil, kept
	}
	if !saveKeychainUnlocked(path, kept) {
		return false, "钥匙串保存失败", nil, entries
	}
	return true, "", deleted, kept
}

func GetKeychainEntry(path, entryID string) *KeychainEntry {
	if path == "" {
		path = KeychainPath()
	}
	entries, valid, _ := loadKeychainUnlocked(path)
	if !valid {
		return nil
	}
	for _, e := range entries {
		if e.ID == entryID {
			c := e
			return &c
		}
	}
	return nil
}
