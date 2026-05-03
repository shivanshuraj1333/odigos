/**
 * Converts Pyroscope / Grafana flamebearer delta-encoded x offsets in each `levels` row to absolute
 * offsets — mirrors `deltaDecoding` in github.com/grafana/pyroscope/pkg/model/flamegraph.go. The
 * Odigos backend passes Pyroscope's wire shape through unchanged, so the decoding happens here
 * just before the flame canvas consumes `levels`.
 */
function decodeFlamebearerLevelDeltas(levels: number[][]): void {
  for (const row of levels) {
    let prev = 0;
    for (let i = 0; i + 1 < row.length; i += 4) {
      const delta = row[i] + row[i + 1];
      row[i] += prev;
      prev += delta;
    }
  }
}

type FlamebearerWire = { levels?: number[][] };

type ProfileJsonWire = { flamebearer?: FlamebearerWire };

/**
 * Parses Odigos profiling `profileJson`, mutates `flamebearer.levels` in place to absolute x offsets,
 * and re-serializes. On parse errors or missing levels, returns the input unchanged.
 */
export function decodeFlamebearerProfileJson(profileJson: string): string {
  if (!profileJson) {
    return profileJson;
  }
  try {
    const parsed = JSON.parse(profileJson) as ProfileJsonWire;
    const levels = parsed?.flamebearer?.levels;
    if (levels && Array.isArray(levels)) {
      decodeFlamebearerLevelDeltas(levels);
    }
    return JSON.stringify(parsed);
  } catch {
    return profileJson;
  }
}
