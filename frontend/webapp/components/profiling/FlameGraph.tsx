'use client';

import React, { useMemo, useState } from 'react';
import type { FlameNode } from '@/utils/profiling';

const ROW_HEIGHT = 20;
const FONT_SIZE = 12;

interface LayoutNode {
  node: FlameNode;
  x: number; // 0–100 percent of total width
  w: number; // width in percent
  depth: number;
}

function flattenLayout(
  node: FlameNode,
  parentX: number,
  parentW: number,
  depth: number,
  out: LayoutNode[]
): void {
  if (parentW < 0.1) return;
  out.push({ node, x: parentX, w: parentW, depth });
  let offset = 0;
  const siblingSum = node.children.reduce((s, c) => s + c.value, 0) || 1;
  for (const child of node.children) {
    const cw = parentW * (child.value / siblingSum);
    if (cw >= 0.1) {
      flattenLayout(child, parentX + offset, cw, depth + 1, out);
      offset += cw;
    }
  }
}

/** Stable color per symbol name (Pyroscope-style). */
function barColorForName(name: string, dark: boolean, isHovered: boolean): string {
  if (isHovered) return dark ? '#89b4fa' : 'var(--color-primary-hover, #3b82f6)';
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  const palette = dark
    ? ['#89b4fa', '#cba6f7', '#a6e3a1', '#f9e2af', '#fab387', '#f38ba8', '#94e2d5', '#b4befe', '#89dceb', '#eba0ac']
    : ['#3b82f6', '#8b5cf6', '#22c55e', '#eab308', '#f97316', '#ef4444', '#14b8a6', '#6366f1'];
  return palette[h % palette.length];
}

function formatSamples(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}k`;
  return String(n);
}

interface FlameGraphProps {
  data: FlameNode | null;
  width?: number;
  height?: number;
  className?: string;
  /** Pyroscope-like dark theme (dark bg, light text, vibrant bars) */
  dark?: boolean;
}

const DARK_THEME = {
  bg: '#1e1e2e',
  text: '#cdd6f4',
  textMuted: '#a6adc8',
  border: '#313244',
};

export function FlameGraph({ data, width = 800, height = 400, className, dark = true }: FlameGraphProps) {
  const [hovered, setHovered] = useState<LayoutNode | null>(null);

  const { layout, total } = useMemo(() => {
    if (!data || data.value === 0) return { layout: [] as LayoutNode[], total: 0 };
    const layout: LayoutNode[] = [];
    flattenLayout(data, 0, 100, 0, layout);
    return { layout, total: data.value };
  }, [data]);

  const maxDepth = layout.length > 0 ? Math.max(...layout.map((l) => l.depth)) : 0;
  const svgHeight = Math.min(height, (maxDepth + 1) * ROW_HEIGHT);

  const bg = dark ? DARK_THEME.bg : 'var(--color-bg-secondary, #f5f5f5)';
  const textColor = dark ? DARK_THEME.text : 'var(--color-text-secondary, #666)';
  const tooltipBg = dark ? '#313244' : 'var(--color-bg-secondary, #eee)';

  if (!data || total === 0) {
    return (
      <div
        className={className}
        style={{
          width,
          height: svgHeight || 120,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: bg,
          borderRadius: 8,
          border: dark ? `1px solid ${DARK_THEME.border}` : undefined,
        }}
      >
        <span style={{ color: textColor }}>No profile data to display</span>
      </div>
    );
  }

  return (
    <div className={className} style={{ width }}>
      {hovered && (
        <div
          style={{
            marginBottom: 8,
            padding: '6px 10px',
            background: tooltipBg,
            borderRadius: 6,
            fontSize: 12,
            color: dark ? DARK_THEME.text : undefined,
          }}
        >
          <strong>{hovered.node.name}</strong> — {hovered.node.value.toLocaleString()} samples
          {total > 0 && (
            <span> ({(100 * (hovered.node.value / total)).toFixed(1)}%)</span>
          )}
        </div>
      )}
      <svg
        width={width}
        height={svgHeight}
        style={{ display: 'block', overflow: 'visible', background: dark ? bg : undefined, borderRadius: 8 }}
        onMouseLeave={() => setHovered(null)}
      >
        {layout.map((item, i) => {
          const x = (item.x / 100) * width;
          const w = Math.max((item.w / 100) * width, 2);
          const y = item.depth * ROW_HEIGHT;
          const samplesStr = formatSamples(item.node.value);
          const shortName = item.node.name.length > 18 ? item.node.name.slice(0, 15) + '…' : item.node.name;
          const label = w > 70 ? `${shortName} (${samplesStr})` : w > 40 ? shortName : w > 20 ? '…' : '';
          const barColor = barColorForName(item.node.name, dark ?? true, hovered?.node === item.node);
          return (
            <g
              key={i}
              onMouseEnter={() => setHovered(item)}
              onMouseMove={() => setHovered(item)}
            >
              <rect
                x={x}
                y={y}
                width={w}
                height={ROW_HEIGHT - 1}
                fill={barColor}
                stroke={dark ? 'rgba(0,0,0,0.15)' : 'rgba(0,0,0,0.08)'}
                strokeWidth={0.5}
                rx={1}
              />
              {w > 20 && label && (
                <text
                  x={x + 4}
                  y={y + ROW_HEIGHT / 2 + FONT_SIZE / 3}
                  fontSize={FONT_SIZE}
                  fill={dark ? '#1e1e2e' : 'rgba(0,0,0,0.85)'}
                  style={{ pointerEvents: 'none', fontWeight: 500 }}
                >
                  {label}
                </text>
              )}
            </g>
          );
        })}
      </svg>
    </div>
  );
}
