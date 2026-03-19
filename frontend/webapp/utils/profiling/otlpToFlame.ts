/**
 * Parse OTLP/JSON profile chunks from the backend into a single flame tree.
 * Handles flexible JSON shapes (camelCase, snake_case, PascalCase) from Go pprofile marshaler.
 */

export interface FlameNode {
  name: string;
  value: number;
  children: FlameNode[];
}

function ensureArray<T>(v: unknown): T[] {
  if (Array.isArray(v)) return v as T[];
  return [];
}

function ensureNumber(v: unknown): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  return 0;
}

/** Get nested value with multiple key variants (camel, snake, Pascal). */
function getKey(obj: Record<string, unknown>, ...keys: string[]): unknown {
  for (const k of keys) {
    if (Object.prototype.hasOwnProperty.call(obj, k)) return obj[k];
  }
  return undefined;
}

/** Direct path for OTLP shape: resourceProfiles -> scopeProfiles -> profiles -> samples. Returns number of samples added. */
function parseOneChunkOTLP(
  parsed: Record<string, unknown>,
  samples: { locationIds: number[]; value: number }[]
): number {
  const rps = getKey(parsed, 'resourceProfiles', 'ResourceProfiles', 'resource_profiles');
  if (!Array.isArray(rps)) return 0;
  let added = 0;
  for (const rp of rps as Record<string, unknown>[]) {
    const scopes = getKey(rp, 'scopeProfiles', 'ScopeProfiles', 'scope_profiles');
    if (!Array.isArray(scopes)) continue;
    for (const scope of scopes as Record<string, unknown>[]) {
      const profiles = getKey(scope, 'profiles', 'Profiles');
      if (!Array.isArray(profiles)) continue;
      for (const profile of profiles as Record<string, unknown>[]) {
        const sampleArr = getKey(profile, 'samples', 'Samples', 'sample', 'Sample');
        if (!Array.isArray(sampleArr)) continue;
        for (const so of sampleArr as Record<string, unknown>[]) {
          if (!so || typeof so !== 'object') continue;
          let locArray = getKey(so, 'attributeIndices', 'attribute_indices', 'locationIdList', 'location_id_list');
          if (!Array.isArray(locArray)) {
            const stackIdx = getKey(so, 'stackIndex', 'stack_index');
            locArray = typeof stackIdx === 'number' ? [stackIdx] : [];
          }
          const locIds = (Array.isArray(locArray) ? [...(locArray as number[])] : []).reverse();
          let value = 0;
          const val = getKey(so, 'value', 'Value', 'values');
          if (typeof val === 'number') value = val;
          else if (Array.isArray(val) && val.length > 0) value = (val as number[]).reduce((a, b) => a + ensureNumber(b), 0);
          if (value === 0) {
            const timestamps = getKey(so, 'timestampsUnixNano', 'timestamps_unix_nano');
            value = Array.isArray(timestamps) ? (timestamps as unknown[]).length : 1;
          }
          if (value > 0 || locIds.length > 0) {
            samples.push({ locationIds: locIds, value: value || 1 });
            added++;
          }
        }
      }
    }
  }
  return added;
}

