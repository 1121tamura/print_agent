package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = `C:\ProgramData\PrintAgent\config.yaml`

// Load はYAML設定ファイルを読み込み、必須項目を検証する。
// 環境変数 PRINT_AGENT_CONFIG が設定されていればそのパスを優先する。
func Load() (*Config, error) {
	path := os.Getenv("PRINT_AGENT_CONFIG")
	if path == "" {
		path = defaultConfigPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found (%s): %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config parse error: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	cfg.path = path
	return &cfg, nil
}

// WriteAgentID は agent.id を config.yaml に書き込む。
// 初回登録時に Backend から取得した UUID を永続化するために使用する。
func (c *Config) WriteAgentID(id string) error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("config read error: %w", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config parse error: %w", err)
	}

	agent, ok := raw["agent"].(map[string]any)
	if !ok {
		agent = map[string]any{}
		raw["agent"] = agent
	}
	agent["id"] = id

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("config marshal error: %w", err)
	}

	if err := os.WriteFile(c.path, out, 0644); err != nil {
		return fmt.Errorf("config write error: %w", err)
	}

	c.Agent.ID = id
	return nil
}

func (c *Config) validate() error {
	required := []struct {
		value string
		name  string
	}{
		// agent.id は空でも起動可（初回起動時に自動登録）
		{c.Backend.BaseURL, "backend.base_url"},
		{c.Redis.Addr, "redis.addr"},
		{c.Redis.StreamName, "redis.stream_name"},
		{c.Redis.ConsumerGroup, "redis.consumer_group"},
		{c.Storage.TempDir, "storage.temp_dir"},
		{c.Storage.LogDir, "storage.log_dir"},
	}

	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("required field missing: %s", r.name)
		}
	}

	return nil
}
