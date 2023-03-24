package taxfiller

import (
	"github.com/spf13/viper"
)

type ChainConfig struct {
	SkipAddress string              `mapstructure:"skip_address"`
	Token       string              `mapstructure:"token"`
	RedisAddr   string              `mapstructure:"redis_address"`
	DBHost      string              `mapstructure:"db_host"`
	DBPassword  string              `mapstructure:"db_password"`
	NodeRPC     string              `mapstructure:"node_address"`
	TestAccts   []string		    `mapstructure:"test_accts"`
	Threads     int                 `mapstructure:"num_threads"`
}

type Config map[string]ChainConfig

func FillConfig(home, password string) (*Config, error) {
	// set config name
	viper.SetConfigName("taxes")
	viper.SetConfigType("toml")
	viper.AddConfigPath(home)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	dbConfig := make(Config)
	if err := viper.Unmarshal(&dbConfig); err != nil {
		return nil, err
	}
	// check if pgPassWord was given, if so, override
	if password != "" {
		for _, config := range dbConfig {
			config.DBPassword = password
		}
	}

	return &dbConfig, nil
}
