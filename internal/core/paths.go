package core

import (
	"os"
	"path/filepath"
)

func ZCodeDir() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".zcode", "v2")
}

func ZCodeConfig() string {
	return filepath.Join(ZCodeDir(), "config.json")
}

func AppDataDir() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home
	}
	return filepath.Join(dir, "AgentProviderManager")
}

func LegacyAppDataDir() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home
	}
	return filepath.Join(dir, "ZCodeProviderManager")
}

func OpencodeConfig() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func KeychainPath() string {
	return filepath.Join(AppDataDir(), KeychainFilename)
}

func SettingsPath() string {
	return filepath.Join(AppDataDir(), "settings.json")
}

func DefaultConfigPath() string {
	return ZCodeConfig()
}
