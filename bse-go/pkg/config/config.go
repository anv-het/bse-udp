package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Multicast  MulticastConfig `json:"multicast"`
	BufferSize int             `json:"buffer_size"`
	Timeout    int             `json:"timeout"`
	StoreLimit int             `json:"store_limit"`
}

type MulticastConfig struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Segment string `json:"segment"`
	Env     string `json:"env"`
}

func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &config, nil
}
