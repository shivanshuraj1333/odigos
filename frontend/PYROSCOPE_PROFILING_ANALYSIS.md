# Pyroscope vs Odigos profiling: what to borrow

## Pyroscope: how it works

### Backend

1. **Ingest (OTLP)**  
   - `pkg/ingester/otlp/ingest_handler.go`: receives OTLP profiles (gRPC/HTTP).  
   - `pkg/ingester/otlp/convert.go`: **ConvertOtelToGoogle** turns OTLP `Profile` + `ProfilesDictionary` into Google pprof-style `Profile` (samples, locations, functions, string table). One OTLP profile can become multiple “converted” profiles (e.g. by `service.name`).  
   - Converted profiles are pushed to the **distributor** → **ingester** / **segment writer** and stored (blocks, not a simple in-memory buffer).

2. **Storage**  
   - Profiles are stored in **phlaredb** (blocks, compaction, retention). Not a rolling in-memory cache like Odigos; it’s a full time-series profile store.

3. **Query → flame graph**  
   - **Render** API: `GET /render?query=...&from=...&until=...&format=json`  
   - Handler (`pkg/querier/http.go` `Render`):  
     - Calls **SelectMergeStacktraces** (time range + label selector) → returns a **merged flame graph** (proto: `Names`, `Levels`, `Total`, `MaxSelf`).  
     - **SelectSeries** for timeline.  
   - **Flame graph is built on the backend**: query layer reads profiles from storage, merges stack traces, and returns a **FlameGraph** struct.  
   - **ExportToFlamebearer** (`pkg/model/flamegraph.go`): converts that FlameGraph proto into **FlamebearerProfile** (JSON for the UI).

4. **Flamebearer format (backend → frontend)**  
   - **FlamebearerProfile** (`pkg/og/structs/flamebearer/flamebearer.go`):  
     - `flamebearer.names`: `[]string` (symbol names).  
     - `flamebearer.levels`: `[][]int` — each level is a row; each node is 4 ints (single) or 7 (diff): **x offset (delta)**, **total**, **self**, **name index** (and for diff: same for right tree).  
     - `flamebearer.numTicks`, `maxSelf`, `metadata` (format, units, spyName, etc.).  
   - **Tree** (`pkg/model/tree.go`): in-memory tree of `node{ name, self, total, children }`. **InsertStack(value, stack...)** merges stacks; **NewFlameGraph(tree, maxNodes)** walks the tree and produces the Levels + Names encoding (with delta-encoded x offsets).

So in Pyroscope: **backend stores profiles, runs the query, merges stacks into a Tree, then converts Tree → FlameGraph → FlamebearerProfile. The API returns FlamebearerProfile JSON. No raw OTLP chunks are sent to the UI.**

### Frontend

1. **API**  
   - **render.ts**: `GET /pyroscope/render?...&format=json` → returns **FlamebearerProfile** (with optional timeline).

2. **Decode**  
   - **decodeFlamebearer** (`public/app/models/flamebearer.ts`): applies **deltaDiffWrapper** to `levels` (decodes delta-encoded x offsets).  
   - **flamebearerToDataFrameDTO** (`public/app/util/flamebearer.ts`): converts `names` + `levels` into a Grafana **DataFrame** (node tree with offset, val, self, children).

3. **Render**  
   - **FlameGraphWrapper.tsx**: uses **@grafana/flamegraph** `<FlameGraph data={dataFrame} />`.  
   - So the UI only **decodes** the backend format and **renders**; it does not parse OTLP or build the tree.

---

## Odigos today

- **Backend**: stores **raw OTLP/JSON chunks** per source in a BoundedBuffer; GET returns `{ chunks: ["<json>", ...] }`. No merging, no flame graph.  
- **Frontend**: fetches chunks, **parseChunksToFlameTree** (parses OTLP JSON, extracts samples + names, **builds tree**, aggregates across chunks), **FlameGraph** renders a simple `FlameNode` tree (name, value, children).

---

## What to borrow from Pyroscope

### 1. Backend: build flame graph, return Flamebearer (or equivalent)

