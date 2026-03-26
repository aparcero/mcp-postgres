package sqlguard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

type StatementClass string

const (
	StatementClassReadOnly          StatementClass = "read_only"
	StatementClassMutating          StatementClass = "mutating"
	StatementClassAdministrative    StatementClass = "administrative"
	StatementClassDestructive       StatementClass = "destructive"
	StatementClassPrivilegeChanging StatementClass = "privilege_changing"
	StatementClassSessionChanging   StatementClass = "session_changing"
	StatementClassUnsupported       StatementClass = "unsupported"
)

type Statement struct {
	Class                 StatementClass
	Kind                  string
	TargetSchemas         []string
	HasUnqualifiedTargets bool
	HasBroadMutation      bool
	Reason                string
}

func ParseAndClassify(sql string) (Statement, error) {
	parseResult, err := pg_query.Parse(sql)
	if err != nil {
		return Statement{}, fmt.Errorf("parse SQL: %w", err)
	}

	if len(parseResult.GetStmts()) != 1 {
		return Statement{}, fmt.Errorf("exactly one SQL statement is allowed")
	}

	statement := classifyNode(parseResult.GetStmts()[0].GetStmt())
	statement.TargetSchemas = normalizeSchemas(statement.TargetSchemas)
	return statement, nil
}

func Fingerprint(sql string) (string, error) {
	fingerprint, err := pg_query.Fingerprint(sql)
	if err != nil {
		return "", fmt.Errorf("fingerprint SQL: %w", err)
	}
	return fingerprint, nil
}

