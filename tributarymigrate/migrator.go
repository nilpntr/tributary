package tributarymigrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
	"go.uber.org/zap"

	"github.com/nilpntr/tributary/tributarytype"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// Migrator handles database migrations for Tributary.
type Migrator struct {
	databaseURL string
	options     *Options
}

// New creates a new migrator with the given database URL and options.
func New(databaseURL string, options *Options) *Migrator {
	if options == nil {
		options = DefaultOptions()
	}
	return &Migrator{
		databaseURL: databaseURL,
		options:     options,
	}
}

// Up applies all pending migrations.
func (m *Migrator) Up() error {
	return m.UpWithContext(context.Background())
}

// UpWithContext applies all pending migrations with context.
func (m *Migrator) UpWithContext(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return &tributarytype.MigrationError{
			Operation: "up",
			Err:       err,
		}
	}
	if err = migrator.Init(ctx); err != nil {
		return &tributarytype.MigrationError{
			Operation: "up",
			Err:       err,
		}
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	_, err = migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	return nil
}

// Down applies all down migrations (rolls back to initial state).
func (m *Migrator) Down() error {
	return m.DownWithContext(context.Background())
}

// DownWithContext applies all down migrations with context.
func (m *Migrator) DownWithContext(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return &tributarytype.MigrationError{
			Operation: "down",
			Err:       err,
		}
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	_, err = migrator.Rollback(ctx)
	if err != nil {
		return err
	}

	return nil
}

// Version returns the current migration version.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return 0, err
	}
	if err = migrator.Lock(ctx); err != nil {
		return 0, err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	status, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return 0, err
	}

	return status.LastGroupID(), nil
}

// Drop drops all tables
// WARNING: This is destructive and cannot be undone.
func (m *Migrator) Drop(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	if err = migrator.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset migrator: %w", err)
	}

	return nil
}

// createMigrator creates a new bun-migrate instance.
func (m *Migrator) createMigrator() (*migrate.Migrator, error) {
	config, err := pgxpool.ParseConfig(m.databaseURL)
	if err != nil {
		zap.L().Sugar().Errorf("failed to parse database url: %v", err)
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		zap.L().Sugar().Errorf("failed to connect to database: %v", err)
		return nil, err
	}

	sqldb := stdlib.OpenDBFromPool(pool)

	idb := bun.NewDB(sqldb, pgdialect.New())
	return migrate.NewMigrator(idb, Migrations), nil
}

// Validate checks if the migration configuration is valid.
func (m *Migrator) Validate() error {
	if m.databaseURL == "" {
		return &tributarytype.ValidationError{
			Field:   "DatabaseURL",
			Message: "cannot be empty",
		}
	}

	// Parse database URL to validate format
	_, err := url.Parse(m.databaseURL)
	if err != nil {
		return &tributarytype.ValidationError{
			Field:   "DatabaseURL",
			Message: fmt.Sprintf("invalid format: %v", err),
		}
	}

	return m.options.Validate()
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Name    string
	Version int64
	Applied bool
}

// MigrationFile represents a migration file that was created.
type MigrationFile struct {
	Name    string
	Path    string
	Content string
}

// GoMigrationOption configures Go migration creation.
type GoMigrationOption func(cfg *goMigrationConfig)

type goMigrationConfig struct {
	packageName string
	goTemplate  string
}

// WithPackageName sets the package name for generated Go migrations.
func WithPackageName(name string) GoMigrationOption {
	return func(cfg *goMigrationConfig) {
		cfg.packageName = name
	}
}

// WithGoTemplate sets the template for generated Go migrations.
func WithGoTemplate(template string) GoMigrationOption {
	return func(cfg *goMigrationConfig) {
		cfg.goTemplate = template
	}
}

// Status returns the status of all available migrations.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}
	if err = migrator.Lock(ctx); err != nil {
		return nil, err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	ms, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, version := range ms {
		status = append(status, MigrationStatus{
			Name:    version.Name,
			Version: version.ID,
			Applied: version.IsApplied(),
		})
	}

	return status, nil
}

// CreateSQLMigration creates up and down SQL migration files.
func (m *Migrator) CreateSQLMigration(ctx context.Context, name string) ([]*MigrationFile, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}

	bunFiles, err := migrator.CreateSQLMigrations(ctx, name)
	if err != nil {
		return nil, err
	}

	// Convert bun migration files to our type
	files := make([]*MigrationFile, len(bunFiles))
	for i, bunFile := range bunFiles {
		files[i] = &MigrationFile{
			Name:    bunFile.Name,
			Path:    bunFile.Path,
			Content: bunFile.Content,
		}
	}

	return files, nil
}

