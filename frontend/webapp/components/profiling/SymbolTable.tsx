'use client';

import React from 'react';
import type { SymbolRow } from '@/utils/profiling/flamebearerToFlame';

const SYMBOL_TABLE_STYLES = {
  container: {
    width: '100%',
    overflow: 'auto' as const,
    maxHeight: 420,
    background: '#1e1e2e',
    borderRadius: 8,
    border: '1px solid #313244',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse' as const,
    fontSize: 13,
  },
  th: {
    position: 'sticky' as const,
    top: 0,
    background: '#181825',
    color: '#a6adc8',
    fontWeight: 600,
    textAlign: 'left' as const,
    padding: '10px 12px',
    borderBottom: '1px solid #313244',
  },
  td: {
    padding: '8px 12px',
    borderBottom: '1px solid #313244',
    color: '#cdd6f4',
  },
  tdRight: {
    textAlign: 'right' as const,
    fontVariantNumeric: 'tabular-nums' as const,
    color: '#a6adc8',
  },
  rowHover: {
    background: '#313244',
  },
};

function formatSamples(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}k`;
  return String(n);
}

export interface SymbolTableProps {
  symbols: SymbolRow[];
  totalSamples: number;
  className?: string;
}

export function SymbolTable({ symbols, totalSamples, className }: SymbolTableProps) {
  if (!symbols?.length) {
    return (
      <div
        className={className}
        style={{
          ...SYMBOL_TABLE_STYLES.container,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          minHeight: 120,
          color: '#a6adc8',
        }}
      >
        No symbol data. Ensure the profile contains resolved function names.
      </div>
    );
  }

  return (
    <div className={className} style={SYMBOL_TABLE_STYLES.container}>
      <table style={SYMBOL_TABLE_STYLES.table}>
        <thead>
          <tr>
            <th style={SYMBOL_TABLE_STYLES.th}>Symbol</th>
            <th style={{ ...SYMBOL_TABLE_STYLES.th, textAlign: 'right' }}>Self ↓</th>
            <th style={{ ...SYMBOL_TABLE_STYLES.th, textAlign: 'right' }}>Total</th>
          </tr>
        </thead>
        <tbody>
          {symbols.map((row, i) => (
            <tr key={`${row.name}-${i}`} style={{ background: i % 2 === 1 ? 'rgba(0,0,0,0.15)' : undefined }}>
              <td style={SYMBOL_TABLE_STYLES.td} title={row.name}>
                <span style={{ fontFamily: 'ui-monospace, monospace', fontSize: 12 }}>{row.name}</span>
              </td>
              <td style={{ ...SYMBOL_TABLE_STYLES.td, ...SYMBOL_TABLE_STYLES.tdRight }}>
                {formatSamples(row.self)} {totalSamples > 0 ? `(${((100 * row.self) / totalSamples).toFixed(1)}%)` : ''}
              </td>
              <td style={{ ...SYMBOL_TABLE_STYLES.td, ...SYMBOL_TABLE_STYLES.tdRight }}>
                {formatSamples(row.total)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
