// Manifest divergence guard.
//
// The upstream-parity byte and semantic guards exclude package.json on
// purpose: manifests are the fork's deliberate divergence surface. That
// exclusion left the manifests unguarded, and unverified manifest bumps
// broke the workspace repeatedly (typescript ^7 crashed every install
// twice, js-yaml ^5 broke the include build twice, a bump-everything pass
// shipped both). This script is the machine gate for that surface: every
// workspace manifest (root package.json + packages/*/package.json) must
// match the pinned upstream commit exactly, except for the explicit
// allowlist below. Sync manifests and bump the pin in the same commit.
//
// Run from the repo root (the pin commit must exist in the local git store):
//   node scripts/manifest-parity.mjs
import { readFileSync } from 'node:fs'
import { execSync } from 'node:child_process'

const pin = process.env.UPSTREAM_PIN?.trim() || readFileSync('.github/UPSTREAM_PIN', 'utf8').trim()

// path -> section -> allowlisted keys (values may diverge freely)
const ALLOWED = {
  'package.json': {
    devDependencies: ['@types/node'],
  },
}

function git(args) {
  return execSync(`git ${args}`, { encoding: 'utf8' }).trim()
}

function manifestPaths(ref) {
  const listing = git(`ls-tree -r --name-only ${ref} -- package.json packages`)
  return listing
    .split('\n')
    .filter(Boolean)
    .filter((path) => path === 'package.json' || /(^|\/)package\.json$/.test(path))
}

const forkPaths = new Set(manifestPaths('HEAD'))
const pinPaths = new Set(manifestPaths(pin))

const failures = []

for (const path of [...forkPaths].filter((p) => !pinPaths.has(p))) {
  failures.push(`${path}: exists in the fork but not in pin ${pin}`)
}
for (const path of [...pinPaths].filter((p) => !forkPaths.has(p))) {
  failures.push(`${path}: exists in pin ${pin} but not in the fork`)
}

const allowedFor = (path) => ALLOWED[path] ?? {}

for (const path of [...forkPaths].filter((p) => pinPaths.has(p))) {
  const forkManifest = JSON.parse(readFileSync(path, 'utf8'))
  const pinManifest = JSON.parse(git(`show ${pin}:${path}`))
  const sections = new Set([...Object.keys(forkManifest), ...Object.keys(pinManifest)])
  for (const section of sections) {
    const forkSection = forkManifest[section]
    const pinSection = pinManifest[section]
    const allowlist = allowedFor(path)[section]
    if (allowlist) {
      for (const key of new Set([
        ...Object.keys(forkSection ?? {}),
        ...Object.keys(pinSection ?? {}),
      ])) {
        if (allowlist.includes(key)) continue
        if (JSON.stringify(forkSection?.[key]) !== JSON.stringify(pinSection?.[key])) {
          failures.push(
            `${path}: ${section}.${key}: fork=${JSON.stringify(forkSection?.[key])} pin=${JSON.stringify(pinSection?.[key])}`,
          )
        }
      }
    } else if (JSON.stringify(forkSection) !== JSON.stringify(pinSection)) {
      failures.push(
        `${path}: ${section}: fork=${JSON.stringify(forkSection)} pin=${JSON.stringify(pinSection)}`,
      )
    }
  }
}

if (failures.length > 0) {
  console.error(`manifest-parity: ${failures.length} divergence(s) outside the allowlist:`)
  for (const failure of failures) console.error(`  ${failure}`)
  process.exit(1)
}

console.log(`manifest-parity: all manifests match pin ${pin} (allowlisted keys aside)`)
