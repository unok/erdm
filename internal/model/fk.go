// Package model はスキーマ・テーブル・カラム・FK・インデックス・グループを表す
// 純粋ドメイン構造体と不変条件・派生計算を提供する。
//
// このパッケージは I/O・パース・レンダリングへ依存しない。標準ライブラリのみを
// 利用し、他レイヤー（parser/serializer/dot/elk/html/ddl/server）から参照される
// 共通ドメイン基盤として位置づけられる。
package model

// FK はカラムが保持する外部キー関係を表す値オブジェクト。
//
// TargetTable は同 Schema 内の Table.Name と一致する必要がある（Schema.Validate
// で検証）。CardinalitySource/CardinalityDestination は PEG 文法で受理される
// "0".."1".."*" の組（例 "1", "*", "1..*"）をそのまま保持する。
type FK struct {
	TargetTable            string
	CardinalitySource      string
	CardinalityDestination string
}
