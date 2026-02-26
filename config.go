package main

import (
	"fmt"
	"log"
	"os"
	"gopkg.in/yaml.v3"
)

//reverse proxy config
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Routes  []RouteConfig `yaml:"routes"`
	ApiKeys []string
	LLMConfig LLMConfig `yaml:"llm"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type RouteConfig struct {
	Path   string `yaml:"path"`
	Target []string `yaml:"target"`
}


type LLMConfig struct {
	Classifier ClassifierConfig `yaml:"classifier"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Tiers map[string]TierConfig `yaml:"tiers"`
}

type ClassifierConfig struct {
	Provider string `yaml:"provider"`
	Model string `yaml:"model"`
}

type ProviderConfig struct {
	Type string `yaml:"type"`
	BaseURL string `yaml:"base_url"`
	ApiKeyEnv string `yaml:"api_key_env"`
	Models []string `yaml:"models"`
}

type TierConfig struct{
	Provider string `yaml:"provider"`
	Model string `yaml:"model"`
}

func loadConfig(path string, apiKeys []string) (Config, error) {
	content, err := os.ReadFile(path)

	if err != nil {
		return Config{}, fmt.Errorf("can not read file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(content, &config)

	if err != nil {
		return Config{}, fmt.Errorf("can not parse config: %w", err)
	}
	config.ApiKeys = apiKeys

	log.Println("config file is loaded successfully")
	return config, nil
}