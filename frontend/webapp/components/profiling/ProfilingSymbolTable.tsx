'use client';

import React, { useMemo, useState } from 'react';
import styled from 'styled-components';
import type { SymbolStatsRow } from './flamebearerSymbolStats';
import { formatSampleCount } from './flamebearerSymbolStats';

type SortKey = 'symbol' | 'self' | 'total';

const TableWrap = styled.div`
  width: 100%;
  max-height: min(50vh, 420px);
  overflow: auto;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 12px;
  background: ${({ theme }) => theme.colors.dropdown_bg_2 || theme.colors.dark_grey};
`;

const Table = styled.table`
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
`;

const Th = styled.th<{ $active?: boolean }>`
  text-align: left;
  padding: 8px 10px;
  position: sticky;
  top: 0;
  background: ${({ theme }) => theme.colors.translucent_bg || theme.colors.dropdown_bg};
  border-bottom: 1px solid ${({ theme }) => theme.colors.border};
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
  color: ${({ theme }) => theme.text.secondary};
  font-weight: ${({ $active }) => ($active ? 600 : 500)};
  &:hover {
    color: ${({ theme }) => theme.text.primary};
  }
`;

const Td = styled.td`
  padding: 6px 10px;
  border-bottom: 1px solid ${({ theme }) => theme.colors.border + '66'};
  max-width: 0;
  color: ${({ theme }) => theme.text.primary};
`;

const SymbolCell = styled.span`
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ${({ theme }) => theme.font_family?.secondary || 'monospace'};
  font-size: 11px;
  color: ${({ theme }) => theme.text.primary};
`;

const Tr = styled.tr`
  cursor: pointer;
  &:hover {
    background: ${({ theme }) => theme.colors.border + '33'};
  }
`;

export function ProfilingSymbolTable({ rows, search }: { rows: SymbolStatsRow[]; search: string }) {
  const [sortKey, setSortKey] = useState<SortKey>('self');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc');

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) => r.symbol.toLowerCase().includes(q));
  }, [rows, search]);

  const sorted = useMemo(() => {
    const out = [...filtered];
    const dir = sortDir === 'asc' ? 1 : -1;
    out.sort((a, b) => {
      let cmp = 0;
      if (sortKey === 'symbol') cmp = a.symbol.localeCompare(b.symbol);
      else if (sortKey === 'self') cmp = a.self - b.self;
      else cmp = a.total - b.total;
      return cmp * dir;
    });
    return out;
  }, [filtered, sortKey, sortDir]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    else {
      setSortKey(key);
      setSortDir(key === 'symbol' ? 'asc' : 'desc');
    }
  };

  const arrow = (key: SortKey) => (sortKey === key ? (sortDir === 'desc' ? ' ↓' : ' ↑') : '');

  const copySymbol = async (symbol: string) => {
    try {
      await navigator.clipboard.writeText(symbol);
    } catch {
      /* ignore */
    }
  };

  if (!rows.length) return null;

  return (
    <TableWrap data-profiler-symbol-table>
      <Table>
        <thead>
          <tr>
            <Th $active={sortKey === 'symbol'} onClick={() => toggleSort('symbol')}>
              Symbol{arrow('symbol')}
            </Th>
            <Th $active={sortKey === 'self'} onClick={() => toggleSort('self')}>
              Self{arrow('self')}
            </Th>
            <Th $active={sortKey === 'total'} onClick={() => toggleSort('total')}>
              Total{arrow('total')}
            </Th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((r) => (
            <Tr key={r.nameIndex} onClick={() => void copySymbol(r.symbol)} title="Click to copy symbol">
              <Td>
                <SymbolCell>{r.symbol}</SymbolCell>
              </Td>
              <Td>{formatSampleCount(r.self)}</Td>
              <Td>{formatSampleCount(r.total)}</Td>
            </Tr>
          ))}
        </tbody>
      </Table>
    </TableWrap>
  );
}