func NormalizedHash(sql string) (string, error) {
	canonical, err := canonicalSQL(sql)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

func classifyNode(node *pg_query.Node) Statement {
	if node == nil {
		return unsupportedStatement("empty", "statement is empty")
	}

	switch {
	case node.GetSelectStmt() != nil:
		return classifySelect(node.GetSelectStmt())
	case node.GetVariableShowStmt() != nil:
		return Statement{Class: StatementClassReadOnly, Kind: "show"}
	case node.GetExplainStmt() != nil:
		return classifyExplain(node.GetExplainStmt())
	case node.GetInsertStmt() != nil:
		return classifyInsert(node.GetInsertStmt())
	case node.GetUpdateStmt() != nil:
		return classifyUpdate(node.GetUpdateStmt())
	case node.GetDeleteStmt() != nil:
		return classifyDelete(node.GetDeleteStmt())
	case node.GetMergeStmt() != nil:
		return classifyMerge(node.GetMergeStmt())
	case node.GetDropStmt() != nil:
		schemas, hasUnqualified := collectObjectSchemas(node.GetDropStmt().GetObjects(), node.GetDropStmt().GetRemoveType())
		return Statement{
			Class:                 StatementClassDestructive,
			Kind:                  "drop",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetTruncateStmt() != nil:
		schemas, hasUnqualified := collectRangeVarNodes(node.GetTruncateStmt().GetRelations())
		return Statement{
			Class:                 StatementClassDestructive,
			Kind:                  "truncate",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetAlterTableStmt() != nil:
		return classifyAlterTable(node.GetAlterTableStmt())
	case node.GetGrantStmt() != nil:
		schemas, hasUnqualified := collectObjectSchemas(node.GetGrantStmt().GetObjects(), node.GetGrantStmt().GetObjtype())
		kind := "grant"
		if !node.GetGrantStmt().GetIsGrant() {
			kind = "revoke"
		}
		return Statement{
			Class:                 StatementClassPrivilegeChanging,
			Kind:                  kind,
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetGrantRoleStmt() != nil:
		kind := "grant_role"
		if !node.GetGrantRoleStmt().GetIsGrant() {
			kind = "revoke_role"
		}
		return Statement{Class: StatementClassPrivilegeChanging, Kind: kind}
	case node.GetVariableSetStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "set", Reason: "session settings are not allowed"}
	case node.GetTransactionStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "transaction", Reason: "transaction control statements are not allowed"}
	case node.GetDiscardStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "discard", Reason: "session discard statements are not allowed"}
	case node.GetListenStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "listen", Reason: "LISTEN is not allowed"}
	case node.GetUnlistenStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "unlisten", Reason: "UNLISTEN is not allowed"}
	case node.GetLockStmt() != nil:
		return Statement{Class: StatementClassSessionChanging, Kind: "lock", Reason: "LOCK is not allowed"}
	case node.GetCreateStmt() != nil:
		schemas, hasUnqualified := collectRangeVar(node.GetCreateStmt().GetRelation())
		return Statement{
			Class:                 StatementClassAdministrative,
			Kind:                  "create",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetCreateSchemaStmt() != nil:
		schema := strings.TrimSpace(node.GetCreateSchemaStmt().GetSchemaname())
		return Statement{
			Class:         StatementClassAdministrative,
			Kind:          "create_schema",
			TargetSchemas: normalizeSchemas([]string{schema}),
		}
	case node.GetCreateTableAsStmt() != nil:
		schemas, hasUnqualified := collectIntoClause(node.GetCreateTableAsStmt().GetInto())
		return Statement{
			Class:                 StatementClassAdministrative,
			Kind:                  "create_table_as",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetIndexStmt() != nil:
		schemas, hasUnqualified := collectRangeVar(node.GetIndexStmt().GetRelation())
		return Statement{
			Class:                 StatementClassAdministrative,
			Kind:                  "create_index",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetViewStmt() != nil:
		schemas, hasUnqualified := collectRangeVar(node.GetViewStmt().GetView())
		return Statement{
			Class:                 StatementClassAdministrative,
			Kind:                  "create_view",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetRenameStmt() != nil:
		schemas, hasUnqualified := collectRangeVar(node.GetRenameStmt().GetRelation())
		return Statement{
			Class:                 StatementClassAdministrative,
			Kind:                  "rename",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	case node.GetCreateExtensionStmt() != nil:
		return Statement{
			Class: StatementClassAdministrative,
			Kind:  "create_extension",
		}
	default:
		return unsupportedStatement("unsupported", "statement type is not supported")
	}
}

func classifySelect(statement *pg_query.SelectStmt) Statement {
	if into := statement.GetIntoClause(); into != nil {
		schemas, hasUnqualified := collectIntoClause(into)
		return Statement{
			Class:                 StatementClassUnsupported,
			Kind:                  "select",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
			Reason:                "SELECT INTO is not allowed in read-only query mode",
		}
	}
	if len(statement.GetLockingClause()) > 0 {
		return Statement{
			Class:  StatementClassSessionChanging,
			Kind:   "select",
			Reason: "SELECT locking clauses are not allowed in read-only query mode",
		}
	}

	if left := statement.GetLarg(); left != nil {
		leftStatement := classifySelect(left)
		if leftStatement.Class != StatementClassReadOnly {
			return leftStatement
		}
	}
	if right := statement.GetRarg(); right != nil {
		rightStatement := classifySelect(right)
		if rightStatement.Class != StatementClassReadOnly {
			return rightStatement
		}
	}

	withSummary := classifyWithClause(statement.GetWithClause())
	if withSummary.Class != StatementClassReadOnly {
		return withSummary
	}

	return Statement{Class: StatementClassReadOnly, Kind: "select"}
}

func classifyExplain(statement *pg_query.ExplainStmt) Statement {
	if explainAnalyzeEnabled(statement) {
		return Statement{
			Class:  StatementClassUnsupported,
			Kind:   "explain",
			Reason: "EXPLAIN ANALYZE is not allowed in read-only query mode",
		}
	}

	target := classifyNode(statement.GetQuery())
	if target.Class != StatementClassReadOnly {
		return target
	}

	return Statement{Class: StatementClassReadOnly, Kind: "explain"}
}

func classifyInsert(statement *pg_query.InsertStmt) Statement {
	result := Statement{Class: StatementClassMutating, Kind: "insert"}
	result.TargetSchemas, result.HasUnqualifiedTargets = collectRangeVar(statement.GetRelation())

	withSummary := classifyWithClause(statement.GetWithClause())
	result = mergeChildStatement(result, withSummary)

	source := classifyChildQuery(statement.GetSelectStmt())
	result = mergeChildStatement(result, source)

	return result
}

func classifyUpdate(statement *pg_query.UpdateStmt) Statement {
	result := Statement{Class: StatementClassMutating, Kind: "update"}
	result.TargetSchemas, result.HasUnqualifiedTargets = collectRangeVar(statement.GetRelation())
	result.HasBroadMutation = statement.GetWhereClause() == nil
	return mergeChildStatement(result, classifyWithClause(statement.GetWithClause()))
}

func classifyDelete(statement *pg_query.DeleteStmt) Statement {
	result := Statement{Class: StatementClassMutating, Kind: "delete"}
	result.TargetSchemas, result.HasUnqualifiedTargets = collectRangeVar(statement.GetRelation())
	result.HasBroadMutation = statement.GetWhereClause() == nil
	return mergeChildStatement(result, classifyWithClause(statement.GetWithClause()))
}

func classifyMerge(statement *pg_query.MergeStmt) Statement {
	result := Statement{Class: StatementClassMutating, Kind: "merge"}
	result.TargetSchemas, result.HasUnqualifiedTargets = collectRangeVar(statement.GetRelation())
	return mergeChildStatement(result, classifyWithClause(statement.GetWithClause()))
}

func classifyAlterTable(statement *pg_query.AlterTableStmt) Statement {
	schemas, hasUnqualified := collectRangeVar(statement.GetRelation())
	isDestructive := false

	for _, commandNode := range statement.GetCmds() {
		command := commandNode.GetAlterTableCmd()
		if command == nil {
			continue
		}
		if strings.Contains(strings.ToUpper(command.GetSubtype().String()), "DROP") {
			isDestructive = true
			break
		}
	}

	if isDestructive {
		return Statement{
			Class:                 StatementClassDestructive,
			Kind:                  "alter_table",
			TargetSchemas:         schemas,
			HasUnqualifiedTargets: hasUnqualified,
		}
	}

	return Statement{
		Class:                 StatementClassAdministrative,
		Kind:                  "alter_table",
		TargetSchemas:         schemas,
		HasUnqualifiedTargets: hasUnqualified,
	}
}

func classifyWithClause(withClause *pg_query.WithClause) Statement {
	if withClause == nil {
		return Statement{Class: StatementClassReadOnly}
	}

	summary := Statement{Class: StatementClassReadOnly}
	for _, cteNode := range withClause.GetCtes() {
		cte := cteNode.GetCommonTableExpr()
		if cte == nil {
			return unsupportedStatement("with", "unsupported CTE node in WITH clause")
		}
		summary = mergeSummary(summary, classifyNode(cte.GetCtequery()))
	}

	return summary
}

func classifyChildQuery(node *pg_query.Node) Statement {
	if node == nil {
		return Statement{Class: StatementClassReadOnly}
	}
	return classifyNode(node)
}

func mergeSummary(current, child Statement) Statement {
	if child.Class == "" {
		return current
	}

	current.TargetSchemas = append(current.TargetSchemas, child.TargetSchemas...)
	current.HasUnqualifiedTargets = current.HasUnqualifiedTargets || child.HasUnqualifiedTargets
	current.HasBroadMutation = current.HasBroadMutation || child.HasBroadMutation

	if statementRank(child.Class) > statementRank(current.Class) {
		current.Class = child.Class
		current.Kind = child.Kind
		current.Reason = child.Reason
	}

	return current
}

func mergeChildStatement(base, child Statement) Statement {
	if child.Class == "" || child.Class == StatementClassReadOnly {
		base.TargetSchemas = append(base.TargetSchemas, child.TargetSchemas...)
		base.HasUnqualifiedTargets = base.HasUnqualifiedTargets || child.HasUnqualifiedTargets
		base.HasBroadMutation = base.HasBroadMutation || child.HasBroadMutation
		base.TargetSchemas = normalizeSchemas(base.TargetSchemas)
		return base
	}

	if child.Class == StatementClassMutating || child.Class == StatementClassAdministrative {
		base.TargetSchemas = append(base.TargetSchemas, child.TargetSchemas...)
		base.HasUnqualifiedTargets = base.HasUnqualifiedTargets || child.HasUnqualifiedTargets
		base.HasBroadMutation = base.HasBroadMutation || child.HasBroadMutation
		base.TargetSchemas = normalizeSchemas(base.TargetSchemas)
		return base
	}

	child.TargetSchemas = normalizeSchemas(append(child.TargetSchemas, base.TargetSchemas...))
	child.HasUnqualifiedTargets = child.HasUnqualifiedTargets || base.HasUnqualifiedTargets
	child.HasBroadMutation = child.HasBroadMutation || base.HasBroadMutation
	return child
}

func collectRangeVar(rangeVar *pg_query.RangeVar) ([]string, bool) {
	if rangeVar == nil {
		return nil, false
	}

	schema := strings.TrimSpace(rangeVar.GetSchemaname())
	if schema == "" {
		return nil, true
	}

	return []string{schema}, false
}

func collectIntoClause(into *pg_query.IntoClause) ([]string, bool) {
	if into == nil {
		return nil, false
	}
	return collectRangeVar(into.GetRel())
}

func collectRangeVarNodes(nodes []*pg_query.Node) ([]string, bool) {
	schemas := make([]string, 0, len(nodes))
	hasUnqualified := false

	for _, node := range nodes {
		rangeVar := node.GetRangeVar()
		if rangeVar == nil {
			continue
		}

		nodeSchemas, nodeHasUnqualified := collectRangeVar(rangeVar)
		schemas = append(schemas, nodeSchemas...)
		hasUnqualified = hasUnqualified || nodeHasUnqualified
	}

	return normalizeSchemas(schemas), hasUnqualified
}

func collectObjectSchemas(objects []*pg_query.Node, objectType pg_query.ObjectType) ([]string, bool) {
	schemas := make([]string, 0, len(objects))
	hasUnqualified := false

	for _, objectNode := range objects {
		if rangeVar := objectNode.GetRangeVar(); rangeVar != nil {
			nodeSchemas, nodeHasUnqualified := collectRangeVar(rangeVar)
			schemas = append(schemas, nodeSchemas...)
			hasUnqualified = hasUnqualified || nodeHasUnqualified
			continue
		}

		objectList := objectNode.GetList()
		if objectList == nil {
			hasUnqualified = true
			continue
		}

		items := objectList.GetItems()
		if objectType == pg_query.ObjectType_OBJECT_SCHEMA {
			if len(items) == 0 || items[0].GetString_() == nil {
				hasUnqualified = true
				continue
			}
			schemas = append(schemas, items[0].GetString_().GetSval())
			continue
		}

		if len(items) < 2 || items[0].GetString_() == nil {
			hasUnqualified = true
			continue
		}

		schemas = append(schemas, items[0].GetString_().GetSval())
	}

	return normalizeSchemas(schemas), hasUnqualified
}

func explainAnalyzeEnabled(statement *pg_query.ExplainStmt) bool {
	for _, optionNode := range statement.GetOptions() {
		option := optionNode.GetDefElem()
		if option == nil || !strings.EqualFold(option.GetDefname(), "analyze") {
			continue
		}

		enabled, ok := explainOptionBool(option.GetArg())
		if !ok {
			return true
		}
		return enabled
	}

	return false
}

func explainOptionBool(node *pg_query.Node) (bool, bool) {
	if node == nil {
		return true, true
	}

	switch {
	case node.GetBoolean() != nil:
		return node.GetBoolean().GetBoolval(), true
	case node.GetAConst() != nil && node.GetAConst().GetBoolval() != nil:
		return node.GetAConst().GetBoolval().GetBoolval(), true
	case node.GetString_() != nil:
		value := strings.ToLower(strings.TrimSpace(node.GetString_().GetSval()))
		switch value {
		case "true", "on":
			return true, true
		case "false", "off":
			return false, true
		}
	case node.GetAConst() != nil && node.GetAConst().GetSval() != nil:
		value := strings.ToLower(strings.TrimSpace(node.GetAConst().GetSval().GetSval()))
		switch value {
		case "true", "on":
			return true, true
		case "false", "off":
			return false, true
		}
	}

	return false, false
}

func normalizeSchemas(schemas []string) []string {
	if len(schemas) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(schemas))
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		normalized := strings.ToLower(strings.TrimSpace(schema))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	sort.Strings(out)
	return out
}

func unsupportedStatement(kind, reason string) Statement {
	return Statement{
		Class:  StatementClassUnsupported,
		Kind:   kind,
		Reason: reason,
	}
}

func statementRank(class StatementClass) int {
	switch class {
	case StatementClassReadOnly:
		return 0
	case StatementClassMutating:
		return 1
	case StatementClassAdministrative:
		return 2
	case StatementClassDestructive, StatementClassPrivilegeChanging:
		return 3
	case StatementClassSessionChanging:
		return 4
	case StatementClassUnsupported:
		return 5
	default:
		return 0
	}
}

func RequiresConfirmation(statement Statement) bool {
	return statement.Class == StatementClassDestructive || statement.Class == StatementClassPrivilegeChanging
}

func canonicalSQL(sql string) (string, error) {
	parseResult, err := pg_query.Parse(sql)
	if err != nil {
		return "", fmt.Errorf("parse SQL: %w", err)
	}

	canonical, err := pg_query.Deparse(parseResult)
	if err == nil {
		return strings.TrimSpace(canonical), nil
	}

	return strings.Join(strings.Fields(sql), " "), nil
}
