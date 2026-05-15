package config

type Config struct {
	Port           int
	HTTPSPort      int
	BindAddr       string
	AllowedOrigins []string
	DetectInterval int
	LogToFile      bool
	LogPath        string
	CertDir        string
}

func Default() *Config {
	return &Config{
		Port:           19100,
		HTTPSPort:      19101,
		BindAddr:       "127.0.0.1",
		AllowedOrigins: []string{"*"},
		DetectInterval: 5,
		LogToFile:      true,
		LogPath:        defaultLogPath(),
		CertDir:        defaultCertDir(),
	}
}