/** Recursively find profile-like structures in JSON. */
function extractSamplesAndNames(
  obj: unknown,
  samples: { locationIds: number[]; value: number }[],
  names: Map<number, string>,
  stringTable: string[]
): void {
  if (obj == null) return;
  if (typeof obj !== 'object') return;
  const o = obj as Record<string, unknown>;

  // Some APIs wrap payload in a "data" object
  const data = getKey(o, 'data', 'Data');
  if (data && typeof data === 'object') {
    extractSamplesAndNames(data, samples, names, stringTable);
    return;
  }

  // Process "samples" array as soon as we see it (profile object has "samples", not "profiles")
  const sampleArrEarly = getKey(o, 'samples', 'Samples', 'sample', 'Sample');
  if (Array.isArray(sampleArrEarly)) {
    for (const s of sampleArrEarly) {
      const so = s && typeof s === 'object' ? (s as Record<string, unknown>) : null;
      if (!so) continue;
      const locRaw = getKey(so, 'locationId', 'LocationId', 'location_id', 'locationIdList', 'location_id_list');
      let locArray: number[] = Array.isArray(locRaw) ? (locRaw as number[]) : [];
      if (locArray.length === 0) {
        const attrIndices = getKey(so, 'attributeIndices', 'attribute_indices');
        locArray = Array.isArray(attrIndices) ? [...(attrIndices as number[])].reverse() : [];
        if (locArray.length === 0) {
          const stackIdx = getKey(so, 'stackIndex', 'stack_index');
          if (typeof stackIdx === 'number') locArray = [stackIdx];
        }
      }
      const locIds = locArray;
      const val = getKey(so, 'value', 'Value', 'values');
      let value =
        typeof val === 'number'
          ? val
          : Array.isArray(val) && (val as number[]).length > 0
            ? (val as number[]).reduce((a, b) => a + ensureNumber(b), 0)
            : 0;
      if (value === 0) {
        const timestamps = getKey(so, 'timestampsUnixNano', 'timestamps_unix_nano');
        value = Array.isArray(timestamps) ? (timestamps as unknown[]).length : 1;
      }
      if (value > 0 || locIds.length > 0) samples.push({ locationIds: locIds, value: value || 1 });
    }
    return; // already processed samples for this object
  }

  // ResourceProfiles / resourceProfiles / resource_profiles
  const rps = getKey(o, 'resourceProfiles', 'ResourceProfiles', 'resource_profiles');
  if (Array.isArray(rps)) {
    rps.forEach((rp) => extractSamplesAndNames(rp, samples, names, stringTable));
    return;
  }

  // ScopeProfiles / scopeProfiles / scope_profiles
  const scopes = getKey(o, 'scopeProfiles', 'ScopeProfiles', 'scope_profiles');
  if (Array.isArray(scopes)) {
    scopes.forEach((s) => extractSamplesAndNames(s, samples, names, stringTable));
    return;
  }

  // Profile array or single profile (Profiles = PascalCase from some serializers)
  const profiles = getKey(o, 'profiles', 'Profiles');
  if (Array.isArray(profiles)) {
    profiles.forEach((p) => extractSamplesAndNames(p, samples, names, stringTable));
    return;
  }

  // String table: stringTable / StringTable / string_table (array of strings)
  const st = getKey(o, 'stringTable', 'StringTable', 'string_table');
  if (Array.isArray(st)) {
    st.forEach((s, i) => {
      if (typeof s === 'string') stringTable[i] = s;
    });
  }
  // OTLP: dictionary may contain stringTable at top level
  const dict = getKey(o, 'dictionary', 'Dictionary');
  if (dict && typeof dict === 'object') {
    extractSamplesAndNames(dict, samples, names, stringTable);
  }

  // Sample: sample / Sample / samples / Samples (array of samples)
  const sampleArr = getKey(o, 'sample', 'Sample', 'samples', 'Samples');
  if (Array.isArray(sampleArr)) {
    for (const s of sampleArr) {
      const so = s && typeof s === 'object' ? (s as Record<string, unknown>) : null;
      if (!so) continue;
      // OTLP format: locationIdList (legacy) or attributeIndices (stack as indices; root-first for flame)
      const locRaw2 = getKey(so, 'locationId', 'LocationId', 'location_id', 'locationIdList', 'location_id_list');
      let locArray2: number[] = Array.isArray(locRaw2) ? (locRaw2 as number[]) : [];
      if (locArray2.length === 0) {
        const attrIndices = getKey(so, 'attributeIndices', 'attribute_indices');
        locArray2 = Array.isArray(attrIndices) ? (attrIndices as number[]) : [];
        if (locArray2.length > 0) locArray2 = [...locArray2].reverse();
        if (locArray2.length === 0) {
          const stackIdx = getKey(so, 'stackIndex', 'stack_index');
          if (typeof stackIdx === 'number') locArray2 = [stackIdx];
        }
      }
      const locIds = locArray2;
      const val = getKey(so, 'value', 'Value', 'values');
      let value =
        typeof val === 'number'
          ? val
          : Array.isArray(val) && (val as number[]).length > 0
            ? (val as number[]).reduce((a, b) => a + ensureNumber(b), 0)
            : 0;
      if (value === 0) {
        const timestamps = getKey(so, 'timestampsUnixNano', 'timestamps_unix_nano');
        value = Array.isArray(timestamps) ? (timestamps as unknown[]).length : 1;
      }
      if (value > 0 && locIds.length >= 0) samples.push({ locationIds: locIds, value });
    }
  }

  // Attribute table / mapping id -> string index (for names)
  const attrTable = getKey(o, 'attributeTable', 'AttributeTable', 'attribute_table');
  if (attrTable && typeof attrTable === 'object') {
    const at = attrTable as Record<string, unknown>;
    const keys = getKey(at, 'keys', 'Keys');
    if (Array.isArray(keys)) {
      (keys as unknown[]).forEach((k, i) => {
        const idx = typeof k === 'number' ? k : Array.isArray(k) ? (k[0] as number) : i;
        if (stringTable[idx]) names.set(i, stringTable[idx]);
      });
    }
  }

  // Location: location / Location (array) with line/function reference
  const locs = getKey(o, 'location', 'Location', 'locations');
  if (Array.isArray(locs)) {
    locs.forEach((loc, locIndex) => {
      const lo = loc && typeof loc === 'object' ? (loc as Record<string, unknown>) : null;
      if (!lo) return;
      const nameRef = getKey(lo, 'name', 'Name', 'functionName', 'function_name', 'line', 'Line');
      let label = `loc_${locIndex}`;
      const nameIdx = typeof nameRef === 'number' ? nameRef : Array.isArray(nameRef) ? (nameRef[0] as number) : -1;
      if (nameIdx >= 0 && stringTable[nameIdx]) label = stringTable[nameIdx];
      else {
        const lineArr = getKey(lo, 'line', 'Line');
        if (Array.isArray(lineArr) && lineArr.length > 0) {
          const firstLine = (lineArr[0] as Record<string, unknown>);
          const funcIdx = getKey(firstLine, 'functionIndex', 'FunctionIndex', 'function_index');
          const idx = typeof funcIdx === 'number' ? funcIdx : Array.isArray(funcIdx) ? (funcIdx[0] as number) : -1;
          if (idx >= 0 && stringTable[idx]) label = stringTable[idx];
        }
      }
      names.set(locIndex, label);
    });
  }

  // Function: function / Function (array) for name table
  const funcs = getKey(o, 'function', 'Function', 'functions');
  if (Array.isArray(funcs)) {
    funcs.forEach((fn, i) => {
      const fo = fn && typeof fn === 'object' ? (fn as Record<string, unknown>) : null;
      if (!fo) return;
      const nameRef = getKey(fo, 'name', 'Name');
      const nameIdx = typeof nameRef === 'number' ? nameRef : Array.isArray(nameRef) ? (nameRef[0] as number) : -1;
      if (nameIdx >= 0 && stringTable[nameIdx]) names.set(i, stringTable[nameIdx]);
    });
  }

  // Recurse into common nested keys (scope profile object has scope + profiles)
  const nested = getKey(o, 'resource', 'scope', 'profile', 'Profile');
  if (nested && typeof nested === 'object') extractSamplesAndNames(nested, samples, names, stringTable);
}

