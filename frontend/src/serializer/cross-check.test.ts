// クライアント側 serializer と Go 側 internal/serializer.Serialize の出力一致を
// 検証するクロスチェックテスト（タスク 7.8、要件 7.10）。
//
// 共有 fixture JSON をパース → `serialize(fixture)` の結果と Go 出力をベースに
// した期待値ファイル（`testdata/expected/<name>.erdm`）をバイト単位で比較する。
//
// 期待値は Go 側 `internal/serializer/cross_check_test.go` が同じ fixture を
// `*model.Schema` にデコードして `Serialize` した結果と一致することも検証する
// （リファレンス実装の変更時には Go 側で先に失敗する）。
//
// Requirements: 7.10

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { Schema } from '../model'
import { serialize } from './serialize'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const FIXTURES_PATH = path.join(__dirname, 'testdata', 'cross-check-fixtures.json')
const EXPECTED_DIR = path.join(__dirname, 'testdata', 'expected')

interface FixtureMap {
  [name: string]: Schema
}

function loadFixtures(): FixtureMap {
  const raw = fs.readFileSync(FIXTURES_PATH, 'utf8')
  return JSON.parse(raw) as FixtureMap
}

function loadExpected(name: string): string {
  return fs.readFileSync(path.join(EXPECTED_DIR, `${name}.erdm`), 'utf8')
}

describe('cross-check vs Go reference serializer', () => {
  const fixtures = loadFixtures()
  const names = Object.keys(fixtures).sort()

  for (const name of names) {
    it(`fixture "${name}" serializes to byte-identical output`, () => {
      const fixture = fixtures[name]
      if (fixture === undefined) {
        throw new Error(`fixture ${name} missing`)
      }
      const got = serialize(fixture)
      const want = loadExpected(name)
      expect(got).toBe(want)
    })
  }
})
