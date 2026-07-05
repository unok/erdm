// クライアント側 `buildElkInput` と Go 側 `internal/elk` の ELK 入力グラフ
// 「構造」の一致を検証するクロスチェックテスト（ADR-0001）。
//
// 共有 fixture（`testdata/elk-cross-check-fixtures.json`）から ELK 入力を構築し、
// 「一致すべき構造」= primary グループ所属 + エッジ集合 を抽出して、Go 側が生成した
// 期待値（`testdata/expected/<name>.elk-structure.json`）と比較する。
// 期待値は Go 側 `internal/elk/cross_check_test.go` が真実の単一の源として生成する。
// 期待値が古くなった場合は Go 側テストが先に失敗する。

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { ElkNode } from 'elkjs/lib/elk.bundled.js'
import { describe, expect, it } from 'vitest'
import type { Schema } from '../model'
import { buildElkInput } from './elk'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const FIXTURES_PATH = path.join(__dirname, 'testdata', 'elk-cross-check-fixtures.json')
const EXPECTED_DIR = path.join(__dirname, 'testdata', 'expected')

interface StructureSummary {
  groups: { id: string; tables: string[] }[]
  ungrouped: string[]
  edges: { source: string; target: string }[]
}

// summarize は ELK 入力ルートから「一致すべき構造」を抽出する。
// Go 側 `summarizeRoot` と同じ規則: children を持つノード=groupNode、
// それ以外=ungrouped テーブル。エッジは親→子（sources[0]→targets[0]）。
function summarize(root: ElkNode): StructureSummary {
  const groups: { id: string; tables: string[] }[] = []
  const ungrouped: string[] = []
  for (const child of root.children ?? []) {
    if (child.children && child.children.length > 0) {
      groups.push({ id: child.id, tables: child.children.map((m) => m.id) })
    } else {
      ungrouped.push(child.id)
    }
  }
  const edges = (root.edges ?? []).map((e) => ({
    source: e.sources[0]!,
    target: e.targets[0]!,
  }))
  return { groups, ungrouped, edges }
}

function loadFixtures(): Record<string, Schema> {
  return JSON.parse(fs.readFileSync(FIXTURES_PATH, 'utf8')) as Record<string, Schema>
}

function loadExpected(name: string): StructureSummary {
  const raw = fs.readFileSync(path.join(EXPECTED_DIR, `${name}.elk-structure.json`), 'utf8')
  return JSON.parse(raw) as StructureSummary
}

describe('ELK structure cross-check vs Go reference', () => {
  const fixtures = loadFixtures()
  const names = Object.keys(fixtures).sort()

  it('covers every fixture with an expected golden', () => {
    expect(names.length).toBeGreaterThan(0)
  })

  for (const name of names) {
    it(`matches Go structure for ${name}`, () => {
      const got = summarize(buildElkInput(fixtures[name]!))
      expect(got).toEqual(loadExpected(name))
    })
  }
})