/** Build a single flame tree from parsed samples and name map. */
function buildTree(
  samples: { locationIds: number[]; value: number }[],
  names: Map<number, string>
): FlameNode | null {
  const root: FlameNode = { name: 'root', value: 0, children: [] };
  const keyToNode = new Map<string, FlameNode>();
  keyToNode.set('', root);

  for (const { locationIds, value } of samples) {
    const stack = locationIds.map((id) => names.get(id) ?? `frame_${id}`).filter(Boolean);
    if (stack.length === 0) {
      root.value += value;
      continue;
    }
    root.value += value;
    let path = '';
    let parent = root;
    for (let i = 0; i < stack.length; i++) {
      const label = stack[i];
      path += (path ? '|' : '') + label;
      let node = keyToNode.get(path);
      if (!node) {
        node = { name: label, value: 0, children: [] };
        parent.children.push(node);
        keyToNode.set(path, node);
      }
      node.value += value;
      parent = node;
    }
  }

  if (root.value === 0 && root.children.length === 0) return null;
  return root;
}

/** Parse all chunks and merge into one flame tree. Chunks can be JSON strings or pre-parsed objects. */
export function parseChunksToFlameTree(chunks: (string | object)[] | { chunks?: (string | object)[] }): FlameNode | null {
  const samples: { locationIds: number[]; value: number }[] = [];
  const names = new Map<number, string>();
  const stringTable: string[] = [];

  // Normalize: accept raw API response { chunks: [...] } or the array directly
  const list: (string | object)[] = Array.isArray(chunks)
    ? chunks
    : chunks && typeof chunks === 'object' && Array.isArray((chunks as { chunks?: unknown }).chunks)
      ? (chunks as { chunks: (string | object)[] }).chunks
      : [];

  if (typeof process !== 'undefined' && process.env?.NODE_ENV === 'development' && list.length > 0) {
    const first = list[0];
    const preview = typeof first === 'string' ? first.slice(0, 80) + (first.length > 80 ? '...' : '') : JSON.stringify(first).slice(0, 80) + '...';
    console.log('[parseChunksToFlameTree] chunks=', list.length, 'first=', typeof first, preview);
  }

  for (let i = 0; i < list.length; i++) {
    const chunk = list[i];
    try {
      let parsed: unknown =
        typeof chunk === 'string' ? (JSON.parse(chunk) as unknown) : (chunk as unknown);
      // Backend may double-encode: chunk is a string that parses to another JSON string
      if (typeof parsed === 'string') {
        try {
          parsed = JSON.parse(parsed) as unknown;
        } catch {
          /* leave as string */
        }
      }
      if (parsed && typeof parsed === 'object') {
        const direct = parseOneChunkOTLP(parsed as Record<string, unknown>, samples);
        if (direct === 0) extractSamplesAndNames(parsed, samples, names, stringTable);
      }
    } catch (e) {
      if (typeof process !== 'undefined' && process.env?.NODE_ENV === 'development') {
        console.warn('[parseChunksToFlameTree] chunk', i, 'parse failed:', e);
      }
    }
  }

  if (samples.length === 0) {
    if (typeof process !== 'undefined' && process.env?.NODE_ENV === 'development' && list.length > 0) {
      console.warn('[parseChunksToFlameTree] no samples extracted from', list.length, 'chunks');
    }
    return null;
  }
  return buildTree(samples, names);
}
