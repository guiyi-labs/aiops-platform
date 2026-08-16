// M100-D: license-scan frontend parser. Reads `pnpm licenses list --prod
// --json` from stdin, verifies every package license against ALLOWLIST
// (space-separated, from env), exits 1 on any non-allowlisted license.
//
// Tolerates pnpm output shape drift across versions: the top-level JSON is
// keyed by license ID (v10/v11 classic) or by severity ("licenses" object),
// and a given key's value may be an array of package objects or a bare
// package object. Non-package values are skipped, never fatal; an empty
// stream is treated as "nothing to verify" (a prior `pnpm install` failure is
// caught by the caller before the scan runs).
let input = "";
for await (const chunk of process.stdin) input += chunk;
const allowlist = (process.env.ALLOWLIST ?? "").split(" ").filter(Boolean);
let fail = 0;

function packagesFor(value) {
  if (Array.isArray(value)) return value;
  if (value && typeof value === "object" && typeof value.name === "string") return [value];
  // pnpm >= 10 with the licenses plugin emits {"license": "MIT", "name": ...}
  // entries; older shapes are handled above. Anything else (null, string,
  // number, empty object) is not a package list.
  return [];
}

function packageVersions(pkg) {
  if (typeof pkg === "string") return pkg;
  if (Array.isArray(pkg.versions)) return pkg.versions.join(",");
  if (typeof pkg.version === "string") return pkg.version;
  return String(pkg.versions ?? "");
}

try {
  const groups = JSON.parse(input.trim());
  // Nested form: { "licenses": { "MIT": [...], ... } }
  const entries = groups && typeof groups === "object" && groups.licenses && typeof groups.licenses === "object"
    ? Object.entries(groups.licenses)
    : Object.entries(groups ?? {});
  for (const [license, packages] of entries) {
    for (const pkg of packagesFor(packages)) {
      if (!allowlist.includes(license)) {
        const name = typeof pkg === "string" ? pkg : String(pkg.name ?? "unknown");
        console.log(`  LICENSE CHECK FAIL: ${name}@${packageVersions(pkg)} -> ${license}`);
        fail = 1;
      }
    }
  }
} catch (error) {
  console.error(`  license JSON parse failed: ${error.message}`);
  process.exit(1);
}
process.exit(fail);