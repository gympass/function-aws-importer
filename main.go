// Package main implements a Composition Function.
package main

import (
	"context"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/function-sdk-go"
)

// CLI of this Function.
type CLI struct {
	Debug bool `short:"d" help:"Emit debug logs in addition to info logs."`

	Network     string `help:"Network on which to listen for gRPC connections." default:"tcp"`
	Address     string `help:"Address at which to listen for gRPC connections." default:":9443"`
	TLSCertsDir string `help:"Directory containing server certs (tls.key, tls.crt) and the CA used to verify client certificates (ca.crt)" env:"TLS_SERVER_CERTS_DIR"`
	Insecure    bool   `help:"Run without mTLS credentials. If you supply this flag --tls-server-certs-dir will be ignored."`
}

// Run this Function.
func (c *CLI) Run() error {
	log, err := function.NewLogger(c.Debug)
	if err != nil {
		return err
	}

	client, err := c.newResourceGroupsTaggingClient()
	if err != nil {
		return errors.Wrap(err, "building Resource Groups Tagging API client")

	}

	return function.Serve(&Function{log: log, client: client},
		function.Listen(c.Network, c.Address),
		function.MTLSCertificates(c.TLSCertsDir),
		function.Insecure(c.Insecure))
}

func (c *CLI) newResourceGroupsTaggingClient() (*resourcegroupstaggingapi.Client, error) {
	sdkConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "loading default AWS SDK configuration")
	}

	return resourcegroupstaggingapi.NewFromConfig(sdkConfig), nil
}

func main() {
	ctx := kong.Parse(&CLI{}, kong.Description("A Crossplane Composition Function."))
	ctx.FatalIfErrorf(ctx.Run())
}
