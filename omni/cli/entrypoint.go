package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/operator"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
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
	root.AddCommand(c.newTeamCommand())
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

	flags := config.ProvisionConfigGetFlags()
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Print resolved omni config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.configResolver == nil {
				return errors.New("config resolver is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}
			return printOutput("config.get", resolved.Output, cfg)
		},
	}
	getCmd.Flags().StringP("output", "o", flags.Output, "Output format: yaml|table|json")
	configCmd.AddCommand(getCmd)

	configCmd.AddCommand(c.newConfigSetCommand())

	return configCmd
}

func (c *DefaultCli) newConfigSetCommand() *cobra.Command {
	flags := config.ProvisionConfigSetFlags()

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Update omni config feature flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.configResolver == nil {
				return errors.New("config resolver is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}

			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}

			if resolved.Memory != "" {
				v, parseErr := strconv.ParseBool(resolved.Memory)
				if parseErr != nil {
					return fmt.Errorf("invalid --memory value %q: %w", resolved.Memory, parseErr)
				}
				cfg.Features.Memory = v
			}
			if resolved.AutoSync != "" {
				v, parseErr := strconv.ParseBool(resolved.AutoSync)
				if parseErr != nil {
					return fmt.Errorf("invalid --autosync value %q: %w", resolved.AutoSync, parseErr)
				}
				cfg.Features.AutoSync = v
			}

			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}

			return printJSON(cfg)
		},
	}

	setCmd.Flags().String("memory", flags.Memory, "Set memory feature (true|false); empty leaves value unchanged")
	setCmd.Flags().String("autosync", flags.AutoSync, "Set autosync feature (true|false); empty leaves value unchanged")

	return setCmd
}

func (c *DefaultCli) newAgentCommand() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents through operator",
	}

	agentCmd.AddCommand(c.newAgentDiscoverCommand())

	agentCmd.AddCommand(c.newAgentListCommand())
	agentCmd.AddCommand(c.newAgentCreateCommand())
	agentCmd.AddCommand(c.newAgentResumeCommand())
	agentCmd.AddCommand(c.newAgentUpgradeCommand())
	agentCmd.AddCommand(c.newAgentDeleteCommand())

	return agentCmd
}

func (c *DefaultCli) newTeamInitCommand() *cobra.Command {
	flags := config.ProvisionTeamInitFlags()

	cmd := &cobra.Command{
		Use:   "team-init",
		Short: "Initialize a team for the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}

			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve current workspace: %w", err)
			}
			memoryDir := fmt.Sprintf("%s/%s", wd, "memory")
			_, existedErr := os.Stat(memoryDir)
			existed := existedErr == nil

			if err := c.operator.TeamInit(operator.TeamInitParams{
				Workspace: sandbox.WorkspaceDir(wd),
				RepoURL:   resolved.RepoURL,
			}); err != nil {
				return err
			}
			if existed {
				fmt.Println("team reinitialized")
			} else {
				fmt.Println("team initialized")
			}
			return nil
		},
	}

	cmd.Flags().String("repo_url", flags.RepoURL, "Optional git repository URL used to add memory as submodule")
	return cmd
}

func (c *DefaultCli) newTeamCommand() *cobra.Command {
	teamCmd := &cobra.Command{
		Use:   "team",
		Short: "Manage teams (workspace groups)",
	}

	teamCmd.AddCommand(c.newTeamListCommand())
	teamCmd.AddCommand(c.newTeamGetCommand())
	teamCmd.AddCommand(c.newTeamInitSubcommand())

	return teamCmd
}

func (c *DefaultCli) newTeamListCommand() *cobra.Command {
	flags := config.ProvisionTeamListFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teams/workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			result, err := c.operator.ListWorkspaces(operator.ListWorkspacesParams{})
			if err != nil {
				return err
			}
			return printOutput("team.list", resolved.Output, result)
		},
	}

	cmd.Flags().StringP("output", "o", flags.Output, "Output format: table|yaml|json")
	return cmd
}

