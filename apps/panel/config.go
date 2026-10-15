package panel

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"b0go/core/gateway"

	"github.com/BurntSushi/toml"
)

const configFileName = "config.ini"

// PanelConfig 面板可编辑项
type PanelConfig struct {
	Port     string `json:"port"`
	Path     string `json:"path"`
	Password string `json:"password"`
	AutoOpen bool   `json:"autoOpen"`
}

type iniFile struct {
	Gateway struct {
		ListenAddr string
		Domain     string
		Live       bool
	} `toml:"gateway"`
	Pass struct {
		Path          string
		CodeReadOnly  string
		CodeReadWrite string
		Live          bool
	} `toml:"pass"`
}

func defaultPanelConfig() PanelConfig {
	return PanelConfig{
		Port:     "8000",
		Path:     "files",
		Password: "admin",
		AutoOpen: true,
	}
}

func loadPanelConfig() PanelConfig {
	cfg := defaultPanelConfig()
	raw, err := os.ReadFile(configFileName)
	if err != nil {
		return cfg
	}
	var ini iniFile
	if _, err := toml.Decode(string(raw), &ini); err != nil {
		return cfg
	}
	if p := gateway.PortOnly(ini.Gateway.ListenAddr); p != "" {
		cfg.Port = p
	}
	if strings.TrimSpace(ini.Pass.Path) != "" {
		cfg.Path = strings.TrimSpace(ini.Pass.Path)
	}
	cfg.Password = ini.Pass.CodeReadWrite
	return cfg
}

func savePanelConfig(c PanelConfig) error {
	port := strings.TrimSpace(c.Port)
	if port == "" {
		port = "8000"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("端口必须是数字")
	}
	path := strings.TrimSpace(c.Path)
	if path == "" {
		path = "files"
	}
	pwd := strings.TrimSpace(c.Password)
	content := fmt.Sprintf(
		"[gateway]\nListenAddr = %q\nDomain = \"\"\nLive = false\n\n[pass]\nPath = %q\nCodeReadOnly = \"\"\nCodeReadWrite = %q\nLive = false\n",
		gateway.NormalizeListenAddr(port),
		path,
		pwd,
	)
	if err := os.WriteFile(configFileName, []byte(content), 0666); err != nil {
		return err
	}
	_ = os.MkdirAll(path, 0666)
	return nil
}
