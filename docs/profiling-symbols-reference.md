# Profiling symbols reference

This document explains **what “symbols” are** in continuous profiling (CPU-style stacks), **what information they carry**, and **how frame names tend to look per language/runtime**. It is written for anyone interpreting flame graphs or symbol tables (for example Pyroscope-shaped `flamegraph.names[]` in JSON).

**Scope note:** Odigos can surface profiles built from OTLP / pprof-style data (see `frontend/services/profiles/flamegraph`). **Symbol strings** ultimately come from the **profiler + symbolization pipeline** (for example [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler)), not from Odigos alone. Examples below mix **real patterns** (from a Java + gRPC + Netty workload) with **representative** patterns for other languages—use them as a field guide, not as an exhaustive dump of every possible frame.

---

## 1. What you actually “get”

Profiling is **sampling**: the system periodically captures **each thread’s call stack**. A **symbol** (in the UI sense) is the **human-readable label for one frame** on that stack.

| You get | You do **not** get |
|--------|---------------------|
| Names for **many** frames in **your** code, **dependencies**, **runtime/JIT**, **libc**, and often the **kernel**—when those frames are **on the stack at sample time** | **Every** function that ever ran; very short calls may **never** be sampled |
| **Aggregated** weight per frame (self / total time or sample count, depending on profile type) | A complete instruction-level trace |

In **full** OpenTelemetry Profiles / pprof payloads, a frame can also carry **file path**, **line number**, **mapping**, etc. In **Pyroscope-style flame JSON**, the `names` array is often **just the display string** per frame index; extra fields may live in parallel structures or in the upstream profile, not in `names` alone.

---

## 2. What is encoded in a “symbol” string?

| Piece of information | Often in the string? | Notes |
|----------------------|----------------------|--------|
| Function / method identity | **Yes** | Main payload of `names[i]`. |
| Class / namespace / package | **Sometimes** | Java: FQCN in string. Go: `package.Function`. C++: may be mangled or demangled. |
| Return type & parameter types | **Java (typical)** | JVMS-style demangling prefixes return type (e.g. `void …`, `int …`). |
| Source file & line | **Not always in the same string** | May be separate in OTLP/pprof; Pyroscope `names` may be function-only. |
| Kernel vs user | **Implied** | Kernel frames look like short C identifiers (`tcp_sendmsg`); Java frames look like `void pkg.Class.method(…)`. |

---

## 3. Categories of symbols (all languages / runtimes)

| Category | Typical appearance | Meaning |
|----------|-------------------|--------|
| **Application** | Distinctive package prefix for *your* repo | Your business logic. |
| **Third-party library** | Known library prefixes (`io.netty`, `com.fasterxml.jackson`, `requests`, `rails`, etc.) | Dependencies on the hot path—not “noise”; often where optimization matters. |
| **Language runtime / VM** | `java.lang.*`, `jdk.internal.*`, `runtime.*`, `gc`, etc. | Runtime services: allocation, reflection, threading, I/O bridges. |
| **JIT / compiler synthetic** | `C2 Runtime …`, `StubRoutines …`, `LambdaForm$MH+…` (JVM); similar stubs exist in other VMs | Generated or helper code; not your `.java`/`.rb` source line, but real CPU. |
| **Native / JNI** | `jni_*`, `Unsafe.*`, `sun.nio.ch.*` | Boundary between managed code and native code. |
| **User-space native** | `malloc`, `memcpy`, or mangled C++ (`_ZN…`) | C/C++/Rust/Zig code, or runtime written in native code. |
| **Kernel** | `tcp_sendmsg`, `schedule`, `do_syscall_64`, `__netif_receive_skb_core` | Thread was executing in **kernel mode** (syscall, networking stack, scheduler, …). |
| **Unknown / synthetic** | `[unknown]`, `frame_123`, `<unknown>` | Unmapped PC, missing debug info, or interpreter fallback (depends on pipeline). |

---

## 4. Language-by-language guide

The eBPF-based host agent symbolizes **interpreted/JIT languages on the host** where possible; **stripped native** code may be resolved later via **file ID + offset** and a symbol store. Frame **shape** still follows the table below.

### Java (HotSpot JVM)

| Pattern | Example (illustrative) | Meaning |
|---------|------------------------|--------|
| Full JVMS-style signature | `void com.example.Service.handle(com.example.Request)` | **Return type** + **FQCN** + **method** + **parameter types** in JVM notation (`/` → `.` in class names). |
| Primitives in signature | `boolean java.util.HashMap.containsKey(java.lang.Object)` | Same; `boolean`/`int`/… are return or param types. |
| Inner classes | `void com.example.Outer$Inner.run()` | **`$`** = nested class. |
| JIT / VM | `C2 Runtime new_instance`, `StubRoutines (initialstubs)` | HotSpot **compiler or stub** code. |
| Method handles / lambdas | `java.lang.invoke.LambdaForm$MH+0x….invoke(…)` | **Invokedynamic** / **MH** machinery; address suffix is build-specific. |

