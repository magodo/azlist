package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/hashicorp/go-hclog"
	"github.com/magodo/azlist/azlist"

	"github.com/urfave/cli/v2"
)

func main() {
	var (
		flagEnvironment          string
		flagSubscriptionId       string
		flagRecursive            bool
		flagWithBody             bool
		flagIncludeManaged       bool
		flagIncludeResourceGroup bool
		flagParallelism          int
		flagExtensions           cli.StringSlice
		flagPrintError           bool
		flagVerbose              bool
	)

	app := &cli.App{
		Name:      "azlist",
		Version:   getVersion(),
		Usage:     "List Azure resources by an Azure Resource Graph `where` predicate",
		UsageText: "azlist [option] <ARG where predicate>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "env",
				EnvVars:     []string{"AZLIST_ENV"},
				Usage:       `The environment. Can be one of "public", "china", "usgovernment".`,
				Destination: &flagEnvironment,
				Value:       "public",
			},
			&cli.StringFlag{
				Name:        "subscription-id",
				EnvVars:     []string{"AZLIST_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID"},
				Aliases:     []string{"s"},
				Required:    true,
				Usage:       "The subscription id",
				Destination: &flagSubscriptionId,
			},
			&cli.BoolFlag{
				Name:        "recursive",
				Aliases:     []string{"r"},
				EnvVars:     []string{"AZLIST_RECURSIVE"},
				Usage:       "Recursively list child resources of the query result",
				Destination: &flagRecursive,
			},
			&cli.BoolFlag{
				Name:        "with-body",
				EnvVars:     []string{"AZLIST_WITH_BODY"},
				Aliases:     []string{"b"},
				Usage:       "Print each resource's body",
				Destination: &flagWithBody,
			},
			&cli.BoolFlag{
				Name:        "include-managed",
				Aliases:     []string{"m"},
				EnvVars:     []string{"AZLIST_INCLUDE_MANAGED"},
				Usage:       "Include resource whose lifecycle is managed by others",
				Destination: &flagIncludeManaged,
			},
			&cli.BoolFlag{
				Name:        "include-resource-group",
				EnvVars:     []string{"AZLIST_INCLUDE_RESOURCE_GROUP"},
				Usage:       "Include the resource groups that the listed resources belong to",
				Destination: &flagIncludeResourceGroup,
			},
			&cli.IntFlag{
				Name:        "parallelism",
				EnvVars:     []string{"AZLIST_PARALLELISM"},
				Aliases:     []string{"p"},
				Usage:       "Limit the number of parallel operations to list resources",
				Value:       10,
				Destination: &flagParallelism,
			},
			&cli.StringSliceFlag{
				Name:    "extension",
				EnvVars: []string{"AZLIST_EXTENSION"},
				Usage: `Specify a list of extension resource types (e.g. "Microsoft.Authorization/roleAssignments"). Some extension resource types have special filtering, which includes:
	- Microsoft.Authorization/roleAssignments: Only role assignments whose "scope" is the same as the current resource is listed`,
				Destination: &flagExtensions,
			},
			&cli.BoolFlag{
				Name:        "print-error",
				Aliases:     []string{"e"},
				EnvVars:     []string{"AZLIST_PRINT_ERROR"},
				Usage:       "Print errors received during listing resources",
				Destination: &flagPrintError,
			},
			&cli.BoolFlag{
				Name:        "verbose",
				EnvVars:     []string{"AZLIST_VERBOSE"},
				Usage:       "Print verbose output",
				Destination: &flagVerbose,
			},
		},
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() == 0 {
				return fmt.Errorf("No ARG where predicate specified")
			}
			if ctx.NArg() > 1 {
				return fmt.Errorf("More than one where predicates specified")
			}

			if flagVerbose {
				logger := hclog.New(&hclog.LoggerOptions{
					Name:  "azlist",
					Level: hclog.Debug,
				}).StandardLogger(&hclog.StandardLoggerOptions{
					InferLevels: true,
				})
				azlist.SetLogger(logger)
			}

			cloudCfg := cloud.AzurePublic
			switch strings.ToLower(flagEnvironment) {
			case "public":
				cloudCfg = cloud.AzurePublic
			case "usgovernment":
				cloudCfg = cloud.AzureGovernment
			case "china":
				cloudCfg = cloud.AzureChina
			default:
				return fmt.Errorf("unknown environment specified: %q", flagEnvironment)
			}

			if v, ok := os.LookupEnv("ARM_TENANT_ID"); ok {
				os.Setenv("AZURE_TENANT_ID", v)
			}
			if v, ok := os.LookupEnv("ARM_CLIENT_ID"); ok {
				os.Setenv("AZURE_CLIENT_ID", v)
			}
			if v, ok := os.LookupEnv("ARM_CLIENT_SECRET"); ok {
				os.Setenv("AZURE_CLIENT_SECRET", v)
			}
			if v, ok := os.LookupEnv("ARM_CLIENT_CERTIFICATE_PATH"); ok {
				os.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", v)
			}

			clientOpt := arm.ClientOptions{
				ClientOptions: policy.ClientOptions{
					Cloud: cloudCfg,
					Telemetry: policy.TelemetryOptions{
						ApplicationID: "azlist",
						Disabled:      false,
					},
					Logging: policy.LogOptions{
						IncludeBody: true,
					},
				},
			}

			cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
				ClientOptions: clientOpt.ClientOptions,
				TenantID:      os.Getenv("ARM_TENANT_ID"),
			})
			if err != nil {
				return fmt.Errorf("failed to obtain a credential: %v", err)
			}

			var extensions []azlist.ExtensionResource
			for _, rt := range flagExtensions.Value() {
				extension := azlist.ExtensionResource{Type: rt}
				if strings.EqualFold(rt, "Microsoft.Authorization/roleAssignments") {
					extension.Filter = func(res, extensionRes map[string]interface{}) bool {
						idRaw, ok := res["id"]
						if !ok {
							return false
						}
						id := idRaw.(string)

						propsRaw, ok := extensionRes["properties"]
						if !ok {
							return false
						}
						scopeRaw, ok := propsRaw.(map[string]interface{})["scope"]
						if !ok {
							return false
						}
						scope := scopeRaw.(string)

						return strings.EqualFold(id, scope)
					}
				}
				extensions = append(extensions, extension)
			}

			opt := azlist.Option{
				SubscriptionId: flagSubscriptionId,
				Cred:           cred,
				ClientOpt:      clientOpt,

				Parallelism:            flagParallelism,
				Recursive:              flagRecursive,
				IncludeManaged:         flagIncludeManaged,
				IncludeResourceGroup:   flagIncludeResourceGroup,
				ExtensionResourceTypes: extensions,
			}

			result, err := azlist.List(ctx.Context, ctx.Args().First(), opt)
			if err != nil {
				return err
			}

			if flagPrintError {
				if len(result.Errors) != 0 {
					fmt.Println("Listing errors:")
					for _, err := range result.Errors {
						fmt.Printf("\t%v\n", err)
					}
					fmt.Println()
				}
			}

			for _, res := range result.Resources {
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