- **Idea**: Keep your current **ingest** (OTLP → chunks or merged profile per source). On **GET**, instead of returning raw chunks:
  - Either **merge chunks into one OTLP profile** then **build a tree** (stack merge) and **serialize to a single response format** (e.g. Flamebearer-like),  
  - Or reuse Pyroscope’s **Flamebearer** shape so you can reuse their UI.

- **Concrete backend work** (high level):  
  - Ingest: keep storing per-source data (chunks or one merged OTLP profile per slot).  
  - Add a **tree builder** that consumes your stored data (e.g. from merged OTLP or from parsed chunks):  
    - Walk samples; for each sample (stack + value), **insert into a Tree** (same idea as `model.Tree` **InsertStack**).  
  - Add **Tree → Flamebearer** (or Tree → your own “flame graph” JSON):  
    - Same idea as **NewFlameGraph** + **ExportToFlamebearer**: names array + levels (x, total, self, name index), delta-encoded.  
  - GET handler returns **one JSON object** (e.g. `{ flamebearer: { names, levels, numTicks }, metadata: { ... } }`) instead of `{ chunks: [...] }`.

- You can **reuse Pyroscope’s Go code** where it fits:  
  - **pkg/model/tree.go** (Tree + InsertStack + Merge) and **pkg/model/flamegraph.go** (NewFlameGraph, delta encoding) are backend-only and format-agnostic.  
  - **pkg/og/structs/flamebearer**: defines the JSON shape; you can return the same shape so the same frontend decoder works.  
  - You’d need an **adapter** from your stored data (OTLP or chunks) into that Tree (e.g. iterate samples, resolve location/function names to strings, call InsertStack).

### 2. Frontend: use Pyroscope’s UI components

- **Flamebearer format**: If your backend returns **FlamebearerProfile** (or a subset: `names`, `levels`, `numTicks`, `metadata.format`), you can:
  - Reuse **decodeFlamebearer** + **deltaDiffWrapper** + **flamebearerToDataFrameDTO** from Pyroscope’s `public/app` (or port the logic).  
  - Then use **@grafana/flamegraph** like **FlameGraphWrapper** (or wrap it in your own component that calls your API and passes the decoded data).

- **Dependencies**: Pyroscope UI uses `@grafana/flamegraph`, `@grafana/data`, `@grafana/ui`. You’d add those and either:
  - Copy the small decode/util files (`flamebearer.ts`, `models/flamebearer.ts`) and the **FlameGraphWrapper** (or a thin wrapper that fetches `/api/sources/.../profiling` and expects Flamebearer JSON), or  
  - Depend on a shared package if you have one.

- **API contract**: Your GET would change from  
  `{ chunks: string[] }`  
  to something like  
  `{ version: 1, flamebearer: { names, levels, numTicks, maxSelf }, metadata: { format: "single", units, ... } }`  
  so it matches what **decodeFlamebearer** and **FlameGraph** expect.

---

## Summary

| Aspect              | Pyroscope                         | Odigos (current)           | Borrow for Odigos                                      |
|---------------------|-----------------------------------|----------------------------|--------------------------------------------------------|
| Storage             | phlaredb (blocks, time range)     | In-memory chunks per source| Keep your slot/cache; optionally merge to one profile  |
| Flame graph built in| Backend (Tree → FlameGraph → FB)  | Frontend (chunks → tree)   | **Move to backend**: Tree + Flamebearer on GET         |
| API response        | FlamebearerProfile JSON           | Raw OTLP chunks            | **Change GET to return FlamebearerProfile-like JSON**  |
| Frontend            | Decode FB → DataFrame → Grafana FG| Parse OTLP → custom tree   | **Use Pyroscope decode + @grafana/flamegraph**         |

So: **borrow backend logic** (tree merge + flame graph encoding) and **Flamebearer** response format; **borrow frontend** decode + **@grafana/flamegraph** so the UI only decodes and renders. Your backend continues to cache “raw” in the sense of “whatever you store per source,” but the **flame graph is built and serialized on the backend** and the **UI only consumes Flamebearer**.
