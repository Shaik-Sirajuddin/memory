package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
	"github.com/Shaik-Sirajuddin/memory/operator"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DefaultCli wires cobra namespaces for config and agent operations.
type DefaultCli struct {
	root           *cobra.Command
	operator       operator.Operator
	configResolver config.OmniConfigResolver
}

// Entrypoint builds the CLI root command.
func Entrypoint(op operator.Operator, resolver config.OmniConfigResolver) *DefaultCli {
	c := &DefaultCli{
		operator:       op,
		configResolver: resolver,
	}

	root := &cobra.Command{
		Use:   "omni",
		Short: "Omni agent CLI",
	}

	root.AddCommand(c.newConfigCommand())
	root.AddCommand(c.newAgentCommand())
	root.AddCommand(c.newTeamInitCommand())

	c.root = root
	return c
}

// Install executes the CLI.
func (c *DefaultCli) Install() error {
	if c == nil || c.root == nil {
		return errors.New("cli is not initialized")
	}
	return c.root.Execute()
}

func (c *DefaultCli) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage omni config",
	}

	var output string
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Print resolved omni config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.configResolver == nil {
				return errors.New("config resolver is required")
			}
			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}
			return printOutput("config.get", output, cfg)
		},
	}
	getCmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output format: yaml|table|json")
	configCmd.AddCommand(getCmd)

	configCmd.AddCommand(c.newConfigSetCommand())

	return configCmd
}

func (c *DefaultCli) newConfigSetCommand() *cobra.Command {
	var memory bool
	var autoSync bool

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Update omni config feature flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.configResolver == nil {
				return errors.New("config resolver is required")
			}

			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("memory") {
				cfg.Features.Memory = memory
			}
			if cmd.Flags().Changed("autosync") {
				cfg.Features.AutoSync = autoSync
			}

			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}

			return printJSON(cfg)
		},
	}

	setCmd.Flags().BoolVar(&memory, "memory", false, "Enable or disable memory feature")
	setCmd.Flags().BoolVar(&autoSync, "autosync", true, "Enable or disable autosync feature")

	return setCmd
}

func (c *DefaultCli) newAgentCommand() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents through operator",
	}

	agentCmd.AddCommand(&cobra.Command{
		Use:   "discover",
		Short: "Discover available code agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			result, err := c.operator.DisoverCodeAgents()
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	})

	agentCmd.AddCommand(c.newAgentListCommand())
	agentCmd.AddCommand(c.newAgentCreateCommand())
	agentCmd.AddCommand(c.newAgentDeleteCommand())
	agentCmd.AddCommand(c.newWorkspaceListCommand())
	agentCmd.AddCommand(c.newWorkspaceGetCommand())

	return agentCmd
}

func (c *DefaultCli) newTeamInitCommand() *cobra.Command {
	var interactive bool

	cmd := &cobra.Command{
		Use:   "team-init",
		Short: "Initialize a team for the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}

			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve current workspace: %w", err)
			}

			return c.operator.CreateAgent(operator.CreateAgentParams{
				Workspace:   sandbox.WorkspaceDir(wd),
				Interactive: interactive,
			})
		},
	}

	cmd.Flags().BoolVar(&interactive, "interactive", false, "Launch agent after initialization")
	return cmd
}

func (c *DefaultCli) newAgentListCommand() *cobra.Command {
	var workspace string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agents for a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			result, err := c.operator.ListCodeAgents(operator.GetCodeAgentsParams{
				Workspace: sandbox.WorkspaceDir(workspace),
			})
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}

	listCmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory")

	return listCmd
}

func (c *DefaultCli) newAgentCreateCommand() *cobra.Command {
	var workspace string
	var interactive bool

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create an agent in workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			return c.operator.CreateAgent(operator.CreateAgentParams{
				Workspace:   sandbox.WorkspaceDir(workspace),
				Interactive: interactive,
			})
		},
	}

	createCmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory")
	createCmd.Flags().BoolVar(&interactive, "interactive", true, "Launch agent after create")

	return createCmd
}

func (c *DefaultCli) newAgentDeleteCommand() *cobra.Command {
	var id string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete agent from index by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			if id == "" {
				return errors.New("agent id is required")
			}
			return c.operator.DeleteAgent(operator.DeleteAgentParams{ID: id})
		},
	}

	deleteCmd.Flags().StringVar(&id, "id", "", "Agent ID")

	return deleteCmd
}

func (c *DefaultCli) newWorkspaceListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "workspace-list",
		Short: "List operator workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			result, err := c.operator.ListWorkspaces(operator.ListWorkspacesParams{})
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
}

func (c *DefaultCli) newWorkspaceGetCommand() *cobra.Command {
	var id string
	var output string

	getCmd := &cobra.Command{
		Use:   "workspace-get",
		Short: "Get a workspace by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			if id == "" {
				return errors.New("workspace id is required")
			}
			result, err := c.operator.GetWorkspace(operator.GetWorkSpaceParams{ID: id})
			if err != nil {
				return err
			}
			return printOutput("agent.workspace-get", output, result)
		},
	}

	getCmd.Flags().StringVar(&id, "id", "", "Workspace ID")
	getCmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|yaml|json")

	return getCmd
}

func printOutput(kind, format string, v any) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return printJSON(v)
	case "yaml", "yml":
		return printYAML(v)
	case "table":
		return printTable(kind, v)
	default:
		return fmt.Errorf("unsupported output format %q (use: yaml|table|json)", format)
	}
}

func printJSON(v any) error {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

func printYAML(v any) error {
	payload, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Print(string(payload))
	return nil
}

func printTable(kind string, v any) error {
	switch kind {
	case "config.get":
		return printConfigTable(v)
	case "agent.workspace-get":
		return printWorkspaceTable(v)
	default:
		return printJSON(v)
	}
}

func printConfigTable(v any) error {
	cfg, ok := v.(*config.OmniConfig)
	if !ok || cfg == nil {
		return errors.New("invalid config payload for table output")
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	memory := false
	autoSync := false
	if cfg.Features != nil {
		memory = cfg.Features.Memory
		autoSync = cfg.Features.AutoSync
	}
	fmt.Fprintf(tw, "features.memory\t%t\n", memory)
	fmt.Fprintf(tw, "features.autosync\t%t\n", autoSync)
	fmt.Fprintf(tw, "agent\t%t\n", cfg.Agent != nil)
	return tw.Flush()
}

func printWorkspaceTable(v any) error {
	result, ok := v.(operator.GetTeamResult)
	if !ok {
		return errors.New("invalid workspace payload for table output")
	}

	if result.Info != nil {
		fmt.Printf("WORKSPACE\t%s\n", result.Info.ID)
		fmt.Printf("NAME\t%s\n", result.Info.Name)
		fmt.Printf("DIR\t%s\n", result.Info.WorkspaceDir)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT_ID\tNAME\tWORKSPACE\tMEMORY_DIR")
	for _, a := range result.Agents {
		if a == nil {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.ID, a.Name, a.WorkspaceDir, a.MemoryDir)
	}
	return tw.Flush()
}
