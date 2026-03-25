'use client';

import React, { useMemo } from 'react';
import styled, { useTheme } from 'styled-components';
import type { DefaultTheme } from 'styled-components';
import type { FlamebearerProfile } from '@/types/profiling';

/** Distinct Odigos theme colors (v2 palette + brand) so bars stay visible on dark UI. */
function barBackgroundForFrame(theme: DefaultTheme, label: string, depth: number): string {
  const v2 = theme.v2.colors;
  const palettes = [
    v2.purple['500'],
    v2.green['500'],
    v2.blue['500'],
    v2.yellow['500'],
    v2.red['500'],
    theme.colors.majestic_blue,
    theme.colors.orange_og,
    theme.colors.dark_green,
    v2.purple['400'],
    v2.blue['600'],
    v2.green['600'],
  ];
  let h = depth * 131;
  for (let i = 0; i < label.length; i++) {
    h = (h + label.charCodeAt(i) * 17) >>> 0;
  }
  return palettes[h % palettes.length];
}

/** Pyroscope single format: 4 ints per bar — offset, total, self, nameIndex */
const J_STEP = 4;
const J_NAME = 3;

const Wrap = styled.div`
  width: 100%;
  min-height: 200px;
  font-size: 11px;
  line-height: 1;
  user-select: none;
  color: ${({ theme }) => theme.text.primary};
`;

const Row = styled.div`
  position: relative;
  width: 100%;
  height: 20px;
  margin-bottom: 1px;
`;

const Bar = styled.div<{ $left: number; $width: number; $bg: string }>`
  position: absolute;
  left: ${({ $left }) => $left}%;
  width: ${({ $width }) => Math.max($width, 0.05)}%;
  height: 100%;
  top: 0;
  background: ${({ $bg }) => $bg};
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: ${({ theme }) => theme.text.white};
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.75);
  padding: 2px 4px;
  box-sizing: border-box;
  cursor: default;
  &:hover {
    filter: brightness(1.1) saturate(1.05);
    z-index: 1;
  }
`;

const Legend = styled.div`
  margin-bottom: 10px;
  font-size: 0.8125rem;
  color: ${({ theme }) => theme.text.secondary};
`;

export function FlamebearerIcicle({ profile }: { profile: FlamebearerProfile }) {
  const theme = useTheme() as DefaultTheme;
  const { numTicks, names, levels } = profile.flamebearer;
  const fmt = profile.metadata?.format || 'single';

  const rows = useMemo(() => {
    if (!levels?.length || numTicks <= 0 || fmt !== 'single') {
      return [];
    }
    return levels.map((row, depth) => {
      const bars: { left: number; width: number; label: string; key: string }[] = [];
      for (let t = 0; t + J_STEP - 1 < row.length; t += J_STEP) {
        const offset = row[t];
        const total = row[t + 1];
        const nameIdx = row[t + J_NAME];
        const label = names[nameIdx] ?? `?(${nameIdx})`;
        const left = (offset / numTicks) * 100;
        const width = (total / numTicks) * 100;
        bars.push({
          left,
          width,
          label,
          key: `${depth}-${t}-${nameIdx}`,
        });
      }
      return { depth, bars };
    });
  }, [levels, names, numTicks, fmt]);

  if (fmt !== 'single') {
    return (
      <Legend>
        Flame graph preview supports format &quot;single&quot; only (got {String(fmt)}).
      </Legend>
    );
  }

  if (rows.length === 0) {
    return <Legend>No levels to render (numTicks={numTicks}).</Legend>;
  }

  return (
    <Wrap>
      <Legend>
        CPU profile · {numTicks.toLocaleString()} samples · {names.length} symbols · scroll vertically for stack depth
      </Legend>
      {rows.map(({ depth, bars }) => (
        <Row key={depth}>
          {bars.map((b) => (
            <Bar
              key={b.key}
              title={b.label}
              $left={b.left}
              $width={b.width}
              $bg={barBackgroundForFrame(theme, b.label, depth)}
            >
              {b.width > 6 ? b.label : ''}
            </Bar>
          ))}
        </Row>
      ))}
    </Wrap>
  );
}
