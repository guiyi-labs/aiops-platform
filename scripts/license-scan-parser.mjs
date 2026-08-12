// M100-D: license-scan frontend parser. Reads `pnpm licenses list --prod
// --json` from stdin, verifies every package license against ALLOWLIST
// (space-separated, from env), exits 1 on any non-allowlisted license.
let input = "";
for await (const chunk of process.stdin) input += chunk;
const allowlist = (process.env.ALLOWLIST ?? "").split(" ").filter(Boolean);
let fail = 0;
try {
  const groups = JSON.parse(input);
  for (const [license, packages] of Object.entries(groups)) {
    for (const pkg of packages ?? []) {
      if (!allowlist.includes(license)) {
        const versions = Array.isArray(pkg.versions) ? pkg.versions.join(",") : String(pkg.versions ?? "");
        console.log(`  LICENSE CHECK FAIL: ${pkg.name}@${versions} -> ${license}`);
        fail = 1;
      }
    }
  }
} catch (error) {
  console.error(`  license JSON parse failed: ${error.message}`);
  process.exit(1);
}
process.exit(fail);
