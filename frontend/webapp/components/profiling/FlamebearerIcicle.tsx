'use client';

import React, { useMemo } from 'react';
import styled from 'styled-components';
import type { FlamebearerProfile } from '@/types/profiling';

/** Pyroscope single format: 4 ints per bar — offset, total, self, nameIndex */
const J_STEP = 4;
const J_NAME = 3;

const Wrap = styled.div`
  width: 100%;
  min-height: 200px;
  font-size: 11px;
  line-height: 1;
  user-select: none;
`;

const Row = styled.div`
  position: relative;
  width: 100%;
  height: 20px;
  margin-bottom: 1px;
`;

const Bar = styled.div<{ $left: number; $width: number; $hue: number }>`
  position: absolute;
  left: ${({ $left }) => $left}%;
  width: ${({ $width }) => Math.max($width, 0.05)}%;
  height: 100%;
  top: 0;
  background: ${({ theme, $hue }) => `hsl(${$hue % 360} 48% 36%)`};
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: ${({ theme }) => theme.text.primary};
  padding: 2px 4px;
  box-sizing: border-box;
  cursor: default;
  &:hover {
    filter: brightness(1.12);
    z-index: 1;
  }
`;

const Legend = styled.div`
  margin-bottom: 10px;
  font-size: 0.8125rem;
  color: ${({ theme }) => theme.text.secondary};
`;

export function FlamebearerIcicle({ profile }: { profile: FlamebearerProfile }) {
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
        CPU profile (icicle) · {numTicks.toLocaleString()} samples · {names.length} symbols · scroll vertically for stack depth
      </Legend>
      {rows.map(({ depth, bars }) => (
        <Row key={depth}>
          {bars.map((b) => (
            <Bar
              key={b.key}
              title={b.label}
              $left={b.left}
              $width={b.width}
              $hue={b.label.split('').reduce((a, c) => a + c.charCodeAt(0), depth * 17)}
            >
              {b.width > 6 ? b.label : ''}
            </Bar>
          ))}
        </Row>
      ))}
    </Wrap>
  );
}
