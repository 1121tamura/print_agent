package config

// Config はアプリケーション全体の設定を保持する。
type Config struct {
	path string // 設定ファイルのパス（WriteAgentID で使用）
	Agent    AgentConfig    `yaml:"agent"`
	Backend  BackendConfig  `yaml:"backend"`
	Redis    RedisConfig    `yaml:"redis"`
	LocalAPI LocalAPIConfig `yaml:"local_api"`
	Storage  StorageConfig  `yaml:"storage"`
	Print    PrintConfig    `yaml:"print"`
}

type AgentConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type BackendConfig struct {
	BaseURL    string `yaml:"base_url"`
	TimeoutSec int    `yaml:"timeout_sec"`
	APIKey     string `yaml:"api_key"`
}

type RedisConfig struct {
	Addr          string `yaml:"addr"`
	Password      string `yaml:"password"`
	DB            int    `yaml:"db"`
	StreamName    string `yaml:"stream_name"`
	ConsumerGroup string `yaml:"consumer_group"`
	// ConsumerName は agent.id を使用するため設定不要
}

type LocalAPIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type StorageConfig struct {
	TempDir string `yaml:"temp_dir"`
	LogDir  string `yaml:"log_dir"`
}

type PrintConfig struct {
	DeleteTempAfterPrint bool `yaml:"delete_temp_after_print"`
	CleanupOnStartup     bool `yaml:"cleanup_on_startup"`
}
