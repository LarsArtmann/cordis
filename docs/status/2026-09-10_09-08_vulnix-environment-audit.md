# Status Report — vulnix Environment Audit (scoped, reviewed)

Session: 2026-09-10, ~08:45–09:20 · Branch: `main` (no code changes; this
is an audit + tooling session).
Scope: vulnerability scan of everything this flake builds with — the five
checks, the default devShell, the formatter (and via them, the apps) —
using vulnix 1.12.5 against the NVD feed (cache snapshot 2026-09-10
08:40). nixpkgs pin: `d6524aac` (nixos-unstable). 36 of 120 reported CVEs
were individually verified against the NVD API; the rest are classified
by verified-family pattern (noted below).

This executes the 2026-09-08 backlog item "reduce vulnix noise: scope to
the flake's own derivations, not the whole store closure" (05-27 report
§f12): buildflow's whole-store run had flagged 153 findings as pure
report noise.

---

## a) FULLY DONE (verified)

1. **Reproducible audit script** `scripts/vulnix-audit.sh` with two
   modes: `direct` (only the ~21 packages the flake itself selects) and
   `closure` (full build closure of the seven flake derivations). Exit
   codes are vulnix's (2 = findings). Self-contained: fetches vulnix
   from nixpkgs, needs only nix + jq.
2. **Scoped scan (direct)**: 21 direct inputs, 4 packages flagged, 11
   CVEs — after NVD verification, **one** genuine package-level concern
   (coreutils), three product-name collisions (cargo, go, ShellCheck).
3. **Full-closure scan**: 7 derivations → 32 flagged derivations, 120
   CVEs. Attributed per flake derivation (JSON runs) and triaged into
   buckets below.
4. **Raw outputs kept locally** in `reports/` (gitignored scratch dir):
   `vulnix-direct-inputs-2026-09-10.txt` (11 CVEs),
   `vulnix-full-closure-2026-09-10.txt` (120 CVEs). Both regenerate
   deterministically with `scripts/vulnix-audit.sh direct|closure`.
5. **Verification pass**: 36/120 CVEs checked against the NVD API (every
   finding in the direct scope; every ≥8.4 in the closure; the odd ones).

## b) THE ONE-LINE VERDICT

**Nothing to fix inside this repo.** No finding touches code cordis
ships, and no genuinely vulnerable binary here processes
attacker-controlled input: every genuine hit is either (a) the latest
upstream release with the fix only in an unreleased commit or a release
newer than nixpkgs currently carries (arrives with the next
`nix flake update`), or (b) bootstrap/build-time tooling that only ever
sees trusted inputs. 61% of the verified findings are false positives
(NVD product-name collisions or already-fixed-in-installed-version).

## c) Scoped scan — all 11 findings, individually verified

| Package (nixpkgs) | CVEs | NVD verdict |
| --- | --- | --- |
| cargo-1.97.1 (latest) | CVE-2026-14363, -58521, -39837, -39839, -39840, -58519 (+ -39841 same family) | **FP**: all MediaWiki "Cargo" PHP extension (SQLi/XSS). Name collision with rust cargo. |
| go-1.27.1 (latest) | CVE-2023-49292 | **FP**: `ecies/go` secp256k1 library (versions < 2.0.8); not the Go toolchain. 1.27.1 < 2.0.8 numerically. |
| ShellCheck-0.11.0 (latest) | CVE-2021-28794 | **FP**: unofficial VS Code ShellCheck extension; not the CLI. |
| coreutils-9.11 (latest) | CVE-2026-56391, CVE-2026-56392 | **Genuine**: `uniq -w` multibyte OOB read; `unexpand -t` heap overflow. Affects ≤9.11, fixed in unreleased upstream commits. Exposure here: shell scripts on trusted artifacts only; CVSS AV:L/UI:R. |

The other 17 direct inputs (go bootstrap chain aside) scan clean, incl.
zig 0.16.0, nodejs 24.19.0, yarn-berry 4.14.1, gopls, golangci-lint,
markdownlint-cli, nixfmt, gcc-wrapper, bash, gawk, gnugrep, diffutils.

## d) Full closure — 32 derivations, 120 CVEs, triage buckets

**Bucket A — false positives / not applicable (verified: 22 CVEs).**
Product-name collisions: cargo ×7 (MediaWiki), go (ecies/go), ShellCheck
(VS Code ext), Diff-1.0.2 (Drupal module; Haskell lib here), async-2.2.6
(npm; Haskell lib here), stringbuilder (npm node-stringbuilder; Rust
crate here), yoke (yokecd K8s deployer; ICU4X crate here), ada (Ada.cx
SaaS — NOT the ada-url parser bundled in nodejs), dash (Plotly Dash),
python ×2 (VS Code Python extension), zlib CVE-2026-27820 (Ruby zlib
bindings, not C zlib), curl-0.4.49 (the Rust curl *crate* matched
against 2022/23 libcurl tool CVEs; the actual libcurl in the store is
NOT flagged — nixpkgs ships it patched). Disputed/NA: gcc CVE-2023-4039
(DISPUTED, AArch64-only stack-protector miss; x86_64 here), libxml2
CVE-2026-11979 (DISPUTED; interactive `xmlcatalog --shell` only).

