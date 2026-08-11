package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bytedance/easy-cli/internal/projectroot"
)

const (
	configDirectory = "easy-cli"
	configFileName  = "config.json"
)

var (
	ErrKeyUnavailable = errors.New("configuration key is not available")
	ErrKeyNotSet      = errors.New("configuration key is not set")
)

type LoadOptions struct {
	WorkingDir string
	HomeDir    string
}

type Config struct {
	MySQL MySQL
	SMB   SMB
}

type MySQL struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

type SMB struct {
	BackendRepo  string
	FrontendRepo string
	IDLRepo      string
}

type source struct {
	MySQL *mySQLSource `json:"mysql"`
	SMB   *smbSource   `json:"smb"`
}

type mySQLSource struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	User     *string `json:"user"`
	Password *string `json:"password"`
	Database *string `json:"database"`
}

type smbSource struct {
	BackendRepo  *string `json:"backend_repo"`
	FrontendRepo *string `json:"frontend_repo"`
	IDLRepo      *string `json:"idl_repo"`
}

func Load(options LoadOptions) (Config, error) {
	workingDir := options.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("get working directory: %w", err)
		}
	}
	homeDir := options.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("get home directory: %w", err)
		}
	}

	paths := []string{
		filepath.Join(homeDir, ".config", configDirectory, configFileName),
		filepath.Join(projectroot.Find(workingDir), ".easy-cli", configFileName),
	}
	var config Config
	for _, path := range paths {
		loaded, exists, err := read(path)
		if err != nil {
			return Config{}, err
		}
		if exists {
			config.apply(loaded)
		}
	}
	return config, nil
}

func read(path string) (source, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return source{}, false, nil
	}
	if err != nil {
		return source{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var loaded source
	if err := decoder.Decode(&loaded); err != nil {
		return source{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return source{}, false, fmt.Errorf("parse config %s: multiple JSON values", path)
		}
		return source{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := loaded.validate(); err != nil {
		return source{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	return loaded, true, nil
}

func (s source) validate() error {
	if s.MySQL != nil && s.MySQL.Port != nil && (*s.MySQL.Port < 1 || *s.MySQL.Port > 65535) {
		return fmt.Errorf("mysql.port must be between 1 and 65535")
	}
	return nil
}

func (c *Config) apply(source source) {
	if source.MySQL != nil {
		if source.MySQL.Host != nil {
			c.MySQL.Host = *source.MySQL.Host
		}
		if source.MySQL.Port != nil {
			c.MySQL.Port = *source.MySQL.Port
		}
		if source.MySQL.User != nil {
			c.MySQL.User = *source.MySQL.User
		}
		if source.MySQL.Password != nil {
			c.MySQL.Password = *source.MySQL.Password
		}
		if source.MySQL.Database != nil {
			c.MySQL.Database = *source.MySQL.Database
		}
	}
	if source.SMB != nil {
		if source.SMB.BackendRepo != nil {
			c.SMB.BackendRepo = *source.SMB.BackendRepo
		}
		if source.SMB.FrontendRepo != nil {
			c.SMB.FrontendRepo = *source.SMB.FrontendRepo
		}
		if source.SMB.IDLRepo != nil {
			c.SMB.IDLRepo = *source.SMB.IDLRepo
		}
	}
}

func (c Config) Get(key string) (string, error) {
	var value string
	switch key {
	case "mysql.host":
		value = c.MySQL.Host
	case "mysql.port":
		if c.MySQL.Port != 0 {
			value = strconv.Itoa(c.MySQL.Port)
		}
	case "mysql.user":
		value = c.MySQL.User
	case "mysql.database":
		value = c.MySQL.Database
	case "smb.backend-repo":
		value = c.SMB.BackendRepo
	case "smb.frontend-repo":
		value = c.SMB.FrontendRepo
	case "smb.idl-repo":
		value = c.SMB.IDLRepo
	default:
		return "", fmt.Errorf("%w: %q", ErrKeyUnavailable, key)
	}
	if value == "" {
		return "", fmt.Errorf("%w: %q", ErrKeyNotSet, key)
	}
	return value, nil
}
