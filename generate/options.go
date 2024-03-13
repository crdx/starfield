package generate

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/samber/mo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

type Options struct {
	Out       string            `json:"out" yaml:"out"`
	Package   string            `json:"package" yaml:"package"`
	Rename    map[string]string `json:"rename,omitempty" yaml:"rename"`
	Preserve  []string          `json:"preserve,omitempty" yaml:"preserve"`
	MaxParams mo.Option[int]    `json:"max_params,omitempty" yaml:"max_params"`
}

func parseOptions(req *plugin.GenerateRequest) (*Options, error) {
	var options Options

	if len(req.PluginOptions) == 0 {
		return &options, nil
	}

	if err := json.Unmarshal(req.PluginOptions, &options); err != nil {
		return nil, fmt.Errorf("unmarshalling plugin options: %w", err)
	}

	if options.Out == "" {
		options.Out = req.Settings.Codegen.Out
	}

	if options.Package == "" {
		options.Package = filepath.Base(options.Out)
	}

	if options.MaxParams.IsAbsent() {
		options.MaxParams = mo.Some[int](3)
	}

	return &options, nil
}
