package microrunner

import (
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

var defaultDomains = []string{
	"github.com",
	"api.github.com",
	"*.actions.githubusercontent.com",
	"codeload.github.com",
	"results-receiver.actions.githubusercontent.com",
	"*.blob.core.windows.net",
	"objects.githubusercontent.com",
	"objects-origin.githubusercontent.com",
	"github-releases.githubusercontent.com",
	"github-registry-files.githubusercontent.com",
	"*.pkg.github.com",
	"pkg-containers.githubusercontent.com",
	"ghcr.io",
	"github-cloud.githubusercontent.com",
	"github-cloud.s3.amazonaws.com",
	"dependabot-actions.githubapp.com",
	"release-assets.githubusercontent.com",
	"api.snapcraft.io",
}

func makePolicyRules() []msb.PolicyRule {
	var rules []msb.PolicyRule

	for _, d := range defaultDomains {
		rules = append(rules, msb.PolicyRule{
			Action:      msb.PolicyActionAllow,
			Direction:   msb.PolicyDirectionEgress,
			Destination: strings.TrimPrefix(d, "*"),
			Port:        "443",
		})
	}

	return rules
}