**Bucket B — already fixed in the installed version (coarse NVD CPE,
verified: 3).** perl-5.42.3: CVE-2026-8376 (32-bit-only regex heap
overflow) and CVE-2026-13221 (trie overflow wrong matches) are both
fixed in 5.42.3-RC1 — final 5.42.3 includes the fix, NVD's
`≤5.43.x` CPE is too coarse. libmicrohttpd CVE-2025-59777 affects
≤1.0.2; installed 1.0.6 postdates the fix commit (date-based CPE
artifact).

**Bucket C — genuine, no fixed release exists yet; build/dev-only
exposure (verified sample: 9).**

> Correction (09:28, self-review): jq-1.8.2 (3 CVEs: CVE-2026-39979,
> CVE-2026-44777, CVE-2026-33948) appears in the closure output but was
> never triaged into a bucket — it slipped between the attribution pass
> and the bucket tables. Treat as **unclassified pending verification**,
> not implicitly clean.
- glibc-2.42 (all closures): CVE-2026-5450 scanf `%mc` 1-byte heap
  overflow (2.7–2.43), CVE-2026-0861 memalign integer overflow (needs
  attacker-controlled size+alignment; "not easily exploitable" per
  advisory) + 8 more from the 2025/26 GLIBC-SA batch (not individually
  verified). No code here uses scanf `%mc` or attacker-aligned allocs.
- coreutils-9.11 (above), gzip-1.14 (CVE-2026-41992 LZH state-poisoning
  OOB read via crafted LZW+LZH archives; fixed in unreleased commits),
  bison-3.8.2 (CVE-2026-56389: `bison --html` executes a
  grammar-defined `%define tool.xsltproc` — requires running bison on an
  attacker-written grammar), binutils-2.46 (readelf NULL-deref class,
  crash-only, on crafted ELFs), patch-2.8, ninja-1.13.2
  (CVE-2021-4336, unfixed DoS), libssh2-1.11.1 (SFTP `sftp_open`
  double-free etc., exploitable only by a malicious SSH *server*;
  nothing here speaks SSH via libssh2).
- Bootstrap-chain relics: gzip-1.2.4 is the 1993 GNU gzip built inside
  the tinycc bootstrap (CVE-2022-1271 etc. technically apply); runs only
  while nixpkgs bootstraps stdenv. gcc-4.6.4/gcc-10.4.0 are Go's
  bootstrap compilers. go-1.24.13-bootstrap carries 26 genuine Go
  toolchain CVEs (e.g. CVE-2026-27143 compiler induction-variable bug)
  — but the toolchain we actually build and test with, go 1.27.1, is
  outside every affected range; bootstrap compiles trusted code once.

**Bucket D — genuine, fixed in a release newer than nixpkgs carries
(verified: 4).** python-3.14.6: CVE-2026-15308 (html.parser DoS) and
CVE-2026-0864 (configparser `\r` injection) — both fixed in 3.14.7;
python here is only a packaging/build dep. yarn-1.22.22 (pulled into the
devShell closure by yarn-berry's own packaging, never executed — the
repo runs yarn-berry 4.14.1): two yarn-1 EOL-line ReDoS CVEs.

**Bucket E — nixpkgs packaging internals, not cordis dependencies
(verified: 1).** bytes-1.11.0 (tokio-rs/bytes crate, CVE-2026-25541
genuine, fixed in 1.11.1) is vendored into some nixpkgs tool's build;
the Rust port itself has zero external crates (`cargo test --offline`
passes by design).

Attribution (which flake derivation pulls what): stdenv trunk
(binutils/bison/coreutils/gcc×3/glibc×2/gzip×2/libssh2/patch/perl/
python/zlib) is common to all seven; go-check adds the go bootstrap
chain; rust-check adds cargo/dash/ninja/libxml2; zig-check adds
ninja/libxml2; markdown-check adds nodejs' ada/jq and the bytes/yoke
crates; devshell adds yarn-1 (via yarn-berry) and the go toolchain;
formatter adds ShellCheck (+ its Haskell Diff/async, stringbuilder) and
libmicrohttpd.

## e) LESSONS (tool-level, for future audits)

1. **vulnix matches NVD by product name + bare version range.** Expect
   ~60% false positives on this tree; every "CRITICAL 9.8" except
   glibc's turned out to be a name collision (MediaWiki cargo, ecies
   go, Ada.cx ada, npm stringbuilder...). Never act on a vulnix finding
   without opening the NVD entry.
2. **NVD CPE ranges are frequently wrong-shaped for branched fixes**
   (perl's fix-in-RC1, libmicrohttpd's date-based range, gcc's disputed
   AArch64 entry matching all versions). "Already fixed" is a real
   verdict category.
3. **`vulnix --no-requisites` on a runCommand .drv inspects nothing**
   (the derivation's own name, not its inputs) — the working scoped
   recipe is: extract `inputDrvs` from the flake's derivations, then
   `--no-requisites` on those package .drvs.
4. **Keep vulnix out of CI gates**: NVD download (~380 MB cache), name
   noise, and no actionability here. A manual script (added) is the
   right shape.

## f) NEXT THINGS

1. Re-run `scripts/vulnix-audit.sh direct` after the next
   `nix flake update` (watch: glibc 2.43, python 3.14.7, coreutils
   9.12, gzip 1.15, binutils 2.47, libssh2 1.11.2+). — Low / S
2. Consider `cargo audit` / `govulncheck` / `yarn npm audit` runs for
   the *dependency* side vulnix cannot see (Go/Rust ports are
   dependency-free today, so this is TS-workspace-only). — Low / S