**Takeaway:** A leading **`void`** is the **Java return type**, not “no symbol.”

### Go

| Pattern | Example | Meaning |
|---------|---------|--------|
| Pclntab symbol | `main.handler`, `net/http.(*Server).Serve` | From Go’s **`gopclntab`**-style symbolization: **package** + **function**; methods show **receiver type** in parentheses. |
| Runtime | `runtime.schedule`, `runtime.mcall` | Scheduler / runtime internals. |

**Takeaway:** Go frame strings **usually do not** embed return types or Java-style signatures.

### Python

| Pattern | Example | Meaning |
|---------|---------|--------|
| Code object name | `handle_request`, or qualified depending on extraction | **Function name** from the code object; module path may appear in related metadata or in the name depending on version and pipeline. |
| C extension / interpreter | `PyEval_EvalFrameDefault`, `_cffi_*` | **CPython** internals or **native** extension code. |

**Takeaway:** Python names are often **simpler** than Java’s full signature string; line info is commonly **beside** the name in full profile models.

### Ruby

| Pattern | Example | Meaning |
|---------|---------|--------|
| Qualified method | `MyApp::OrdersController#show` | **Class/module path** + **`#`** instance method (or **`.`** for singleton)—see MRI backtrace conventions. |
| C func | Name from symbol table or `<unknown cfunc>`-style fallback | **Native** implementation of a Ruby method. |
| GC pseudo-frames | Labels indicating GC mark/sweep/compact (pipeline-specific) | Time spent in **Ruby GC** (when exposed as frames). |

### Node.js (V8)

| Pattern | Example | Meaning |
|---------|---------|--------|
| JS function | `processTicksAndRejections`, `Server.listener` | **JavaScript** function name when available. |
| V8 stub | **Stub** / **builtin** style names | **V8 internal** entry points; similar role to JVM `StubRoutines`. |
| `<anonymous>` | Anonymous | **Closure** or missing name metadata. |

### C / C++

| Pattern | Example | Meaning |
|---------|---------|--------|
| Demangled C++ | `my::ns::Foo::bar(int)` | Human-readable **namespace::class::method(params)** after demangling. |
| Itanium mangled | `_ZN2my2ns3Foo3barEi` | **Mangled** symbol (common in profiles until demangled). |
| Plain C | `read`, `epoll_wait` | **glibc** / libc and other C APIs. |

### Rust

| Pattern | Example | Meaning |
|---------|---------|--------|
| Symbol | `_RNvCsgXDX2mvAJAg_7___rustc25…` or demangled `my_crate::module::fn` | Often **mangled** with `rustc` vendor chunks unless demangler applied. |

### PHP, Perl, Erlang, .NET

| Pattern | Meaning |
|---------|--------|
| VM/runtime-specific function and method names | Same idea as Ruby/Python: **user frames** named by the runtime; **VM internals** and **JIT** stubs may appear with synthetic labels. |
| Native / kernel | Same as C and **kernel** rows above when execution crosses those layers. |

---

## 5. Quick reference: one table

| Language | Frame string usually contains | Typical “internal” frames |
|----------|-------------------------------|----------------------------|
| **Java** | Return type, FQCN, method, `(types)` | `C2 Runtime …`, `StubRoutines …`, `LambdaForm$MH…`, `jdk.internal.misc.Unsafe.*` |
| **Go** | `package.Symbol` / receiver method | `runtime.*`, `syscall.*` |
| **Python** | Function name (sometimes qualified) | `PyEval_*`, native extension symbols |
| **Ruby** | `Class#method` or `Class.method` | GC labels, `unknown` cfunc fallbacks |
| **Node (V8)** | JS name or stub label | builtins, `<anonymous>` |
| **C / C++** | C name or mangled/demangled C++ | `malloc`, `memcpy`, template instantiations in demangled form |
| **Rust** | Mangled or demangled crate path | Allocator, `std::` when demangled |
| **Kernel** | Single symbol (`tcp_sendmsg`, …) | Interrupt/syscall glue (`entry_SYSCALL_64_*`, …) |

---

## 6. How to read mixed stacks

A **single** stack can contain **Java + JNI + libc + kernel** (or **Ruby + native + kernel**, etc.). That means the thread was in a **syscall**, **I/O wait path**, **JIT-generated code**, or **kernel helper** at sample time—not that symbols are “wrong” or duplicated across languages.

---

## 7. Related code in this repository

Odigos merges and exposes profiling data in the frontend flame graph pipeline, for example:

- `frontend/services/profiles/flamegraph/tree.go` — `SymbolStats` rows (`name`, `self`, `total`) for symbol tables.
- `frontend/services/profiles/flamegraph/pyroscope_convert.go` — OTLP → merged samples → Pyroscope-shaped output.

For **how** the eBPF profiler builds Java strings from descriptors, see upstream:

- `interpreter/hotspot/demangle.go` — `demangleJavaMethod` (JVMS-style output).

---

*Last updated: conceptual reference for profiling UIs; sample strings are illustrative unless copied from a specific capture.*
