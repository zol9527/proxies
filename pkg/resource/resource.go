// Author       :loyd
// Date         :2025-03-08 00:31:43
// LastEditors  :loyd
// LastEditTime :2025-03-08 00:31:51
// Description  :用于处理和加载代理源配置

package resource

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config 定义顶层配置结构
type Config struct {
	Pool      PoolConfig       `toml:"pool"`
	Platforms []PlatformConfig `toml:"platform"`
}

// PoolConfig 定义池配置
type PoolConfig struct {
	Port       int    `toml:"port"`
	Cron       string `toml:"cron"`
	VerifyTime int    `toml:"verifyTime"`
	Debug      bool   `toml:"debug"`
}

// PlatformConfig 定义代理平台配置
type PlatformConfig struct {
	Name   string   `toml:"name"`
	Method string   `toml:"method"`
	URLs   []string `toml:"urls"`
	Proxy  bool     `toml:"proxy"`
}

// LoadConfig 从指定路径加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	var config Config

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	err = toml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return &config, nil
}

// DefaultConfigPath 返回默认配置文件路径
func DefaultConfigPath() (string, error) {
	// 尝试找到配置文件的常见位置
	locations := []string{
		"./config/proxy-sources.toml",
		"../config/proxy-sources.toml",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			absPath, err := filepath.Abs(loc)
			if err != nil {
				return "", err
			}
			return absPath, nil
		}
	}

	return "", fmt.Errorf("configuration file not found in known locations")
}
