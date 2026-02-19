package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Routes  []RouteConfig  `yaml:"routes"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type RouteConfig struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target"`
}

func loadConfig(path string) (Config, error) {
	content, error := os.ReadFile(path)

	if error != nil {
		return Config{}, fmt.Errorf("can not read file: %w", error)
	}

	var config Config
	error = yaml.Unmarshal(content, &config)

	if error != nil {
		return Config{}, fmt.Errorf("can not parse config: %w", error)
	}

	log.Println("config file is loaded successfully")
	return config, nil
}
