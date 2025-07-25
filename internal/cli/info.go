// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"

	"github.com/hashicorp/nomad-pack/internal/pkg/cache"
	"github.com/hashicorp/nomad-pack/internal/pkg/flag"
	"github.com/hashicorp/nomad-pack/internal/pkg/loader"
	"github.com/hashicorp/nomad-pack/internal/pkg/variable/parser"
	"github.com/hashicorp/nomad-pack/internal/pkg/variable/parser/config"
	"github.com/mitchellh/go-glint"
	"github.com/zclconf/go-cty/cty"
)

type InfoCommand struct {
	*baseCommand
	packConfig *cache.PackConfig
}

func (c *InfoCommand) Run(args []string) int {
	c.cmdKey = "info" // Add cmdKey here to print out helpUsageMessage on Init error
	// Initialize. If we fail, we just exit since Init handles the UI.
	if err := c.Init(
		WithExactArgs(1, args),
		WithFlags(c.Flags()),
		WithNoConfig(),
	); err != nil {
		c.ui.ErrorWithContext(err, ErrParsingArgsOrFlags)
		c.ui.Info(c.helpUsageMessage())
		return 1
	}

	c.packConfig.Name = c.args[0]

	// Set the packConfig defaults if necessary and generate our UI error context.
	errorContext := initPackCommand(c.packConfig)

	// verify packs exist before running jobs
	if err := cache.VerifyPackExists(c.packConfig, errorContext, c.ui); err != nil {
		return 1
	}

	packPath := c.packConfig.Path

	p, err := loader.Load(packPath)
	if err != nil {
		c.ui.ErrorWithContext(err, "failed to load pack from local directory", errorContext.GetAll()...)
		return 1
	}

	variableParser, err := parser.NewParser(&config.ParserConfig{
		ParentPack:        p,
		RootVariableFiles: p.RootVariableFiles(),
		IgnoreMissingVars: c.ignoreMissingVars,
	})
	if err != nil {
		return 1
	}

	parsedVars, diags := variableParser.Parse()
	if diags != nil && diags.HasErrors() {
		c.ui.Info(diags.Error())
		return 1
	}

	// Create a new glint document to handle the outputting of information.
	doc := glint.New()

	doc.Append(glint.Layout(
		glint.Style(glint.Text("Pack Name          "), glint.Bold()),
		glint.Text(p.Metadata.Pack.Name),
	).Row())

	doc.Append(glint.Layout(
		glint.Style(glint.Text("Description        "), glint.Bold()),
		glint.Text(p.Metadata.Pack.Description),
	).Row())

	doc.Append(glint.Layout(
		glint.Style(glint.Text("Application URL    "), glint.Bold()),
		glint.Text(p.Metadata.App.URL),
	).Row())

	for pName, variables := range parsedVars.GetVars() {

		doc.Append(glint.Layout(
			glint.Style(glint.Text(fmt.Sprintf("Pack %q Variables:", pName)), glint.Bold()),
		).Row())

		// to output required variables first
		var required []string
		var optional []string

		for _, v := range variables {

			varType := "unknown"
			if !v.Type.Equals(cty.NilType) {
				// check the explicit "type" parameter
				varType = v.Type.FriendlyName()
			} else if !v.Default.IsNull() {
				// or infer from the default
				varType = v.Default.Type().FriendlyName()
			}

			if v.Default.IsNull() {
				required = append(required, fmt.Sprintf("\t- %q (%s: required) - %s", v.Name, varType, v.Description))
			} else {
				optional = append(optional, fmt.Sprintf("\t- %q (%s: optional) - %s", v.Name, varType, v.Description))
			}

		}

		for _, row := range append(required, optional...) {
			doc.Append(glint.Layout(glint.Style(
				glint.Text(row),
			)).Row())
		}
		glint.Text("\n")
	}

	doc.RenderFrame()
	return 0
}

func (c *InfoCommand) Flags() *flag.Sets {
	return c.flagSet(flagSetOperation, func(set *flag.Sets) {
		c.packConfig = &cache.PackConfig{}

		f := set.NewSet("Render Options")

		f.StringVar(&flag.StringVar{
			Name:    "registry",
			Target:  &c.packConfig.Registry,
			Default: "",
			Usage: `Specific registry name containing the pack to retrieve info
					about. If not specified, the default registry will be used.`,
		})

		f.StringVar(&flag.StringVar{
			Name:    "ref",
			Target:  &c.packConfig.Ref,
			Default: "",
			Usage: `Specific git ref of the pack to retrieve info about.
					Supports tags, SHA, and latest. If no ref is specified,
					defaults to latest.

					Using ref with a file path is not supported.`,
		})
	})
}

func (c *InfoCommand) Help() string {
	c.Example = `
	# Get information on the "hello_world" pack
	nomad-pack info hello_world
	`

	return formatHelp(`
	Usage: nomad-pack info <pack-name>

	Returns information on the given pack including name, description, and variable details.

` + c.GetExample() + c.Flags().Help())
}

func (c *InfoCommand) Synopsis() string {
	return "Get information on a pack"
}
