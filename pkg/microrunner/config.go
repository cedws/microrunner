package microrunner

import (
	"fmt"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

type LabelMatrix struct {
	CPU       []int `json:"cpu" kong:"name='cpu-matrix',default='2,4'" default:"[2,4]"`
	MemoryMiB []int `json:"memoryMiB" kong:"name='memory-matrix',default='1,2'" default:"[1,2]"`
}

type Config struct {
	Debug           bool         `json:"debug" kong:"-"`
	Egress          EgressConfig `json:"egress" kong:"-"`
	GitHubConfigURL string       `json:"githubConfigURL" name:"github-config-url" required:"true"`
	GitHubToken     string       `json:"githubToken" kong:"-"`
	Image           string       `json:"image" required:"true"`
	LabelMatrix     LabelMatrix  `json:"labelMatrix" embed:""`
	MaxRunners      int          `json:"maxRunners" default:"10"`
	MetricsAddr     string       `json:"metricsAddr" help:"Listen address to serve prometheus metrics on."`
	Prefix          string       `json:"prefix" default:"microrunner"`
}

type EgressConfig struct {
	Rules           []EgressRule `json:"rules"`
	UseDefaultRules *bool        `json:"useDefaultRules" default:"true"`
}

type EgressRule struct {
	Action          msb.PolicyAction    `json:"action"`
	Destination     string              `json:"destination"`
	PolicyDirection msb.PolicyDirection `json:"policyDirection"`
	Port            string              `json:"port"`
}

func (c Config) Validate() error {
	if c.Prefix == "" {
		return fmt.Errorf("prefix is required")
	}
	return nil
}
