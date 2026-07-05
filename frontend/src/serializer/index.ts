// 公開境界: 内部 TS モデル → `.erdm` テキストのシリアライザを再エクスポートする
// （design.md §C11、要件 7.6）。Go 側 `internal/serializer.Serialize` と
// バイト一致する出力を返す（要件 7.10）。
export * from './serialize'
