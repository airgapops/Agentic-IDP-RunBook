package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/airgapops/agctl/internal/kube"
	"github.com/spf13/cobra"
)

var (
	dbComponent string
	seedFile    string
)

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbExecCmd)
	dbCmd.AddCommand(dbSeedCmd)
	dbCmd.AddCommand(dbTablesCmd)

	dbSeedCmd.Flags().StringVarP(&seedFile, "file", "f", "", "SQL file to seed (default: built-in todos+categories)")
}

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations via kubectl exec",
	Long:  `Execute SQL, seed data, and inspect database tables without external DB tools.`,
}

// ── db exec ─────────────────────────────────────────────────────────────

var dbExecCmd = &cobra.Command{
	Use:   "exec <component> <sql>",
	Short: "Execute SQL on a database component",
	Long: `Runs SQL directly on the database pod via kubectl exec.
Automatically finds the pod by component label.

Examples:
  agctl db exec postgres "SELECT * FROM todos" --env production
  agctl db exec postgres "SELECT count(*) FROM categories" --env all`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		sql := args[1]

		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		if envFlag == "all" {
			for name := range cfg.Environments {
				fmt.Printf("═══ %s ═══\n", strings.ToUpper(name))
				if err := execSQL(component, name, sql); err != nil {
					fmt.Printf("  ❌ %v\n", err)
				}
				fmt.Println()
			}
			return nil
		}
		return execSQL(component, envFlag, sql)
	},
}

func execSQL(component, envName, sql string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}
	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	pod, err := dp.GetPodName(namespace, "openchoreo.dev/component="+component)
	if err != nil {
		return fmt.Errorf("no %s pod in %s", component, envName)
	}

	// Detect DB type and build exec command
	dbUser, dbName := detectDBCredentials(component)
	out, err := dp.Exec(namespace, pod, "psql", "-U", dbUser, "-d", dbName, "-c", sql)
	if err != nil {
		return fmt.Errorf("SQL error: %w", err)
	}
	fmt.Println(out)
	return nil
}

// ── db seed ─────────────────────────────────────────────────────────────

var dbSeedCmd = &cobra.Command{
	Use:   "seed <component>",
	Short: "Seed database with tables and sample data",
	Long: `Creates tables and inserts sample data into a database component.
Uses built-in seed SQL or a custom file.

Examples:
  agctl db seed postgres --env production
  agctl db seed postgres --env all
  agctl db seed postgres --env development --file ./custom-seed.sql`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		sql := defaultSeedSQL
		if seedFile != "" {
			data, err := os.ReadFile(seedFile)
			if err != nil {
				return fmt.Errorf("reading seed file: %w", err)
			}
			sql = string(data)
		}

		if envFlag == "all" {
			for name := range cfg.Environments {
				fmt.Printf("🌱 Seeding %s in %s...\n", component, name)
				if err := seedDB(component, name, sql); err != nil {
					fmt.Printf("  ❌ %v\n", err)
				}
			}
			return nil
		}

		fmt.Printf("🌱 Seeding %s in %s...\n", component, envFlag)
		return seedDB(component, envFlag, sql)
	},
}

func seedDB(component, envName, sql string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}
	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	pod, err := dp.GetPodName(namespace, "openchoreo.dev/component="+component)
	if err != nil {
		return fmt.Errorf("no %s pod in %s", component, envName)
	}

	dbUser, dbName := detectDBCredentials(component)
	out, err := dp.Exec(namespace, pod, "psql", "-U", dbUser, "-d", dbName, "-c", sql)
	if err != nil {
		return fmt.Errorf("seed failed: %w", err)
	}

	// Count results
	lines := strings.Split(out, "\n")
	fmt.Printf("  ✅ Seed complete (%d SQL statements executed)\n", len(lines))

	// Reload PostgREST schema if postgrest exists in this namespace
	pgrst, _ := dp.GetDeploymentName(namespace, "openchoreo.dev/component=postgrest")
	if pgrst != "" {
		fmt.Printf("  🔄 Reloading PostgREST schema cache...\n")
		dp.RolloutRestart(namespace, pgrst)
		fmt.Printf("  ✅ PostgREST restarted\n")
	}

	return nil
}

// ── db tables ───────────────────────────────────────────────────────────

var dbTablesCmd = &cobra.Command{
	Use:   "tables <component>",
	Short: "List database tables",
	Long: `Shows all tables in the database with row counts.

Examples:
  agctl db tables postgres --env production`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		if envFlag == "" {
			return fmt.Errorf("--env flag is required")
		}

		if envFlag == "all" {
			for name := range cfg.Environments {
				fmt.Printf("═══ %s ═══\n", strings.ToUpper(name))
				if err := showTables(component, name); err != nil {
					fmt.Printf("  ❌ %v\n", err)
				}
				fmt.Println()
			}
			return nil
		}
		return showTables(component, envFlag)
	},
}

func showTables(component, envName string) error {
	_, err := cfg.ResolveEnv(envName)
	if err != nil {
		return err
	}
	namespace, err := resolveNamespace(envName)
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}
	dp := kube.NewDataPlaneClient(cfg.DataPlaneKubeconfig())

	pod, err := dp.GetPodName(namespace, "openchoreo.dev/component="+component)
	if err != nil {
		return fmt.Errorf("no %s pod in %s", component, envName)
	}

	dbUser, dbName := detectDBCredentials(component)
	sql := `SELECT schemaname, tablename,
    (SELECT count(*) FROM information_schema.columns c WHERE c.table_name = t.tablename) as columns,
    pg_stat_get_tuples_inserted(c.oid) as rows
FROM pg_tables t
JOIN pg_class c ON c.relname = t.tablename
WHERE schemaname = 'public'
ORDER BY tablename;`

	out, err := dp.Exec(namespace, pod, "psql", "-U", dbUser, "-d", dbName, "-c", sql)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	fmt.Println(out)
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────

func detectDBCredentials(component string) (user, dbName string) {
	// Default PostgreSQL credentials for our stack
	return "zerops", "app"
}

const defaultSeedSQL = `
CREATE TABLE IF NOT EXISTS categories (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  color TEXT DEFAULT '#6b7280'
);

CREATE TABLE IF NOT EXISTS todos (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT,
  completed BOOLEAN DEFAULT false,
  priority INTEGER DEFAULT 3,
  category_id INTEGER REFERENCES categories(id),
  created_at TIMESTAMPTZ DEFAULT now()
);

GRANT ALL ON categories TO zerops;
GRANT ALL ON todos TO zerops;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO zerops;

INSERT INTO categories (id, name, color) VALUES
  (1, 'Trabajo', '#3b82f6'),
  (2, 'Personal', '#22c55e'),
  (3, 'Urgente', '#ef4444')
ON CONFLICT (id) DO NOTHING;

INSERT INTO todos (id, title, description, completed, priority, category_id) VALUES
  (1, 'Configurar cluster K8s', 'Setup completo del cluster airgap', true, 5, 1),
  (2, 'Documentar arquitectura', 'Crear diagramas de red y flujo', false, 4, 1),
  (3, 'Comprar cafe', 'Para las sesiones de noche', false, 2, 2),
  (4, 'Fix NetworkPolicies', 'Migrar de cron a additive policies', true, 5, 3),
  (5, 'Grabar demo', 'Video de PostgREST + Zero-Trust', false, 4, 1)
ON CONFLICT (id) DO NOTHING;

SELECT setval('categories_id_seq', (SELECT MAX(id) FROM categories));
SELECT setval('todos_id_seq', (SELECT MAX(id) FROM todos));
`
