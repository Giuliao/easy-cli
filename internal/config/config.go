package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Giuliao/easy-cli/internal/projectroot"
)

const (
	configDirectory = "easy-cli"
	configFileName  = "config.json"
)

var (
	ErrKeyUnavailable = errors.New("configuration key is not available")
	ErrKeyNotSet      = errors.New("configuration key is not set")
	ErrConfigExists   = errors.New("configuration file already exists")
)

const homeConfigTemplate = `{
  "mysql": {
    "host": "",
    "port": 3306,
    "user": "",
    "password": "",
    "database": ""
  }
}
`

type LoadOptions struct {
	WorkingDir string
	HomeDir    string
}

type InitOptions struct {
	HomeDir string
	Force   bool
}

type InitResult struct {
	Path    string
	Changed bool
}

type Config struct {
	MySQL MySQL
}

type MySQL struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

type source struct {
	MySQL *mySQLSource `json:"mysql"`
}

type mySQLSource struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	User     *string `json:"user"`
	Password *string `json:"password"`
	Database *string `json:"database"`
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

func InitHome(options InitOptions) (InitResult, error) {
	homeDir := options.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return InitResult{}, fmt.Errorf("get home directory: %w", err)
		}
	}
	target := filepath.Join(homeDir, ".config", configDirectory, configFileName)
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return InitResult{}, fmt.Errorf("create configuration directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return InitResult{}, fmt.Errorf("set configuration directory permissions: %w", err)
	}
	if options.Force {
		if err := writeConfigAtomically(target, homeConfigTemplate); err != nil {
			return InitResult{}, err
		}
		return InitResult{Path: target, Changed: true}, nil
	}
	if err := writeNewConfig(target, homeConfigTemplate); err != nil {
		if errors.Is(err, os.ErrExist) {
			return InitResult{}, fmt.Errorf("%w: %s; run easy config init --force to replace it", ErrConfigExists, target)
		}
		return InitResult{}, err
	}
	return InitResult{Path: target, Changed: true}, nil
}

func writeNewConfig(path, contents string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return fmt.Errorf("set configuration file permissions: %w", err)
	}
	if _, err := file.WriteString(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return fmt.Errorf("sync configuration file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	return nil
}

func writeConfigAtomically(path, contents string) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".config.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary configuration file permissions: %w", err)
	}
	if _, err := temporary.WriteString(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary configuration file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary configuration file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace configuration file: %w", err)
	}
	return nil
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
	default:
		return "", fmt.Errorf("%w: %q", ErrKeyUnavailable, key)
	}
	if value == "" {
		return "", fmt.Errorf("%w: %q", ErrKeyNotSet, key)
	}
	return value, nil
}
