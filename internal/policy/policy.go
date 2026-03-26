package policy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aparcero/mcp-postgres/internal/sqlguard"
)

type Mode string

const (
	ModeReadOnly Mode = "readonly"
	ModeOperator Mode = "operator"
	ModeAdmin    Mode = "admin"
)

var DefaultDeniedSchemas = []string{"pg_catalog", "information_schema"}

type Policy struct {
	mode          Mode
	deniedSchemas map[string]struct{}
	deniedOrder   []string
	mutationDBs   map[string]struct{}
	mutationOrder []string
}

func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ModeReadOnly):
		return ModeReadOnly, nil
	case string(ModeOperator):
		return ModeOperator, nil
	case string(ModeAdmin):
		return ModeAdmin, nil
	default:
		return "", fmt.Errorf("POSTGRES_MODE must be one of readonly, operator, admin")
	}
}

func New(mode Mode, deniedSchemas []string, mutationDatabases []string) Policy {
	if mode == "" {
		mode = ModeReadOnly
	}
	if len(deniedSchemas) == 0 {
		deniedSchemas = DefaultDeniedSchemas
	}

	normalized := normalizeNames(deniedSchemas)

	denied := make(map[string]struct{}, len(normalized))
	for _, schema := range normalized {
		denied[schema] = struct{}{}
	}

	mutationNames := normalizeNames(mutationDatabases)
	mutationDBs := make(map[string]struct{}, len(mutationNames))
	for _, database := range mutationNames {
		mutationDBs[database] = struct{}{}
	}

	return Policy{
		mode:          mode,
		deniedSchemas: denied,
		deniedOrder:   normalized,
		mutationDBs:   mutationDBs,
		mutationOrder: mutationNames,
	}
}

func (p Policy) Mode() Mode {
	return p.mode
}

func (p Policy) DeniedSchemas() []string {
	if len(p.deniedOrder) == 0 {
		return nil
	}

	out := make([]string, len(p.deniedOrder))
	copy(out, p.deniedOrder)
	return out
}

func (p Policy) MutationDatabases() []string {
	if len(p.mutationOrder) == 0 {
		return nil
	}

	out := make([]string, len(p.mutationOrder))
	copy(out, p.mutationOrder)
	return out
}

func (p Policy) AuthorizeQuery(statement sqlguard.Statement) error {
	if statement.Class == sqlguard.StatementClassReadOnly {
		return nil
	}

	if statement.Reason != "" {
		return fmt.Errorf("query tool only accepts read-only statements: %s", statement.Reason)
	}

	return fmt.Errorf("query tool only accepts read-only statements; got %s (%s)", statement.Class, statement.Kind)
}

func (p Policy) AuthorizeDML(database string, statement sqlguard.Statement) error {
	if statement.Class != sqlguard.StatementClassMutating {
		if statement.Reason != "" {
			return fmt.Errorf("DML tool only accepts mutating statements: %s", statement.Reason)
		}
		return fmt.Errorf("DML tool only accepts INSERT, UPDATE, DELETE, or MERGE statements; got %s (%s)", statement.Class, statement.Kind)
	}
	if err := p.checkMutationSafety(statement); err != nil {
		return err
	}

	return p.AuthorizeExec(database, statement)
}

func (p Policy) AuthorizeAdmin(database string, statement sqlguard.Statement) error {
	switch statement.Class {
	case sqlguard.StatementClassAdministrative, sqlguard.StatementClassDestructive, sqlguard.StatementClassPrivilegeChanging:
		if p.mode != ModeAdmin {
			return fmt.Errorf("administrative statements are only allowed in admin mode")
		}
		if err := p.checkMutationDatabase(database); err != nil {
			return err
		}
		return p.checkTargets(statement)
	case sqlguard.StatementClassMutating:
		return fmt.Errorf("admin tool does not accept DML statements; use postgres.exec_dml")
	case sqlguard.StatementClassReadOnly:
		return fmt.Errorf("admin tool does not accept read-only statements; use postgres.query")
	case sqlguard.StatementClassSessionChanging:
		if statement.Reason != "" {
			return fmt.Errorf("session-changing statements are not allowed: %s", statement.Reason)
		}
		return fmt.Errorf("session-changing statements are not allowed")
	case sqlguard.StatementClassUnsupported:
		if statement.Reason != "" {
			return fmt.Errorf("statement is not supported: %s", statement.Reason)
		}
		return fmt.Errorf("statement is not supported")
	default:
		return fmt.Errorf("unknown statement class %q", statement.Class)
	}
}

func (p Policy) AuthorizeExec(database string, statement sqlguard.Statement) error {
	switch statement.Class {
	case sqlguard.StatementClassReadOnly:
		return nil
	case sqlguard.StatementClassAdministrative:
		if p.mode != ModeAdmin {
			return fmt.Errorf("administrative statements are only allowed in admin mode")
		}
		if err := p.checkMutationDatabase(database); err != nil {
			return err
		}
		return p.checkTargets(statement)
	case sqlguard.StatementClassMutating:
		if p.mode == ModeReadOnly {
			return fmt.Errorf("mutating statements are not allowed in readonly mode")
		}
		if err := p.checkMutationSafety(statement); err != nil {
			return err
		}
		if err := p.checkMutationDatabase(database); err != nil {
			return err
		}
		return p.checkTargets(statement)
	case sqlguard.StatementClassDestructive:
		if p.mode != ModeAdmin {
			return fmt.Errorf("destructive statements are only allowed in admin mode")
		}
		if err := p.checkMutationDatabase(database); err != nil {
			return err
		}
		return p.checkTargets(statement)
	case sqlguard.StatementClassPrivilegeChanging:
		if p.mode != ModeAdmin {
			return fmt.Errorf("privilege-changing statements are only allowed in admin mode")
		}
		if err := p.checkMutationDatabase(database); err != nil {
			return err
		}
		return p.checkTargets(statement)
	case sqlguard.StatementClassSessionChanging:
		if statement.Reason != "" {
			return fmt.Errorf("session-changing statements are not allowed: %s", statement.Reason)
		}
		return fmt.Errorf("session-changing statements are not allowed")
	case sqlguard.StatementClassUnsupported:
		if statement.Reason != "" {
			return fmt.Errorf("statement is not supported: %s", statement.Reason)
		}
		return fmt.Errorf("statement is not supported")
	default:
		return fmt.Errorf("unknown statement class %q", statement.Class)
	}
}

func (p Policy) checkMutationDatabase(database string) error {
	normalized := strings.ToLower(strings.TrimSpace(database))
	if normalized == "" {
		return fmt.Errorf("database must not be empty")
	}
	if len(p.mutationDBs) == 0 {
		return fmt.Errorf("no mutation databases are configured for write operations")
	}
	if _, ok := p.mutationDBs[normalized]; !ok {
		return fmt.Errorf("database %q is not allowed for mutation by policy", database)
	}
	return nil
}

func (p Policy) checkMutationSafety(statement sqlguard.Statement) error {
	if statement.HasBroadMutation {
		return fmt.Errorf("UPDATE and DELETE statements must include a WHERE clause")
	}
	return nil
}

func (p Policy) checkTargets(statement sqlguard.Statement) error {
	if statement.HasUnqualifiedTargets {
		return fmt.Errorf("non-read-only statements must use explicit schema-qualified targets")
	}

	for _, schema := range statement.TargetSchemas {
		if _, denied := p.deniedSchemas[strings.ToLower(schema)]; denied {
			return fmt.Errorf("schema %q is denied by policy", schema)
		}
	}

	return nil
}

func normalizeNames(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	sort.Strings(normalized)
	return normalized
}
