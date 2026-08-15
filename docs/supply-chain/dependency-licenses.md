# Dependency License Report

Generated: 2026-07-26 16:30:02 +08:00

This inventory covers dependencies reachable from the backend server binary and the pnpm production dependency graph. It is an engineering inventory, not legal advice. Re-run `scripts/generate-license-report.ps1` after dependency changes.

## Go production dependencies

Count: 28

| Package | Version | License |
|---|---:|---|
| github.com/gabriel-vasile/mimetype | v1.4.3 | MIT |
| github.com/gin-contrib/sse | v0.1.0 | MIT |
| github.com/gin-gonic/gin | v1.10.1 | MIT |
| github.com/golang-jwt/jwt/v5 | v5.2.2 | MIT |
| github.com/go-playground/locales | v0.14.1 | MIT |
| github.com/go-playground/universal-translator | v0.18.1 | MIT |
| github.com/go-playground/validator/v10 | v10.20.0 | MIT |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| github.com/jackc/pgx/v5 | v5.9.2 | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT |
| github.com/jinzhu/inflection | v1.0.0 | MIT |
| github.com/jinzhu/now | v1.1.5 | MIT |
| github.com/leodido/go-urn | v1.4.0 | MIT |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/pelletier/go-toml/v2 | v2.2.2 | MIT |
| github.com/ugorji/go/codec | v1.2.12 | MIT |
| go.uber.org/multierr | v1.10.0 | MIT |
| go.uber.org/zap | v1.27.0 | MIT |
| golang.org/x/crypto | v0.38.0 | BSD-3-Clause |
| golang.org/x/net | v0.25.0 | BSD-3-Clause |
| golang.org/x/sync | v0.14.0 | BSD-3-Clause |
| golang.org/x/sys | v0.33.0 | BSD-3-Clause |
| golang.org/x/text | v0.25.0 | BSD-3-Clause |
| google.golang.org/protobuf | v1.34.1 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT |
| gorm.io/driver/postgres | v1.6.0 | MIT |
| gorm.io/gorm | v1.30.1 | MIT |

## Frontend production dependencies

Count: 39

| Package | Version | License |
|---|---:|---|
| @babel/helper-string-parser | 7.29.7 | MIT |
| @babel/helper-validator-identifier | 7.29.7 | MIT |
| @babel/parser | 7.29.7 | MIT |
| @babel/types | 7.29.7 | MIT |
| @jridgewell/sourcemap-codec | 1.5.5 | MIT |
| @vue/compiler-core | 3.5.39 | MIT |
| @vue/compiler-dom | 3.5.39 | MIT |
| @vue/compiler-sfc | 3.5.39 | MIT |
| @vue/compiler-ssr | 3.5.39 | MIT |
| @vue/devtools-api | 6.6.4, 7.7.10 | MIT |
| @vue/devtools-kit | 7.7.10 | MIT |
| @vue/devtools-shared | 7.7.10 | MIT |
| @vue/reactivity | 3.5.39 | MIT |
| @vue/runtime-core | 3.5.39 | MIT |
| @vue/runtime-dom | 3.5.39 | MIT |
| @vue/server-renderer | 3.5.39 | MIT |
| @vue/shared | 3.5.39 | MIT |
| birpc | 2.9.0 | MIT |
| copy-anything | 4.0.5 | MIT |
| csstype | 3.2.3 | MIT |
| entities | 7.0.1 | BSD-2-Clause |
| estree-walker | 2.0.2 | MIT |
| hookable | 5.5.3 | MIT |
| is-what | 5.5.0 | MIT |
| lucide-vue-next | 0.525.0 | ISC |
| magic-string | 0.30.21 | MIT |
| mitt | 3.0.1 | MIT |
| nanoid | 3.3.17 | MIT |
| perfect-debounce | 1.0.0 | MIT |
| picocolors | 1.1.1 | ISC |
| pinia | 3.0.4 | MIT |
| postcss | 8.5.19 | MIT |
| rfdc | 1.4.1 | MIT |
| source-map-js | 1.2.1 | BSD-3-Clause |
| speakingurl | 14.0.1 | BSD-3-Clause |
| superjson | 2.2.6 | MIT |
| typescript | 5.8.3 | Apache-2.0 |
| vue | 3.5.39 | MIT |
| vue-router | 4.6.4 | MIT |

## Review policy

- `UNKNOWN`, `SEE-LICENSE`, GPL, LGPL or other reciprocal licenses require manual review before redistribution.
- This report records third-party dependencies only; reference projects are documented separately and are not copied into the application source tree.
