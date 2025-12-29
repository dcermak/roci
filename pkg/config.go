package roci

import (
	"os"

	"gopkg.in/yaml.v3"
)

// RpmPreamble are the preamble fields from the spec that are relevant for roci
type RpmPreamble struct {
	Name          string `yaml:"Name"`
	Version       string `yaml:"Version"`
	Release       string `yaml:"Release"`
	Epoch         int    `yaml:"Epoch"`
	License       string `yaml:"License"`
	SourceLicense string `yaml:"SourceLicense"`
	Group         string `yaml:"Group"`
	Summary       string `yaml:"Summary"`

	//
	// Source              []string `yaml:"Source"`
	// Patch               []string `yaml:"Patch"`
	Icon string `yaml:"Icon"`

	// we don't want these two, it was not really used anyway
	// NoSource
	// NoPatch

	URL    string `yaml:"URL"`
	BugURL string `yaml:"BugURL"`

	// please no 😨
	// ModularityLabel     string   `yaml:"ModularityLabel"`

	DistTag      string `yaml:"DistTag"`
	VCS          string `yaml:"VCS"`
	Distribution string `yaml:"Distribution"`
	Vendor       string `yaml:"Vendor"`
	Packager     string `yaml:"Packager"`

	// unused:
	// BuildRoot    string `yaml:"BuildRoot"`

	// not really applicable here:
	// Buildsystem  string `yaml:"Buildsystem"`

	// eh, let's just keep this on
	// AutoReqProv         string   `yaml:"AutoReqProv"`
	// AutoReq             string   `yaml:"AutoReq"`
	// AutoProv            string   `yaml:"AutoProv"`

	// Requires dependencies:
	RequiresPre       []string `yaml:"Requires(pre)"`
	RequiresPost      []string `yaml:"Requires(post)"`
	RequiresPreUn     []string `yaml:"Requires(preun)"`
	RequiresPostUn    []string `yaml:"Requires(postun)"`
	RequiresPreTrans  []string `yaml:"Requires(pretrans)"`
	RequiresPostTrans []string `yaml:"Requires(posttrans)"`
	RequiresVerify    []string `yaml:"Requires(verify)"`
	RequiresInterp    []string `yaml:"Requires(interp)"`
	RequiresMeta      []string `yaml:"Requires(meta)"`
	Requires          []string `yaml:"Requires"`

	// remaining dependencies
	Provides          []string `yaml:"Provides"`
	Conflicts         []string `yaml:"Conflicts"`
	Obsoletes         []string `yaml:"Obsoletes"`
	Recommends        []string `yaml:"Recommends"`
	Suggests          []string `yaml:"Suggests"`
	Supplements       []string `yaml:"Supplements"`
	Enhances          []string `yaml:"Enhances"`
	OrderWithRequires []string `yaml:"OrderWithRequires"`

	// deprecated:
	// Prereq            []string `yaml:"Prereq"`
	// BuildPrereq       []string `yaml:"BuildPrereq"`

	// not applicable:
	// BuildRequires  []string `yaml:"BuildRequires"`
	// BuildConflicts []string `yaml:"BuildConflicts"`

	ExcludeArch   []string `yaml:"ExcludeArch"`
	ExclusiveArch []string `yaml:"ExclusiveArch"`
	ExcludeOS     []string `yaml:"ExcludeOS"`
	ExclusiveOS   []string `yaml:"ExclusiveOS"`
	BuildArch     string   `yaml:"BuildArch"`
	Prefixes      []string `yaml:"Prefixes"`
	DocDir        string   `yaml:"DocDir"`

	// not applicable:
	// RemovePathPostfixes string   `yaml:"RemovePathPostfixes"`
}

// type RpmScriptlet struct {
// 	Options []string `yaml:"Options"`
// 	Name    string   `yaml:"Name"`
// }

type RpmPackage struct {
	RpmPreamble `yaml:",inline"`

	Description string `yaml:"Description"`

	Postin       string `yaml:"Postin"`
	Posttrans    string `yaml:"Posttrans"`
	Postun       string `yaml:"Postun"`
	Prein        string `yaml:"Prein"`
	Pretrans     string `yaml:"Pretrans"`
	Preun        string `yaml:"Preun"`
	VerifyScript string `yaml:"VerifyScript"`
}

// Config represents the roci configuration file
type Config struct {
	RpmPackage `yaml:",inline"`

	Package map[string]RpmPackage `yaml:"package"`
}

// LoadConfig reads and parses the YAML configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
