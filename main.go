package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	var (
		flagSubscriptionId string
		flagWithBody       bool
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
			&cli.BoolFlag{
				Name:        "with-body",
				EnvVars:     []string{"AZLIST_WITH_BODY"},
				Aliases:     []string{"b"},
				Usage:       "Print each resource's body",
				Destination: &flagWithBody,
			},
		},
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() == 0 {
				return fmt.Errorf("No query specified")
			}
			if ctx.NArg() > 1 {
				return fmt.Errorf("More than one queries specified")
			}

			rl, err := List(ctx.Context, flagSubscriptionId, ctx.Args().First(), nil)
			if err != nil {
				return err
			}

			for _, res := range rl {
				fmt.Println(res.Id)
				if flagWithBody {
					b, _ := json.MarshalIndent(res.Properties, "", "  ")
					fmt.Println(string(b))
				}
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
