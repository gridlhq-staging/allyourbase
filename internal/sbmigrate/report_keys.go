package sbmigrate

import (
	"encoding/json"
	"sort"
	"strconv"
)

func tableKey(schema, name string) string {
	data, err := json.Marshal([2]string{schema, name})
	if err != nil {
		return schemaQualifiedName(schema, name)
	}
	return string(data)
}

func functionKey(schema, name, identityArguments string) string {
	data, err := json.Marshal([3]string{schema, name, identityArguments})
	if err != nil {
		return FunctionIdentity{SchemaName: schema, Name: name, IdentityArguments: identityArguments}.QualifiedName()
	}
	return string(data)
}

func triggerKey(
	eventTrigger bool,
	tableSchema,
	tableName,
	name,
	handlerSchema,
	handlerName,
	handlerIdentityArguments string,
) string {
	data, err := json.Marshal([7]string{
		strconv.FormatBool(eventTrigger),
		tableSchema,
		tableName,
		name,
		handlerSchema,
		handlerName,
		handlerIdentityArguments,
	})
	if err != nil {
		return TriggerIdentity{
			EventTrigger:             eventTrigger,
			TableSchemaName:          tableSchema,
			TableName:                tableName,
			Name:                     name,
			HandlerSchemaName:        handlerSchema,
			HandlerName:              handlerName,
			HandlerIdentityArguments: handlerIdentityArguments,
		}.DisplayName()
	}
	return string(data)
}

func skippedTableReport(key, reason string) SkippedTableReport {
	schema, table, ok := tableKeyParts(key)
	if !ok {
		return SkippedTableReport{TableName: key, Reason: reason}
	}
	return SkippedTableReport{
		SchemaName: schema,
		TableName:  table,
		Reason:     reason,
	}
}

func skippedFunctionReport(key, reason string) SkippedFunctionReport {
	schema, name, identityArguments, ok := functionKeyParts(key)
	if !ok {
		return SkippedFunctionReport{Name: key, Reason: reason}
	}
	return SkippedFunctionReport{
		SchemaName:        schema,
		Name:              name,
		IdentityArguments: identityArguments,
		Reason:            reason,
	}
}

func skippedTriggerReport(key, reason string) SkippedTriggerReport {
	identity, ok := triggerKeyParts(key)
	if !ok {
		return SkippedTriggerReport{Name: key, Reason: reason}
	}
	return SkippedTriggerReport{
		EventTrigger:             identity.EventTrigger,
		TableSchemaName:          identity.TableSchemaName,
		TableName:                identity.TableName,
		Name:                     identity.Name,
		HandlerSchemaName:        identity.HandlerSchemaName,
		HandlerName:              identity.HandlerName,
		HandlerIdentityArguments: identity.HandlerIdentityArguments,
		Reason:                   reason,
	}
}

func tableKeyParts(key string) (string, string, bool) {
	var parts [2]string
	if err := json.Unmarshal([]byte(key), &parts); err != nil {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func functionKeyParts(key string) (string, string, string, bool) {
	var parts [3]string
	if err := json.Unmarshal([]byte(key), &parts); err != nil {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func triggerKeyParts(key string) (TriggerIdentity, bool) {
	var parts []string
	if err := json.Unmarshal([]byte(key), &parts); err != nil {
		return TriggerIdentity{}, false
	}
	switch len(parts) {
	case 7:
		return TriggerIdentity{
			EventTrigger:             parts[0] == "true",
			TableSchemaName:          parts[1],
			TableName:                parts[2],
			Name:                     parts[3],
			HandlerSchemaName:        parts[4],
			HandlerName:              parts[5],
			HandlerIdentityArguments: parts[6],
		}, true
	case 6:
		return TriggerIdentity{
			TableSchemaName:          parts[0],
			TableName:                parts[1],
			Name:                     parts[2],
			HandlerSchemaName:        parts[3],
			HandlerName:              parts[4],
			HandlerIdentityArguments: parts[5],
		}, true
	default:
		return TriggerIdentity{}, false
	}
}

func sortedSkippedTableReports(skipped SkippedTableReasons) []SkippedTableReport {
	reports := make([]SkippedTableReport, 0, len(skipped))
	for key, reason := range skipped {
		reports = append(reports, skippedTableReport(key, reason))
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].SchemaName != reports[j].SchemaName {
			return reports[i].SchemaName < reports[j].SchemaName
		}
		if reports[i].TableName != reports[j].TableName {
			return reports[i].TableName < reports[j].TableName
		}
		return reports[i].Reason < reports[j].Reason
	})
	return reports
}

func sortedSkippedFunctionReports(skipped SkippedFunctionReasons) []SkippedFunctionReport {
	reports := make([]SkippedFunctionReport, 0, len(skipped))
	for key, reason := range skipped {
		reports = append(reports, skippedFunctionReport(key, reason))
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].SchemaName != reports[j].SchemaName {
			return reports[i].SchemaName < reports[j].SchemaName
		}
		if reports[i].Name != reports[j].Name {
			return reports[i].Name < reports[j].Name
		}
		if reports[i].IdentityArguments != reports[j].IdentityArguments {
			return reports[i].IdentityArguments < reports[j].IdentityArguments
		}
		return reports[i].Reason < reports[j].Reason
	})
	return reports
}

func sortedSkippedTriggerReports(skipped SkippedTriggerReasons) []SkippedTriggerReport {
	reports := make([]SkippedTriggerReport, 0, len(skipped))
	for key, reason := range skipped {
		reports = append(reports, skippedTriggerReport(key, reason))
	}
	sort.Slice(reports, func(i, j int) bool {
		left := reports[i].Identity()
		right := reports[j].Identity()
		if left.EventTrigger != right.EventTrigger {
			return !left.EventTrigger
		}
		if left.TableSchemaName != right.TableSchemaName {
			return left.TableSchemaName < right.TableSchemaName
		}
		if left.TableName != right.TableName {
			return left.TableName < right.TableName
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.HandlerQualifiedName() != right.HandlerQualifiedName() {
			return left.HandlerQualifiedName() < right.HandlerQualifiedName()
		}
		return reports[i].Reason < reports[j].Reason
	})
	return reports
}
