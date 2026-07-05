package model

import (
	"errors"
	"fmt"
	"strings"
)

// Schema はスキーマ全体を表す集約ルート。
//
// Title はスキーマタイトル、Tables は登場順で並ぶテーブル一覧、
// Groups は `.erdm` 内で最初に出現した順に蓄積されるグループ名集合
// （要件 2.7：登場順保持）。
type Schema struct {
	Title  string
	Tables []Table
	Groups []string
}

// ValidationError は Schema.Validate が検出した不変条件違反を表す。
//
// Path は違反箇所を識別する JSON Pointer 風の文字列（例:
//
//	"Tables[0].Columns[2].FK.TargetTable", "Tables[1].PrimaryKeys[0]"）。
//
// Cause は人間可読の説明文。Schema.Validate は複数違反を検出した場合
// errors.Join でまとめて返す。
type ValidationError struct {
	Path  string
	Cause string
}

// Error は標準エラーインターフェースの実装。
func (e *ValidationError) Error() string {
	return fmt.Sprintf("model: invalid schema at %s: %s", e.Path, e.Cause)
}

// Validate は Schema の不変条件をすべて検査する。違反があれば errors.Join で
// まとめた error を返す（個別の違反は *ValidationError 型）。違反がなければ nil。
//
// 検査する不変条件:
//   - Schema.Tables 内の Table.Name は重複しない。
//   - Table.Name は空でない。
//   - Table.Groups の各要素は空文字列でない（要件 2.5：空配列は文法側で禁止、
//     ここでは構築済みモデルが空文字列を含まないことを担保）。
//   - Table.PrimaryKeys の各添字は len(Columns) 未満。
//   - Column.FK.TargetTable は Schema 内の Table.Name と一致する。
//   - Schema.Groups に登場するグループ名は空文字列でない。
func (s *Schema) Validate() error {
	if s == nil {
		return &ValidationError{Path: "", Cause: "schema is nil"}
	}
	var errs []error

	tableNames := make(map[string]int, len(s.Tables))
	for i, t := range s.Tables {
		path := fmt.Sprintf("Tables[%d]", i)
		if strings.TrimSpace(t.Name) == "" {
			errs = append(errs, &ValidationError{
				Path:  path + ".Name",
				Cause: "table name is empty",
			})
		}
		if prev, ok := tableNames[t.Name]; ok && t.Name != "" {
			errs = append(errs, &ValidationError{
				Path:  path + ".Name",
				Cause: fmt.Sprintf("duplicate table name (also at Tables[%d])", prev),
			})
		} else if t.Name != "" {
			tableNames[t.Name] = i
		}

		for gi, g := range t.Groups {
			if strings.TrimSpace(g) == "" {
				errs = append(errs, &ValidationError{
					Path:  fmt.Sprintf("%s.Groups[%d]", path, gi),
					Cause: "group name is empty",
				})
			}
		}

		for pi, idx := range t.PrimaryKeys {
			if idx < 0 || idx >= len(t.Columns) {
				errs = append(errs, &ValidationError{
					Path:  fmt.Sprintf("%s.PrimaryKeys[%d]", path, pi),
					Cause: fmt.Sprintf("index %d out of range [0, %d)", idx, len(t.Columns)),
				})
			}
		}
	}

	for ti, t := range s.Tables {
		for ci, c := range t.Columns {
			if c.FK == nil {
				continue
			}
			if _, ok := tableNames[c.FK.TargetTable]; !ok {
				errs = append(errs, &ValidationError{
					Path:  fmt.Sprintf("Tables[%d].Columns[%d].FK.TargetTable", ti, ci),
					Cause: fmt.Sprintf("target table %q not found in schema", c.FK.TargetTable),
				})
			}
		}
	}

	for gi, g := range s.Groups {
		if strings.TrimSpace(g) == "" {
			errs = append(errs, &ValidationError{
				Path:  fmt.Sprintf("Groups[%d]", gi),
				Cause: "group name is empty",
			})
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// DeriveGroups は Schema から登場順を保持した primary グループ集合を返す。
// group.go の DeriveGroups(*Schema) を呼び出すラッパーで、テンプレートからは
// {{.DeriveGroups}} 記法で参照できる。
func (s *Schema) DeriveGroups() []Group {
	return DeriveGroups(s)
}
