# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Repo

- [ ] Verify green CI on GitHub for the line carrying the new `flake` and
      `upstream-parity` jobs (pushed as `fa45896`) AND for the manifest
      repair that reverts `typescript` to `^5.9.3` (0542b6d reintroduced
      `^7.0.2`, crashing every `yarn install` on the yarn
      `lib/_tsc.js` lstat — see AGENTS.md "Toolchain pins are
      load-bearing"); both Build and Ports must pass, including the
      thread-safe clippy gate (source: docs/status/2026-09-08_15-43 §c)
