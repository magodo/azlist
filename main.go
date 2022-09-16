package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
)

func main() {
	var (
		flagSubscriptionId string
	)

	app := &cli.App{
		Name:      "azlist",
		Version:   getVersion(),
		Usage:     "List Azure resources (including proxy resources)",
		UsageText: "azlist [option] <ARG query>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "subscription-id",
				// Honor the "ARM_SUBSCRIPTION_ID" as is used by the AzureRM provider, for easier use.
				EnvVars:     []string{"AZLIST_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID"},
				Aliases:     []string{"s"},
				Required:    true,
				Usage:       "The subscription id",
				Destination: &flagSubscriptionId,
			},
		},
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() == 0 {
				return fmt.Errorf("No query specified")
			}
			if ctx.NArg() > 1 {
				return fmt.Errorf("More than one queries specified")
			}

			rset, err := List(ctx.Context, flagSubscriptionId, ctx.Args().First())
			if err != nil {
				return err
			}

			var rl []string
			for id := range rset {
				rl = append(rl, id)
			}
			slices.Sort(rl)
			for _, id := range rl {
				fmt.Println(id)
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
