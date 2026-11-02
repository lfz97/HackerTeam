package config

type Observability struct {
	Service  string `yaml:"service"`
	Host     string `yaml:"host"`
	Public   string `yaml:"public"`
	Secret   string `yaml:"secret"`
	Insecure bool   `yaml:"insecure"`
	Enabled  bool   `yaml:"enabled"`
}
