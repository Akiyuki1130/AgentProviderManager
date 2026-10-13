package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// apm-backup v1：备份信封的格式常量与加解密。
// sealed 用内置种子派生密钥：任何设备上的本软件都能解密，防的是把备份当明文直读，
// 种子随程序发布，不抵御有意提取种子的破解；password 是 PBKDF2-SHA256 + AES-256-GCM。
const (
	BackupFormat  = "apm-backup"
	BackupVersion = 1

	backupPBKDF2Iters = 600000
	backupKeyBytes    = 32

	backupSealedSeed = "8B7OPc4LUtISyQOZWdvCt53ICX+NMpqtZUie797gM2/zEDJLIgFqDTw0FFCIs/zHwvBTZ+fcfNhiOamWnZi7HQ=="
)

// BackupEncBlock 描述被加密的 Providers 载荷；plain 模式时信封里没有它。
type BackupEncBlock struct {
	Algo  string `json:"algo"`
	KDF   string `json:"kdf,omitempty"`
	Salt  string `json:"salt,omitempty"`
	Iters int    `json:"iters,omitempty"`
	Nonce string `json:"nonce,omitempty"`
	Data  string `json:"data"`
}

// BackupPackage 是备份文件的完整信封。KeyMode：plain|sealed|password。
type BackupPackage struct {
	Format        string          `json:"format"`
	Version       int             `json:"version"`
	CreatedAt     string          `json:"created_at"`
	AppVersion    string          `json:"app_version"`
	Agent         string          `json:"agent"`
	IncludeKeys   bool            `json:"include_keys"`
	KeyMode       string          `json:"key_mode"`
	ProviderCount int             `json:"provider_count"`
	Providers     []ProviderEdit  `json:"providers,omitempty"`
	Enc           *BackupEncBlock `json:"enc,omitempty"`
}

var (
	ErrBackupNeedsPassword = errors.New("该备份需要密码")
	ErrBackupWrongPassword = errors.New("密码错误或备份内容已损坏")
)

// AgentLabelFor 导出 agentLabelFor，供备份预览展示 Agent 名称。
func AgentLabelFor(agentID string) string { return agentLabelFor(agentID) }

// StripBackupKeys 返回去掉全部凭据的副本：清空 APIKey、RawProvider，
// OptionsJSON 可解析时只删除 apiKey/api_key 字段，无法解析时整个字段丢弃（宁缺勿漏）。
func StripBackupKeys(p []ProviderEdit) []ProviderEdit {
	out := make([]ProviderEdit, 0, len(p))
	for _, e := range p {
		e.APIKey = ""
		e.RawProvider = nil
		if e.OptionsJSON != "" {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(e.OptionsJSON), &m); err == nil {
				delete(m, "apiKey")
				delete(m, "api_key")
				if b, err := json.Marshal(m); err == nil {
					e.OptionsJSON = string(b)
				}
			} else {
				e.OptionsJSON = ""
			}
		}
		out = append(out, e)
	}
	return out
}

// SealBackup 按 pkg.KeyMode 生成要写入备份文件的字节。
// 加密方式只包裹 Providers 数组，信封其余部分保持明文；password 仅在派生期间存在，
// 派生出的密钥用完立即清零。
func SealBackup(pkg BackupPackage, password string) ([]byte, error) {
	pkg.Format = BackupFormat
	pkg.Version = BackupVersion
	switch pkg.KeyMode {
	case "plain":
		return json.Marshal(pkg)
	case "sealed", "password":
		payload, err := json.Marshal(pkg.Providers)
		if err != nil {
			return nil, err
		}
		var key []byte
		enc := BackupEncBlock{Algo: "aes-256-gcm"}
		if pkg.KeyMode == "password" {
			if password == "" {
				return nil, ErrBackupNeedsPassword
			}
			salt := make([]byte, 16)
			if _, err := rand.Read(salt); err != nil {
				return nil, err
			}
			key, err = pbkdf2.Key(sha256.New, password, salt, backupPBKDF2Iters, backupKeyBytes)
			if err != nil {
				return nil, err
			}
			enc.KDF = "pbkdf2-sha256"
			enc.Salt = base64.StdEncoding.EncodeToString(salt)
			enc.Iters = backupPBKDF2Iters
		} else {
			seedKey := sha256.Sum256([]byte(backupSealedSeed))
			key = seedKey[:]
			enc.KDF = "sha256-seed"
		}
		defer clear(key)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, err
		}
		enc.Nonce = base64.StdEncoding.EncodeToString(nonce)
		enc.Data = base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, payload, nil))
		pkg.Providers = nil
		pkg.Enc = &enc
		return json.Marshal(pkg)
	default:
		return nil, fmt.Errorf("不支持的备份加密方式：%s", pkg.KeyMode)
	}
}

// OpenBackup 解析信封并解密出 Providers。
// sealed 用内置种子派生密钥、无需密码即可在任意设备上的本软件解密；password 需要密码。
func OpenBackup(raw []byte, password string) (*BackupPackage, error) {
	var pkg BackupPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("备份文件解析失败：%v", err)
	}
	if pkg.Format != BackupFormat || pkg.Version != BackupVersion {
		return nil, errors.New("不是受支持的 apm-backup v1 备份文件")
	}
	if pkg.Enc == nil {
		return &pkg, nil
	}
	enc := pkg.Enc
	if enc.Algo != "aes-256-gcm" {
		return nil, fmt.Errorf("不支持的备份加密算法：%s", enc.Algo)
	}
	var key []byte
	switch enc.KDF {
	case "sha256-seed":
		seedKey := sha256.Sum256([]byte(backupSealedSeed))
		key = seedKey[:]
	case "pbkdf2-sha256":
		if enc.Iters <= 0 {
			return nil, ErrBackupWrongPassword
		}
		if password == "" {
			return nil, ErrBackupNeedsPassword
		}
		salt, err := base64.StdEncoding.DecodeString(enc.Salt)
		if err != nil {
			return nil, ErrBackupWrongPassword
		}
		key, err = pbkdf2.Key(sha256.New, password, salt, enc.Iters, backupKeyBytes)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrBackupWrongPassword
	}
	defer clear(key)
	nonce, err1 := base64.StdEncoding.DecodeString(enc.Nonce)
	data, err2 := base64.StdEncoding.DecodeString(enc.Data)
	if err1 != nil || err2 != nil {
		return nil, ErrBackupWrongPassword
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	payload, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, ErrBackupWrongPassword
	}
	var providers []ProviderEdit
	if err := json.Unmarshal(payload, &providers); err != nil {
		return nil, fmt.Errorf("备份内容已损坏：%v", err)
	}
	pkg.Providers = providers
	pkg.Enc = nil
	return &pkg, nil
}
