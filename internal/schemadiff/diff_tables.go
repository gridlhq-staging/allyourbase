package schemadiff

// compares tables between snapshots, handling created and dropped tables and delegating to helper functions for columns, indexes, foreign keys, check constraints, and RLS policies in existing tables.
func diffTables(old, new *Snapshot) []Change {
	var changes []Change

	newTableMap := make(map[string]SnapTable, len(new.Tables))
	for _, t := range new.Tables {
		newTableMap[t.FullName()] = t
	}
	oldTableMap := make(map[string]SnapTable, len(old.Tables))
	for _, t := range old.Tables {
		oldTableMap[t.FullName()] = t
	}

	keys := sortedStringKeys(newTableMap)
	for _, key := range keys {
		nt := newTableMap[key]
		if _, exists := oldTableMap[key]; !exists {
			changes = append(changes, Change{
				Type:       ChangeCreateTable,
				SchemaName: nt.Schema,
				TableName:  nt.Name,
				TableKind:  nt.Kind,
				AllColumns: nt.Columns,
			})
			for _, idx := range nt.Indexes {
				if !idx.IsPrimary {
					changes = append(changes, Change{
						Type:       ChangeCreateIndex,
						SchemaName: nt.Schema,
						TableName:  nt.Name,
						Index:      idx,
					})
				}
			}
			for _, fk := range nt.ForeignKeys {
				changes = append(changes, Change{
					Type:       ChangeAddForeignKey,
					SchemaName: nt.Schema,
					TableName:  nt.Name,
					ForeignKey: fk,
				})
			}
			for _, cc := range nt.CheckConstraints {
				changes = append(changes, Change{
					Type:            ChangeAddCheckConstraint,
					SchemaName:      nt.Schema,
					TableName:       nt.Name,
					CheckConstraint: cc,
				})
			}
			for _, pol := range nt.RLSPolicies {
				changes = append(changes, Change{
					Type:       ChangeAddRLSPolicy,
					SchemaName: nt.Schema,
					TableName:  nt.Name,
					RLSPolicy:  pol,
				})
			}
			continue
		}

		ot := oldTableMap[key]
		changes = append(changes, diffColumns(ot, nt)...)
		changes = append(changes, diffIndexes(ot, nt)...)
		changes = append(changes, diffForeignKeys(ot, nt)...)
		changes = append(changes, diffCheckConstraints(ot, nt)...)
		changes = append(changes, diffRLSPolicies(ot, nt)...)
	}

	keys = sortedStringKeys(oldTableMap)
	for _, key := range keys {
		if _, exists := newTableMap[key]; !exists {
			ot := oldTableMap[key]
			changes = append(changes, Change{
				Type:       ChangeDropTable,
				SchemaName: ot.Schema,
				TableName:  ot.Name,
			})
		}
	}

	return changes
}

// compares columns between tables, returning changes for added, dropped, and altered columns including type, default expression, and nullability modifications.
func diffColumns(old, new SnapTable) []Change {
	var changes []Change

	newColMap := make(map[string]SnapColumn, len(new.Columns))
	for _, c := range new.Columns {
		newColMap[c.Name] = c
	}
	oldColMap := make(map[string]SnapColumn, len(old.Columns))
	for _, c := range old.Columns {
		oldColMap[c.Name] = c
	}

	for _, nc := range new.Columns {
		if _, exists := oldColMap[nc.Name]; !exists {
			changes = append(changes, Change{
				Type:         ChangeAddColumn,
				SchemaName:   new.Schema,
				TableName:    new.Name,
				ColumnName:   nc.Name,
				NewTypeName:  nc.TypeName,
				NewDefault:   nc.DefaultExpr,
				NewNullable:  nc.IsNullable,
				IsPrimaryKey: nc.IsPrimaryKey,
			})
		}
	}

	for _, oc := range old.Columns {
		if _, exists := newColMap[oc.Name]; !exists {
			changes = append(changes, Change{
				Type:        ChangeDropColumn,
				SchemaName:  old.Schema,
				TableName:   old.Name,
				ColumnName:  oc.Name,
				OldTypeName: oc.TypeName,
			})
		}
	}

	for _, nc := range new.Columns {
		oc, exists := oldColMap[nc.Name]
		if !exists {
			continue
		}
		if oc.TypeName != nc.TypeName {
			changes = append(changes, Change{
				Type:        ChangeAlterColumnType,
				SchemaName:  new.Schema,
				TableName:   new.Name,
				ColumnName:  nc.Name,
				OldTypeName: oc.TypeName,
				NewTypeName: nc.TypeName,
			})
		}
		if oc.DefaultExpr != nc.DefaultExpr {
			changes = append(changes, Change{
				Type:       ChangeAlterColumnDefault,
				SchemaName: new.Schema,
				TableName:  new.Name,
				ColumnName: nc.Name,
				OldDefault: oc.DefaultExpr,
				NewDefault: nc.DefaultExpr,
			})
		}
		if oc.IsNullable != nc.IsNullable {
			changes = append(changes, Change{
				Type:        ChangeAlterColumnNullable,
				SchemaName:  new.Schema,
				TableName:   new.Name,
				ColumnName:  nc.Name,
				OldNullable: oc.IsNullable,
				NewNullable: nc.IsNullable,
			})
		}
	}

	return changes
}

