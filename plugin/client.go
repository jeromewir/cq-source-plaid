package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/cloudquery/cq-source-plaid/client"
	"github.com/cloudquery/cq-source-plaid/resources"
	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/plugin"
	"github.com/cloudquery/plugin-sdk/v4/scheduler"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
	"github.com/rs/zerolog"
)

type Client struct {
	logger          zerolog.Logger
	config          client.Spec
	tables          schema.Tables
	scheduler       *scheduler.Scheduler
	schedulerClient *client.Client
}

func (c *Client) Logger() *zerolog.Logger {
	return &c.logger
}

func (c *Client) Sync(ctx context.Context, options plugin.SyncOptions, res chan<- message.SyncMessage) error {
	tt, err := c.tables.FilterDfs(options.Tables, options.SkipTables, options.SkipDependentTables)
	if err != nil {
		return err
	}

	return c.scheduler.Sync(ctx, c.schedulerClient, tt, res, scheduler.WithSyncDeterministicCQID(options.DeterministicCQID))
}

func (c *Client) Tables(_ context.Context, options plugin.TableOptions) (schema.Tables, error) {
	tt, err := c.tables.FilterDfs(options.Tables, options.SkipTables, options.SkipDependentTables)
	if err != nil {
		return nil, err
	}

	return tt, nil
}

func (*Client) Close(_ context.Context) error {
	return nil
}

func (*Client) Write(context.Context, <-chan message.WriteMessage) error {
	// Not implemented, just used for testing destination packaging
	return nil
}

func (*Client) Read(context.Context, *schema.Table, chan<- arrow.Record) error {
	// Not implemented, just used for testing destination packaging
	return nil
}

func Configure(ctx context.Context, logger zerolog.Logger, spec []byte, opts plugin.NewClientOptions) (plugin.Client, error) {
	if opts.NoConnection {
		return &Client{
			logger: logger,
			tables: getTables(),
		}, nil
	}

	config := &client.Spec{}
	if err := json.Unmarshal(spec, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}
	config.SetDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate spec: %w", err)
	}

	schedulerClient, err := client.New(ctx, logger, spec)

	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler client: %w", err)
	}

	return &Client{
		logger: logger,
		config: *config,
		scheduler: scheduler.NewScheduler(
			scheduler.WithLogger(logger),
		),
		tables:          getTables(),
		schedulerClient: schedulerClient,
	}, nil
}

func TestConnection(_ context.Context, _ zerolog.Logger, spec []byte) error {
	config := &client.Spec{}
	if err := json.Unmarshal(spec, config); err != nil {
		return plugin.NewTestConnError("INVALID_SPEC", fmt.Errorf("failed to unmarshal spec: %w", err))
	}

	config.SetDefaults()
	if err := config.Validate(); err != nil {
		return plugin.NewTestConnError("INVALID_SPEC", fmt.Errorf("failed to validate spec: %w", err))
	}
	return nil
}

func getTables() schema.Tables {
	tables := schema.Tables{
		resources.Transactions(),
		resources.Liabilities(),
		resources.Identities(),
		resources.InvestmentsTransactions(),
		resources.InvestmentsHoldings(),
		resources.AccountBalances(),
		resources.Auths(),
		resources.Wallets(),
		resources.Institutions(),
	}

	if err := transformers.TransformTables(tables); err != nil {
		panic(err)
	}
	for _, t := range tables {
		schema.AddCqIDs(t)
	}
	return tables
}
