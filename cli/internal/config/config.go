package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all agctl configuration
type Config struct {
	Clusters     ClustersConfig              `yaml:"clusters"`
	Environments map[string]EnvironmentConfig `yaml:"environments"`
	Gateway      GatewayConfig               `yaml:"gateway"`
	Defaults     DefaultsConfig              `yaml:"defaults"`
	Gitea        GiteaConfig                 `yaml:"gitea"`
}

type ClustersConfig struct {
	ControlPlane ClusterConfig `yaml:"control-plane"`
	DataPlane    ClusterConfig `yaml:"data-plane"`
	DNS          DNSConfig     `yaml:"dns"`
}

type ClusterConfig struct {
	Kubeconfig string `yaml:"kubeconfig"`
	Host       string `yaml:"host"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
}

type DNSConfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	APIKey   string `yaml:"apiKey"`
	Zone     string `yaml:"zone"`
}

type EnvironmentConfig struct {
	Namespace    string `yaml:"namespace"`
	StorageClass string `yaml:"storageClass"`
	Short        string `yaml:"short"`
}

type GatewayConfig struct {
	Name         string `yaml:"name"`
	Namespace    string `yaml:"namespace"`
	IP           string `yaml:"ip"`
	MaxListeners int    `yaml:"maxListeners"`
	TLSIssuer    string `yaml:"tlsIssuer"`
}

type DefaultsConfig struct {
	Project string `yaml:"project"`
	Domain  string `yaml:"domain"`
}

type GiteaConfig struct {
	URL      string `yaml:"url"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
}

// Load reads config from file, falling back to default locations
func Load(path string) (*Config, error) {
	if path == "" {
		// Try default locations in order
		candidates := []string{
			"/etc/agctl/config.yaml",
			filepath.Join(os.Getenv("HOME"), ".agctl", "config.yaml"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
		if path == "" {
			return nil, fmt.Errorf("no config file found. Create /etc/agctl/config.yaml or ~/.agctl/config.yaml")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Validate required fields
	if cfg.Clusters.DataPlane.Kubeconfig == "" {
		return nil, fmt.Errorf("clusters.data-plane.kubeconfig is required")
	}
	if len(cfg.Environments) == 0 {
		return nil, fmt.Errorf("at least one environment must be configured")
	}

	return &cfg, nil
}

// ResolveEnv returns the environment config for a given name, or error
func (c *Config) ResolveEnv(name string) (*EnvironmentConfig, error) {
	env, ok := c.Environments[name]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q. Available: %v", name, c.EnvNames())
	}
	return &env, nil
}

// ResolveNamespace returns the data plane namespace for a project+environment combo.
// For the "default" project, it uses the namespace from config directly.
// For other projects, it discovers the namespace dynamically via kubectl.
func (c *Config) ResolveNamespace(project, envName string) (string, error) {
	env, err := c.ResolveEnv(envName)
	if err != nil {
		return "", err
	}

	// For default project, use the pre-configured namespace
	if project == "" || project == c.Defaults.Project {
		return env.Namespace, nil
	}

	// For other projects, derive from the pattern:
	// dp-{project}-default-{envName}-{hash}
	// We'll discover it dynamically at runtime via kubectl
	return fmt.Sprintf("PROJECT_DISCOVER:%s:%s", project, envName), nil
}

// EnvNames returns all configured environment names
func (c *Config) EnvNames() []string {
	names := make([]string, 0, len(c.Environments))
	for k := range c.Environments {
		names = append(names, k)
	}
	return names
}

// DataPlaneKubeconfig returns the kubeconfig flag for kubectl
func (c *Config) DataPlaneKubeconfig() string {
	return c.Clusters.DataPlane.Kubeconfig
}
