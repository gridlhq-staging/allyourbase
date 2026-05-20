// Package schemadiff Compares two database snapshots and emits an ordered sequence of schema changes (extensions, enums, tables) needed to transform the old schema to the new one.
package schemadiff

import "sort"

// ChangeType identifies the kind of schema change.
type ChangeType string

const (
	ChangeCreateTable         ChangeType = "CreateTable"
	ChangeDropTable           ChangeType = "DropTable"
	ChangeAddColumn           ChangeType = "AddColumn"
	ChangeDropColumn          ChangeType = "DropColumn"
	ChangeAlterColumnType     ChangeType = "AlterColumnType"
	ChangeAlterColumnDefault  ChangeType = "AlterColumnDefault"
	ChangeAlterColumnNullable ChangeType = "AlterColumnNullable"
	ChangeCreateIndex         ChangeType = "CreateIndex"
	ChangeDropIndex           ChangeType = "DropIndex"
	ChangeAddForeignKey       ChangeType = "AddForeignKey"
	ChangeDropForeignKey      ChangeType = "DropForeignKey"
	ChangeAddCheckConstraint  ChangeType = "AddCheckConstraint"
	ChangeDropCheckConstraint ChangeType = "DropCheckConstraint"
	ChangeCreateEnum          ChangeType = "CreateEnum"
	ChangeAlterEnumAddValue   ChangeType = "AlterEnumAddValue"
	ChangeAddRLSPolicy        ChangeType = "AddRLSPolicy"
	ChangeDropRLSPolicy       ChangeType = "DropRLSPolicy"
	ChangeEnableExtension     ChangeType = "EnableExtension"
	ChangeDisableExtension    ChangeType = "DisableExtension"
)

// Change is a single schema change with enough metadata to generate SQL.
// represents a single schema change with fields populated according to its Type, containing metadata needed to generate SQL migration statements.
type Change struct {
	Type ChangeType

	// Table-level fields.
	SchemaName string
	TableName  string
	TableKind  string

	// Column fields.
	ColumnName   string
	OldTypeName  string
	NewTypeName  string
	OldDefault   string
	NewDefault   string
	OldNullable  bool
	NewNullable  bool
	IsPrimaryKey bool
	AllColumns   []SnapColumn // used for CreateTable

	// Index fields.
	Index SnapIndex

	// Foreign key fields.
	ForeignKey SnapForeignKey

	// Check constraint fields.
	CheckConstraint SnapCheckConstraint

	// Enum fields.
	EnumSchema string
	EnumName   string
	EnumValues []string // all values for CreateEnum
	NewValue   string   // single new value for AlterEnumAddValue

	// RLS policy fields.
	RLSPolicy SnapRLSPolicy

	// Extension fields.
	ExtensionName    string
	ExtensionVersion string
}

// ChangeSet is an ordered list of schema changes.
type ChangeSet []Change

// Diff compares two snapshots and returns the changes needed to transform old into new.
// Changes are returned in a stable, topologically sensible order.
func Diff(old, new *Snapshot) ChangeSet {
	if old == nil {
		old = &Snapshot{}
	}
	if new == nil {
		new = &Snapshot{}
	}

	var cs ChangeSet

	// Extensions
	cs = append(cs, diffExtensions(old, new)...)
	// Enums (must come before tables that reference them)
	cs = append(cs, diffEnums(old, new)...)
	// Tables
	cs = append(cs, diffTables(old, new)...)

	return cs
}

// compares extensions between old and new snapshots, returning changes for newly enabled and disabled extensions.
func diffExtensions(old, new *Snapshot) []Change {
	var changes []Change

	newExts := make(map[string]SnapExtension, len(new.Extensions))
	for _, e := range new.Extensions {
		newExts[e.Name] = e
	}
	oldExts := make(map[string]SnapExtension, len(old.Extensions))
	for _, e := range old.Extensions {
		oldExts[e.Name] = e
	}

	// Enabled extensions (in new but not old)
	names := sortedStringKeys(newExts)
	for _, name := range names {
		if _, exists := oldExts[name]; !exists {
			e := newExts[name]
			changes = append(changes, Change{
				Type:             ChangeEnableExtension,
				ExtensionName:    e.Name,
				ExtensionVersion: e.Version,
			})
		}
	}

	// Disabled extensions (in old but not new)
	names = sortedStringKeys(oldExts)
	for _, name := range names {
		if _, exists := newExts[name]; !exists {
			e := oldExts[name]
			changes = append(changes, Change{
				Type:             ChangeDisableExtension,
				ExtensionName:    e.Name,
				ExtensionVersion: e.Version,
			})
		}
	}

	return changes
}

// compares enums between snapshots, emitting ChangeCreateEnum for new enums and ChangeAlterEnumAddValue for new values; does not emit drop changes due to PostgreSQL's CASCADE requirement.
func diffEnums(old, new *Snapshot) []Change {
	var changes []Change

	newEnumMap := make(map[string]SnapEnum, len(new.Enums))
	for _, e := range new.Enums {
		newEnumMap[e.Schema+"."+e.Name] = e
	}
	oldEnumMap := make(map[string]SnapEnum, len(old.Enums))
	for _, e := range old.Enums {
		oldEnumMap[e.Schema+"."+e.Name] = e
	}

	keys := sortedStringKeys(newEnumMap)
	for _, key := range keys {
		ne := newEnumMap[key]
		oe, exists := oldEnumMap[key]
		if !exists {
			// New enum.
			changes = append(changes, Change{
				Type:       ChangeCreateEnum,
				EnumSchema: ne.Schema,
				EnumName:   ne.Name,
				EnumValues: ne.Values,
			})
			continue
		}
		// Check for new values (enum values can only be added, not removed, in PG).
		oldVals := make(map[string]bool, len(oe.Values))
		for _, v := range oe.Values {
			oldVals[v] = true
		}
		for _, v := range ne.Values {
			if !oldVals[v] {
				changes = append(changes, Change{
					Type:       ChangeAlterEnumAddValue,
					EnumSchema: ne.Schema,
					EnumName:   ne.Name,
					NewValue:   v,
				})
			}
		}
	}
	// Note: we do not emit DropEnum — PostgreSQL requires CASCADE which is destructive.
	// Callers can handle that as needed.

	return changes
}

// sortedStringKeys returns sorted keys from a map[string]T.
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
