package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	ServerUrl   string `koanf:"serverUrl"`
	Username    string `koanf:"username"`
	Password    string `koanf:"password"`
	CustomCa    string `koanf:"customCa"`
	LogLevelStr string `koanf:"logLevel"`
	LogLevel    slog.Level
}

func Load(configPath string) (*Config, error) {
	var k = koanf.New(".")
	defConfig := Config{
		ServerUrl:   "",
		Username:    "",
		Password:    "",
		CustomCa:    "",
		LogLevelStr: "INFO",
	}
	k.Load(structs.Provider(defConfig, "koanf"), nil)
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		return nil, err
	}

	var config Config
	err := k.UnmarshalWithConf("", &config, koanf.UnmarshalConf{FlatPaths: true})
	if err != nil {
		return nil, err
	}

	if config.ServerUrl == "" {
		return &config, fmt.Errorf("missing attribute ServerUrl")
	}

	switch strings.Trim(strings.ToLower(config.LogLevelStr), " ") {
	case "debug":
		config.LogLevel = slog.LevelDebug
	case "info":
		config.LogLevel = slog.LevelInfo
	case "warn":
		config.LogLevel = slog.LevelWarn
	case "error":
		config.LogLevel = slog.LevelError
	}

	return &config, nil
}
