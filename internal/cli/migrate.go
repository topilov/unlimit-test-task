package cli

import (
	"fmt"

	migrations "apm-investigator/db"
	"apm-investigator/internal/config"
	"github.com/spf13/cobra"
)

func migrateCommand() *cobra.Command {
	return &cobra.Command{Use: "migrate", Hidden: true, Short: "Apply embedded PostgreSQL migrations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := migrations.Apply(config.DatabaseURL()); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Migrations applied")
		return nil
	}}
}
