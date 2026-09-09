// Fixture-coupling canary for packages/hmr.
//
// The hmr specs mutate fixture sources through literal replaces, e.g.
//   plugin.modify((c) => c.replace("value = 'initial'", "value = 'modified'"))
// If a fixture is reformatted so the literal no longer occurs in it, the
// replace silently no-ops and every reload test dies in a waitFor timeout
// (~190 s) instead of failing fast. This script asserts every string-literal
// first argument of .replace() in the specs exists in at least one fixture
// file under packages/hmr/tests, catching the hazard in milliseconds.
//
// Run from anywhere: node scripts/hmr-fixture-canary.mjs
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const testsDir = join(dirname(fileURLToPath(import.meta.url)), "..", "packages", "hmr", "tests");

const allFiles = readdirSync(testsDir);
const specs = allFiles.filter((f) => f.endsWith(".spec.ts"));
const fixtures = allFiles.filter((f) => !f.endsWith(".spec.ts") && f !== "tsconfig.json");

if (specs.length === 0 || fixtures.length === 0) {
  console.error(`hmr-fixture-canary: expected specs and fixtures in ${testsDir}`);
  process.exit(2);
}

const corpus = new Map(fixtures.map((f) => [f, readFileSync(join(testsDir, f), "utf8")]));

const literalPattern = /\.replace\(\s*(['"])((?:\\.|(?!\1).)*)\1/g;

function unescapeJs(literal) {
  return literal.replace(/\\(.)/g, (_, char) => {
    if (char === "n") return "\n";
    if (char === "t") return "\t";
    return char;
  });
}

let failures = 0;
let checked = 0;

for (const spec of specs) {
  const source = readFileSync(join(testsDir, spec), "utf8");
  // Inline template content in the spec (specs that generate their plugin
  // sources) counts as fixture content, but the replace calls themselves
  // must not: strip every matched first-argument span before searching.
  const cleanedSource = source.replace(literalPattern, "");
  const searchable = [...corpus, [`${spec} (inline content)`, cleanedSource]];
  let match;
  literalPattern.lastIndex = 0;
  while ((match = literalPattern.exec(source))) {
    checked += 1;
    const literal = unescapeJs(match[2]);
    const line = source.slice(0, match.index).split("\n").length;
    const presentIn = searchable.filter(([, content]) => content.includes(literal));
    if (presentIn.length === 0) {
      failures += 1;
      console.error(
        `FAIL ${spec}:${line} replace literal not found in any fixture: ${JSON.stringify(literal)}`,
      );
    }
  }
}

if (failures > 0) {
  console.error(`hmr-fixture-canary: ${failures}/${checked} replace literals missing from fixtures`);
  process.exit(1);
}

console.log(`hmr-fixture-canary: ${checked} replace literals all present in ${fixtures.length} fixtures`);
