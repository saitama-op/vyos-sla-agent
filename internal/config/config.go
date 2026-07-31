package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Threshold struct {
	Latency        float64 `yaml:"latency"`
	LatencySamples int     `yaml:"latency_samples"` // Mapped from your config spec
	Loss           float64 `yaml:"loss"`
	LossSamples    int     `yaml:"loss_samples"`
	Jitter         float64 `yaml:"jitter"`
	JitterSamples  int     `yaml:"jitter_samples"`
}

type WAN struct {
	Name      string    `yaml:"name"`
	Interface string    `yaml:"interface"`
	Targets   []string  `yaml:"targets"`
	Threshold Threshold `yaml:"threshold"`
	OnDown    []string  `yaml:"on_down"` // Commands to execute when SLA fails
	OnUp      []string  `yaml:"on_up"` // Commands to execute when SLA recovers
}

type Config struct {
	Interval time.Duration `yaml:"interval"`
	WANs     []WAN         `yaml:"wans"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