// compares indexes between tables, returning changes for created and dropped indexes, excluding primary key indexes.
func diffIndexes(old, new SnapTable) []Change {
	var changes []Change

	newIdxMap := make(map[string]SnapIndex, len(new.Indexes))
	for _, idx := range new.Indexes {
		newIdxMap[idx.Name] = idx
	}
	oldIdxMap := make(map[string]SnapIndex, len(old.Indexes))
	for _, idx := range old.Indexes {
		oldIdxMap[idx.Name] = idx
	}

	names := sortedStringKeys(newIdxMap)
	for _, name := range names {
		if _, exists := oldIdxMap[name]; !exists {
			idx := newIdxMap[name]
			if !idx.IsPrimary {
				changes = append(changes, Change{
					Type:       ChangeCreateIndex,
					SchemaName: new.Schema,
					TableName:  new.Name,
					Index:      idx,
				})
			}
		}
	}

	names = sortedStringKeys(oldIdxMap)
	for _, name := range names {
		if _, exists := newIdxMap[name]; !exists {
			idx := oldIdxMap[name]
			if !idx.IsPrimary {
				changes = append(changes, Change{
					Type:       ChangeDropIndex,
					SchemaName: old.Schema,
					TableName:  old.Name,
					Index:      idx,
				})
			}
		}
	}

	return changes
}

// compares foreign keys between tables, returning changes for added and dropped constraints.
func diffForeignKeys(old, new SnapTable) []Change {
	var changes []Change

	newFKMap := make(map[string]SnapForeignKey, len(new.ForeignKeys))
	for _, fk := range new.ForeignKeys {
		newFKMap[fk.ConstraintName] = fk
	}
	oldFKMap := make(map[string]SnapForeignKey, len(old.ForeignKeys))
	for _, fk := range old.ForeignKeys {
		oldFKMap[fk.ConstraintName] = fk
	}

	names := sortedStringKeys(newFKMap)
	for _, name := range names {
		if _, exists := oldFKMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeAddForeignKey,
				SchemaName: new.Schema,
				TableName:  new.Name,
				ForeignKey: newFKMap[name],
			})
		}
	}

	names = sortedStringKeys(oldFKMap)
	for _, name := range names {
		if _, exists := newFKMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeDropForeignKey,
				SchemaName: old.Schema,
				TableName:  old.Name,
				ForeignKey: oldFKMap[name],
			})
		}
	}

	return changes
}

// compares check constraints between tables, returning changes for added and dropped constraints.
func diffCheckConstraints(old, new SnapTable) []Change {
	var changes []Change

	newCCMap := make(map[string]SnapCheckConstraint, len(new.CheckConstraints))
	for _, cc := range new.CheckConstraints {
		newCCMap[cc.Name] = cc
	}
	oldCCMap := make(map[string]SnapCheckConstraint, len(old.CheckConstraints))
	for _, cc := range old.CheckConstraints {
		oldCCMap[cc.Name] = cc
	}

	names := sortedStringKeys(newCCMap)
	for _, name := range names {
		if _, exists := oldCCMap[name]; !exists {
			changes = append(changes, Change{
				Type:            ChangeAddCheckConstraint,
				SchemaName:      new.Schema,
				TableName:       new.Name,
				CheckConstraint: newCCMap[name],
			})
		}
	}

	names = sortedStringKeys(oldCCMap)
	for _, name := range names {
		if _, exists := newCCMap[name]; !exists {
			changes = append(changes, Change{
				Type:            ChangeDropCheckConstraint,
				SchemaName:      old.Schema,
				TableName:       old.Name,
				CheckConstraint: oldCCMap[name],
			})
		}
	}

	return changes
}

// compares Row-Level Security policies between tables, returning changes for added and dropped policies.
func diffRLSPolicies(old, new SnapTable) []Change {
	var changes []Change

	newPolMap := make(map[string]SnapRLSPolicy, len(new.RLSPolicies))
	for _, pol := range new.RLSPolicies {
		newPolMap[pol.Name] = pol
	}
	oldPolMap := make(map[string]SnapRLSPolicy, len(old.RLSPolicies))
	for _, pol := range old.RLSPolicies {
		oldPolMap[pol.Name] = pol
	}

	names := sortedStringKeys(newPolMap)
	for _, name := range names {
		if _, exists := oldPolMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeAddRLSPolicy,
				SchemaName: new.Schema,
				TableName:  new.Name,
				RLSPolicy:  newPolMap[name],
			})
		}
	}

	names = sortedStringKeys(oldPolMap)
	for _, name := range names {
		if _, exists := newPolMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeDropRLSPolicy,
				SchemaName: old.Schema,
				TableName:  old.Name,
				RLSPolicy:  oldPolMap[name],
			})
		}
	}

	return changes
}
