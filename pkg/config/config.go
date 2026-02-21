package config

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	ServerUrl string `koanf:"serverUrl"`
	Username  string `koanf:"username"`
	Password  string `koanf:"password"`
	CustomCa  string `koanf:"customCa"`
}

func Load(configPath string) (*Config, error) {
	var k = koanf.New(".")
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		return nil, err
	}

	var config Config
	err := k.UnmarshalWithConf("", &config, koanf.UnmarshalConf{FlatPaths: true})
	if err != nil {
		return nil, err
	}

	return &config, nil
}