func (c *DefaultCli) newTeamInitSubcommand() *cobra.Command {
	flags := config.ProvisionTeamInitFlags()

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a team in the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}

			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve current workspace: %w", err)
			}
			memoryDir := fmt.Sprintf("%s/%s", wd, "memory")
			_, existedErr := os.Stat(memoryDir)
			existed := existedErr == nil

			if err := c.operator.TeamInit(operator.TeamInitParams{
				Workspace: sandbox.WorkspaceDir(wd),
				RepoURL:   resolved.RepoURL,
			}); err != nil {
				return err
			}
			if existed {
				fmt.Println("team reinitialized")
			} else {
				fmt.Println("team initialized")
			}
			return nil
		},
	}

	cmd.Flags().String("repo_url", flags.RepoURL, "Optional git repository URL used to add memory as submodule")
	return cmd
}

func (c *DefaultCli) newTeamGetCommand() *cobra.Command {
	flags := config.ProvisionTeamGetFlags()

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get a team/workspace by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			if c.operator == nil {
				return errors.New("operator is required")
			}
			if resolved.ID == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve current workspace: %w", err)
				}
				workspaces, err := c.operator.ListWorkspaces(operator.ListWorkspacesParams{})
				if err != nil {
					return err
				}
				for _, team := range workspaces.Teams {
					if team != nil && team.WorkspaceDir == wd {
						resolved.ID = team.ID
						break
					}
				}
			}
			if resolved.ID == "" {
				return errors.New("workspace id is required and no team matched current working directory")
			}
			result, err := c.operator.GetWorkspace(operator.GetWorkSpaceParams{ID: resolved.ID})
			if err != nil {
				return err
			}
			return printOutput("team.get", resolved.Output, result)
		},
	}

	getCmd.Flags().String("id", flags.ID, "Workspace ID")
	getCmd.Flags().StringP("output", "o", flags.Output, "Output format: table|yaml|json")
	return getCmd
}

func (c *DefaultCli) newAgentDiscoverCommand() *cobra.Command {
	flags := config.ProvisionAgentDiscoverFlags()

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover available code agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			result, err := c.operator.DisoverCodeAgents()
			if err != nil {
				return err
			}
			return printOutput("agent.discover", resolved.Output, result)
		},
	}

	cmd.Flags().StringP("output", "o", flags.Output, "Output format: table|yaml|json")
	return cmd
}

func (c *DefaultCli) newAgentListCommand() *cobra.Command {
	flags := config.ProvisionAgentListFlags()

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agents for a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			result, err := c.operator.ListCodeAgents(operator.GetCodeAgentsParams{
				Workspace: sandbox.WorkspaceDir(resolved.Workspace),
			})
			if err != nil {
				return err
			}
			return printOutput("agent.list", resolved.Output, result)
		},
	}

	listCmd.Flags().String("workspace", flags.Workspace, "Workspace directory")
	listCmd.Flags().StringP("output", "o", flags.Output, "Output format: table|yaml|json")

	return listCmd
}

func (c *DefaultCli) newAgentCreateCommand() *cobra.Command {
	flags := config.ProvisionAgentCreateFlags()

	createCmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Initialize an agent in workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			if len(args) == 1 {
				resolved.Name = args[0]
			}
			return c.operator.CreateAgent(operator.CreateAgentParams{
				Workspace:          sandbox.WorkspaceDir(resolved.Workspace),
				Name:               resolved.Name,
				Provider:           codeagent.Provider(resolved.Provider),
				Model:              resolved.Model,
				AllowGeneratedName: resolved.AllowGeneratedName,
				Interactive:        resolved.Interactive,
			})
		},
	}

	createCmd.Flags().String("workspace", flags.Workspace, "Workspace directory")
	createCmd.Flags().StringP("provider", "p", flags.Provider, "Agent provider (default: gemini)")
	createCmd.Flags().String("model", flags.Model, "Agent model (default depends on provider)")
	createCmd.Flags().Bool("allow_generated_name", flags.AllowGeneratedName, "Allow operator to generate agent name when name is empty")
	createCmd.Flags().Bool("interactive", flags.Interactive, "Launch agent after create")

	return createCmd
}

