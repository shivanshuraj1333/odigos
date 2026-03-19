/**
 * Decode Pyroscope-style Flamebearer (names + levels) into FlameNode tree for our FlameGraph.
 * Levels are delta-encoded: per node [xOffset, total, self, nameIndex]; we decode xOffset then build tree.
 */

import type { FlameNode } from './otlpToFlame';

export interface SymbolRow {
  name: string;
  self: number;
  total: number;
}

/** Pyroscope-compatible API response (backend matches this shape). */
export interface FlamebearerResponse {
  version?: number;
  flamebearer?: {
    names?: string[];
    levels?: number[][];
    numTicks?: number;
    maxSelf?: number;
  };
  metadata?: { format?: string; spyName?: string; sampleRate?: number; units?: string; name?: string; symbolsHint?: string };
  timeline?: { startTime: number; samples: number[]; durationDelta: number; watermarks?: number[] } | null;
  groups?: unknown;
  heatmap?: unknown;
  symbols?: SymbolRow[];
}

/** Delta-decode x offsets in levels (single format: 4 ints per node). */
function deltaDecodeLevels(levels: number[][]): void {
  for (const level of levels) {
    let prev = 0;
    for (let i = 0; i < level.length; i += 4) {
      level[i] += prev;
      prev = level[i] + level[i + 1];
    }
  }
}

/** Convert Flamebearer API response to FlameNode (for existing FlameGraph component). */
export function flamebearerToFlameNode(response: FlamebearerResponse | null): FlameNode | null {
  if (!response?.flamebearer?.names?.length || !response?.flamebearer?.levels?.length) {
    return null;
  }
  const names = response.flamebearer.names;
  const levels = response.flamebearer.levels.map((row) => [...row]);
  deltaDecodeLevels(levels);

  type BuildNode = { name: string; total: number; self: number; offset: number; children: BuildNode[] };
  const buildNodes: BuildNode[][] = [];
  for (let lev = 0; lev < levels.length; lev++) {
    const row = levels[lev];
    const nodes: BuildNode[] = [];
    for (let i = 0; i < row.length; i += 4) {
      const offset = row[i];
      const total = row[i + 1];
      const self = row[i + 2];
      const nameIdx = row[i + 3];
      const name = names[nameIdx] ?? `frame_${nameIdx}`;
      nodes.push({ name, total, self, offset, children: [] });
    }
    buildNodes.push(nodes);
  }
  if (buildNodes.length === 0) return null;

  function findParent(nodes: BuildNode[], offset: number): BuildNode | null {
    for (const p of nodes) {
      if (offset >= p.offset && offset < p.offset + p.total) return p;
    }
    return null;
  }
  for (let lev = 1; lev < buildNodes.length; lev++) {
    const parents = buildNodes[lev - 1];
    const children = buildNodes[lev];
    for (const c of children) {
      const p = findParent(parents, c.offset);
      if (p) p.children.push(c);
    }
  }

  function toFlameNode(n: BuildNode): FlameNode {
    return { name: n.name, value: n.total, children: n.children.map(toFlameNode) };
  }
  const root: FlameNode = { name: 'root', value: 0, children: [] };
  const level0 = buildNodes[0] ?? [];
  for (const n of level0) {
    root.value += n.total;
    root.children.push(...n.children.map(toFlameNode));
  }
  if (root.value === 0 && root.children.length === 0) return null;
  return root;
}