// CreateGoMigration creates a Go migration file.
func (m *Migrator) CreateGoMigration(ctx context.Context, name string, opts ...GoMigrationOption) (*MigrationFile, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}

	// Convert our options to bun options
	cfg := &goMigrationConfig{
		packageName: "migrations",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var bunOpts []migrate.GoMigrationOption
	if cfg.packageName != "" {
		bunOpts = append(bunOpts, migrate.WithPackageName(cfg.packageName))
	}
	if cfg.goTemplate != "" {
		bunOpts = append(bunOpts, migrate.WithGoTemplate(cfg.goTemplate))
	}

	bunFile, err := migrator.CreateGoMigration(ctx, name, bunOpts...)
	if err != nil {
		return nil, err
	}

	return &MigrationFile{
		Name:    bunFile.Name,
		Path:    bunFile.Path,
		Content: bunFile.Content,
	}, nil
}

// UpTo applies migrations up to the specified target migration.
func (m *Migrator) UpTo(ctx context.Context, target string) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return &tributarytype.MigrationError{
			Operation: "up_to",
			Err:       err,
		}
	}
	if err = migrator.Init(ctx); err != nil {
		return &tributarytype.MigrationError{
			Operation: "up_to",
			Err:       err,
		}
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	// Get current migrations status
	migrations, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}

	// Find target migration
	var targetIndex = -1
	for i, migration := range migrations {
		if migration.Name == target {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("migration %q not found", target)
	}

	// Apply migrations up to target - both mark as applied AND execute them
	for i := 0; i <= targetIndex; i++ {
		migration := &migrations[i]
		if !migration.IsApplied() {
			// Execute the migration first
			if migration.Up != nil {
				if err := migration.Up(ctx, migrator.DB(), nil); err != nil {
					return fmt.Errorf("migration %q failed: %w", migration.Name, err)
				}
			}

			// Then mark as applied
			if err := migrator.MarkApplied(ctx, migration); err != nil {
				return fmt.Errorf("failed to mark migration %q as applied: %w", migration.Name, err)
			}
		}
	}

	return nil
}

// DownTo rolls back migrations down to the specified target migration.
func (m *Migrator) DownTo(ctx context.Context, target string) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return &tributarytype.MigrationError{
			Operation: "down_to",
			Err:       err,
		}
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	// Get current migrations status
	migrations, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}

	// Find target migration
	var targetIndex = -1
	for i, migration := range migrations {
		if migration.Name == target {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("migration %q not found", target)
	}

	// Roll back migrations after target - both execute down migration AND mark as unapplied
	for i := len(migrations) - 1; i > targetIndex; i-- {
		migration := &migrations[i]
		if migration.IsApplied() {
			// Execute the down migration first
			if migration.Down != nil {
				if err := migration.Down(ctx, migrator.DB(), nil); err != nil {
					return fmt.Errorf("migration %q rollback failed: %w", migration.Name, err)
				}
			}

			// Then mark as unapplied
			if err := migrator.MarkUnapplied(ctx, migration); err != nil {
				return fmt.Errorf("failed to mark migration %q as unapplied: %w", migration.Name, err)
			}
		}
	}

	return nil
}

// Rollback rolls back the last migration group.
func (m *Migrator) Rollback(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return &tributarytype.MigrationError{
			Operation: "rollback",
			Err:       err,
		}
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	_, err = migrator.Rollback(ctx)
	return err
}

// Reset drops all migration tables and recreates them.
func (m *Migrator) Reset(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	if err = migrator.Lock(ctx); err != nil {
		return err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	return migrator.Reset(ctx)
}

// MissingMigrations returns applied migrations that can no longer be found.
func (m *Migrator) MissingMigrations(ctx context.Context) ([]MigrationStatus, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}
	if err = migrator.Lock(ctx); err != nil {
		return nil, err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	missing, err := migrator.MissingMigrations(ctx)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, migration := range missing {
		status = append(status, MigrationStatus{
			Name:    migration.Name,
			Version: migration.ID,
			Applied: migration.IsApplied(),
		})
	}

	return status, nil
}

// AppliedMigrations returns all applied migrations.
func (m *Migrator) AppliedMigrations(ctx context.Context) ([]MigrationStatus, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}
	if err = migrator.Lock(ctx); err != nil {
		return nil, err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	applied, err := migrator.AppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, migration := range applied {
		status = append(status, MigrationStatus{
			Name:    migration.Name,
			Version: migration.ID,
			Applied: true,
		})
	}

	return status, nil
}

// UnappliedMigrations returns all unapplied migrations.
func (m *Migrator) UnappliedMigrations(ctx context.Context) ([]MigrationStatus, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return nil, err
	}
	if err = migrator.Lock(ctx); err != nil {
		return nil, err
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	ms, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, migration := range ms {
		if !migration.IsApplied() {
			status = append(status, MigrationStatus{
				Name:    migration.Name,
				Version: migration.ID,
				Applied: false,
			})
		}
	}

	return status, nil
}