func (c *DefaultCli) newAgentResumeCommand() *cobra.Command {
	flags := config.ProvisionAgentResumeFlags()

	cmd := &cobra.Command{
		Use:   "resume <name>",
		Short: "Resume an indexed agent by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			return c.operator.ResumeAgent(operator.ResumeAgentParams{
				Workspace: sandbox.WorkspaceDir(resolved.Workspace),
				Name:      args[0],
			})
		},
	}

	cmd.Flags().String("workspace", flags.Workspace, "Workspace directory")
	return cmd
}

func (c *DefaultCli) newAgentUpgradeCommand() *cobra.Command {
	flags := config.ProvisionAgentUpgradeFlags()

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade an agent memory template",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			if resolved.ID == "" {
				return errors.New("agent id is required")
			}
			return c.operator.UpgradeAgent(operator.UpgradeAgentParams{
				ID:      resolved.ID,
				Version: resolved.Version,
			})
		},
	}

	cmd.Flags().String("id", flags.ID, "Agent ID")
	cmd.Flags().String("version", flags.Version, "Target version (default: latest)")
	return cmd
}

func (c *DefaultCli) newAgentDeleteCommand() *cobra.Command {
	flags := config.ProvisionAgentDeleteFlags()

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete agent from index by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c.operator == nil {
				return errors.New("operator is required")
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			if resolved.ID == "" {
				return errors.New("agent id is required")
			}
			return c.operator.DeleteAgent(operator.DeleteAgentParams{ID: resolved.ID})
		},
	}

	deleteCmd.Flags().String("id", flags.ID, "Agent ID")

	return deleteCmd
}

func loadFlags(cmd *cobra.Command, target any) error {
	k := koanf.New(".")
	if err := k.Load(posflag.Provider(cmd.Flags(), ".", k), nil); err != nil {
		return fmt.Errorf("load command flags: %w", err)
	}
	if err := k.Unmarshal("", target); err != nil {
		return fmt.Errorf("resolve command flags: %w", err)
	}
	return nil
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
	case "agent.discover":
		return printDiscoverTable(v)
	case "agent.list":
		return printAgentListTable(v)
	case "team.get":
		return printWorkspaceTable(v)
	case "team.list":
		return printTeamListTable(v)
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

func printDiscoverTable(v any) error {
	result, ok := v.(*operator.DisocveryResult)
	if !ok || result == nil {
		return errors.New("invalid discover payload for table output")
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROVIDER")
	for _, provider := range result.Providers {
		fmt.Fprintf(tw, "%s\n", provider)
	}
	return tw.Flush()
}

func printAgentListTable(v any) error {
	result, ok := v.(*operator.GetAgentsResult)
	if !ok || result == nil {
		return errors.New("invalid agent list payload for table output")
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT_ID\tNAME\tWORKSPACE\tMEMORY_DIR")
	if result.AgentInfo != nil {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", result.AgentInfo.ID, result.AgentInfo.Name, result.AgentInfo.WorkspaceDir, result.AgentInfo.MemoryDir)
	}
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

func printTeamListTable(v any) error {
	result, ok := v.(operator.ListWorkspacesResult)
	if !ok {
		return errors.New("invalid team list payload for table output")
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TEAM_ID\tNAME\tWORKSPACE_DIR\tAGENTS")
	for _, t := range result.Teams {
		if t == nil {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", t.ID, t.Name, t.WorkspaceDir, t.Agents)
	}
	return tw.Flush()
}
