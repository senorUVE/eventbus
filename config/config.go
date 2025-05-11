package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppName         string        `mapstructure:"app_name"`
	AppPort         int           `mapstructure:"app_port"`
	BufferSize      int           `mapstructure:"buffer_size"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	LogFormat       string        `mapstructure:"log_format"`
	LogFile         string        `mapstructure:"log_file"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Config file not found")
		} else {
			return config, fmt.Errorf("failed to read config: %w", err)
		}
	}
	if err := viper.Unmarshal(&config); err != nil {
		return config, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}
