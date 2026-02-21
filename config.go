package main

import (
	"fmt"
	"log"
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Routes  []RouteConfig `yaml:"routes"`
	ApiKeys []string
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type RouteConfig struct {
	Path   string `yaml:"path"`
	Target []string `yaml:"target"`
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
