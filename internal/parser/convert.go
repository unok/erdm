package parser

import "github.com/unok/erdm/internal/model"

// toSchema は parserBuilder の中間表現を新しい model.Schema へ変換する。
// 旧 erdm.go の ErdM/Table/Column フィールド名から新モデル名への対応は
// design.md §テンプレートと新モデルのフィールド対応表 に従う:
//
//	旧 Table.TitleReal              -> model.Table.Name
//	旧 Table.Title                  -> model.Table.LogicalName
//	旧 Column.TitleReal             -> model.Column.Name
//	旧 Column.Title                 -> model.Column.LogicalName
//	旧 Column.Relation.TableNameReal-> model.Column.FK.TargetTable
//	旧 Column.IndexIndexes           -> model.Column.IndexRefs
//	旧 Index.Title                  -> model.Index.Name
//
// Schema.Groups は全テーブルの Groups から登場順を保持して集約する
// （要件 2.7、重複は除外）。Schema.Validate() の呼び出しは行わず、
// 呼び出し側に委ねる（design.md §C3、本バッチ計画）。
func (p *parserBuilder) toSchema() *model.Schema {
	tables := make([]model.Table, 0, len(p.tables))
	for ti := range p.tables {
		tables = append(tables, p.convertTable(&p.tables[ti]))
	}
	groups := deriveSchemaGroups(p.tables)
	return &model.Schema{
		Title:  p.title,
		Tables: tables,
		Groups: groups,
	}
}

// convertTable は parserTable を model.Table へ変換する。
func (p *parserBuilder) convertTable(t *parserTable) model.Table {
	cols := make([]model.Column, 0, len(t.columns))
	for ci := range t.columns {
		cols = append(cols, convertColumn(&t.columns[ci]))
	}
	pks := append([]int(nil), t.primaryKeys...)
	indexes := make([]model.Index, 0, len(t.indexes))
	for _, idx := range t.indexes {
		indexes = append(indexes, model.Index{
			Name:     idx.title,
			Columns:  append([]string(nil), idx.columns...),
			IsUnique: idx.isUnique,
		})
	}
	groups := append([]string(nil), t.groups...)
	return model.Table{
		Name:        t.titleReal,
		LogicalName: t.title,
		Columns:     cols,
		PrimaryKeys: pks,
		Indexes:     indexes,
		Groups:      groups,
	}
}

// convertColumn は parserColumn を model.Column へ変換する。FK は
// relation.tableNameReal が非空のときのみ生成し、それ以外は nil。
func convertColumn(c *parserColumn) model.Column {
	var fk *model.FK
	if c.relation.tableNameReal != "" {
		fk = &model.FK{
			TargetTable:            c.relation.tableNameReal,
			CardinalitySource:      c.relation.cardinalitySource,
			CardinalityDestination: c.relation.cardinalityDestination,
		}
	}
	return model.Column{
		Name:         c.titleReal,
		LogicalName:  c.title,
		Type:         c.colType,
		AllowNull:    c.allowNull,
		IsUnique:     c.isUnique,
		IsPrimaryKey: c.isPrimaryKey,
		Default:      c.defaultExpr,
		Comments:     append([]string(nil), c.comments...),
		WithoutErd:   c.withoutErd,
		FK:           fk,
		IndexRefs:    append([]int(nil), c.indexIndexes...),
	}
}

// deriveSchemaGroups は全テーブルの Groups から登場順を保ったまま重複除外して
// Schema.Groups を構築する（要件 2.7）。
func deriveSchemaGroups(tables []parserTable) []string {
	seen := make(map[string]struct{})
	out := []string{}
	for _, t := range tables {
		for _, g := range t.groups {
			if _, ok := seen[g]; ok {
				continue
			}
			seen[g] = struct{}{}
			out = append(out, g)
		}
	}
	return out
}
