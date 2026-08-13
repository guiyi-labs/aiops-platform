import { createHash } from 'node:crypto'
import { createReadStream, existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs'
import { basename, dirname, join, relative, resolve } from 'node:path'
import { createGunzip } from 'node:zlib'

const EXPECTED = {
  fixture: {
    schemaVersion: 'aiops.scale-fixture/v1',
    datasetVersion: 'm96-v1',
    seed: 20260810,
    counts: { nodes: 500, workloads: 5000, pods: 50000, events: 100000 },
    configSha256: '414baa5aa90c50040a4f2bae2fd94e1a007100b86c910a31c3f81fb288b8cf5c',
    datasetSha256: '81faa1de39eaca4dfb84944ebd7bf155bdc1e3716e5f1ae6431bcdb406647c71',
    requiredStreams: ['nodes.ndjson.gz', 'workloads.ndjson.gz', 'pods.ndjson.gz', 'events.ndjson.gz', 'history.ndjson.gz'],
  },
  frontend: {
    schema: 1,
    podVersion: 'm96-pods-v1',
    samplesVersion: 'm96-pod-scale-samples-v1',
    baselineVersion: 'm96-pod-scale-baseline-v1',
    payloadSha256: '01274b8d8223887fdc80d211f61a9579605842ca8d096b7fcd8e282f4dbf02cd',
    payloadBytes: 18890589,
    configSha256: '7f1c3f0336b009a4daaa0ce55820f79751af5cfbaa005ead43c8124cf147c7f6',
    podCount: 50000,
    profileNames: ['desktop', 'mobile'],
    repeatsPerProfile: 3,
    hardRenderedRowLimit: 40,
  },
  css: {
    schema: 1,
    version: 'm96-style-baseline-v1',
    mode: 'fail-closed',
    importOrder: ['base.css', 'console-theme.css', 'motion.css', 'premium-ui.css'],
    removedLayer: 'kubesphere-theme.css',
  },
}

const REQUIRED_SAMPLE_INVARIANTS = [
  'fixtureCountExact',
  'renderedRowsBounded',
  'virtualWindowBounded',
  'scrollHeightCoversFixture',
  'scrollTargetVisible',
  'scrollWindowUsesOverscan',
  'scrollPositionStable',
  'filterMatchedOne',
  'filterTargetExact',
  'consoleErrorsZero',
]

const args = parseArgs(process.argv.slice(2))
const root = resolve(args.root ?? process.cwd())
const mode = (process.env.GATE_B_MODE || 'report').trim()
const output = resolve(args.output ?? join(root, '.artifacts', 'm96-gate-b', 'm96-gate-b.json'))
const markdown = resolve(args.markdown ?? output.replace(/\.json$/i, '.md'))
const checks = []
const errors = []
const warnings = []

function parseArgs(values) {
  const parsed = {}
  for (let index = 0; index < values.length; index += 1) {
    const value = values[index]
    if (!value.startsWith('--')) throw new Error(`Unexpected argument: ${value}`)
    const [key, inlineValue] = value.slice(2).split('=', 2)
    if (inlineValue !== undefined) parsed[key] = inlineValue
    else if (values[index + 1] && !values[index + 1].startsWith('--')) parsed[key] = values[++index]
    else parsed[key] = true
  }
  return parsed
}

function addCheck(name, passed, observed, options = {}) {
  const displayObserved = observed && typeof observed === 'object' ? JSON.stringify(observed) : observed
  const check = { name, status: passed ? 'passed' : options.warning ? 'warning' : 'failed', observed: displayObserved }
  checks.push(check)
  if (!passed) (options.warning ? warnings : errors).push(`${name}: ${displayObserved}`)
  return passed
}

function requireValue(name, actual, expected) {
  return addCheck(name, actual === expected, `expected ${JSON.stringify(expected)}, observed ${JSON.stringify(actual)}`)
}

function requireTruthy(name, value, observed = value) {
  return addCheck(name, value === true, observed)
}

function readText(path) {
  const buffer = readFileSync(path)
  if (buffer[0] === 0xff && buffer[1] === 0xfe) return buffer.subarray(2).toString('utf16le')
  if (buffer[0] === 0xfe && buffer[1] === 0xff) return decodeUtf16Be(buffer.subarray(2))
  if (buffer[1] === 0x00 || buffer[2] === 0x00) return buffer.toString('utf16le').replace(/^\uFEFF/, '')
  return buffer.toString('utf8').replace(/^\uFEFF/, '')
}

function decodeUtf16Be(buffer) {
  const swapped = Buffer.allocUnsafe(buffer.length)
  for (let index = 0; index < buffer.length; index += 2) {
    swapped[index] = buffer[index + 1]
    swapped[index + 1] = buffer[index]
  }
  return swapped.toString('utf16le')
}

function readJson(path) {
  try {
    return JSON.parse(readText(path))
  } catch (error) {
    throw new Error(`Invalid JSON in ${path}: ${error.message}`)
  }
}

function listFiles(directory) {
  if (!existsSync(directory)) return []
  const entries = readdirSync(directory, { withFileTypes: true })
  return entries.flatMap((entry) => {
    const path = join(directory, entry.name)
    return entry.isDirectory() ? listFiles(path) : [path]
  })
}

function findRequiredFile(fileName, pathHint) {
  const candidates = listFiles(root).filter((path) => basename(path) === fileName)
  const preferred = candidates.filter((path) => pathHint.test(relative(root, path)))
  const selected = preferred.length === 1 ? preferred[0] : candidates.length === 1 ? candidates[0] : null
  if (!selected) {
    const details = candidates.length === 0 ? 'not found' : `ambiguous candidates: ${candidates.map((path) => relative(root, path)).join(', ')}`
    errors.push(`${fileName}: ${details}`)
    checks.push({ name: `evidence/${fileName}`, status: 'failed', observed: details })
    return null
  }
  checks.push({ name: `evidence/${fileName}`, status: 'passed', observed: relative(root, selected) })
  return selected
}

function findOptionalFile(fileName, pathHint) {
  const candidates = listFiles(root).filter((path) => basename(path) === fileName)
  const preferred = candidates.filter((path) => pathHint.test(relative(root, path)))
  const selected = preferred.length === 1 ? preferred[0] : candidates.length === 1 ? candidates[0] : null
  if (!selected && candidates.length > 1) warnings.push(`${fileName}: ambiguous optional candidates were ignored`)
  if (selected) checks.push({ name: `evidence/${fileName}`, status: 'passed', observed: relative(root, selected) })
  return selected
}

function isSha256(value) {
  return typeof value === 'string' && /^[a-f0-9]{64}$/i.test(value)
}

function isCommit(value) {
  return typeof value === 'string' && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/i.test(value)
}

function canonicalize(value) {
  if (Array.isArray(value)) return value.map(canonicalize)
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonicalize(value[key])]))
  }
  return value
}

function canonicalJson(value) {
  return JSON.stringify(canonicalize(value))
}

function fixtureIdentity(value) {
  return {
    schema_version: value.schema_version,
    dataset_version: value.dataset_version,
    seed: value.seed,
    cluster_id: value.cluster_id,
    observed_at: value.observed_at,
    config_sha256: value.config_sha256,
    summary: value.summary,
    artifacts: value.artifacts,
    dataset_sha256: value.dataset_sha256,
  }
}

async function validateFixture(manifestPath, verificationPath, generationPath, backendPath) {
  if (!manifestPath || !verificationPath || !backendPath) return
  const manifest = readJson(manifestPath)
  const verification = readJson(verificationPath)
  const generation = generationPath ? readJson(generationPath) : null
  const counts = manifest.summary?.counts ?? {}

  requireValue('fixture/schema', manifest.schema_version, EXPECTED.fixture.schemaVersion)
  requireValue('fixture/dataset_version', manifest.dataset_version, EXPECTED.fixture.datasetVersion)
  requireValue('fixture/seed', manifest.seed, EXPECTED.fixture.seed)
  for (const [name, expected] of Object.entries(EXPECTED.fixture.counts)) requireValue(`fixture/counts/${name}`, counts[name], expected)
  requireValue('fixture/dataset_sha256', manifest.dataset_sha256, EXPECTED.fixture.datasetSha256)
  requireValue('fixture/config_sha256', manifest.config_sha256, EXPECTED.fixture.configSha256)

  const artifacts = Array.isArray(manifest.artifacts) ? manifest.artifacts : []
  const artifactNames = artifacts.map((artifact) => artifact.name)
  requireValue('fixture/artifacts/count', artifactNames.length, EXPECTED.fixture.requiredStreams.length)
  for (const stream of EXPECTED.fixture.requiredStreams) {
    const artifact = artifacts.find((candidate) => candidate.name === stream)
    requireTruthy(`fixture/artifacts/${stream}`, Boolean(artifact), artifact ? 'present' : 'missing')
    if (artifact) {
      requireTruthy(`fixture/artifacts/${stream}/sha256`, isSha256(artifact.sha256), artifact.sha256)
      requireTruthy(`fixture/artifacts/${stream}/records`, Number.isInteger(artifact.records) && artifact.records > 0, artifact.records)
      requireTruthy(`fixture/artifacts/${stream}/compressed_bytes`, Number.isInteger(artifact.compressed_bytes) && artifact.compressed_bytes > 0, artifact.compressed_bytes)
      requireValue(`fixture/artifacts/${stream}/compression`, artifact.compression, 'gzip')
    }
  }

  for (const [name, value] of [['manifest', manifest], ['verification', verification], ['generation', generation]]) {
    if (value) requireTruthy(`fixture/${name}_identity`, canonicalJson(fixtureIdentity(value)) === canonicalJson(fixtureIdentity(manifest)), 'matches manifest identity')
  }

  const backend = readJson(backendPath)
  requireValue('backend/schema', backend.schema_version, 'aiops.scale-benchmark/v1')
  requireValue('backend/mode', backend.mode, mode)
  requireValue('backend/fixture/dataset_version', backend.fixture?.dataset_version, manifest.dataset_version)
  requireValue('backend/fixture/config_sha256', backend.fixture?.config_sha256, manifest.config_sha256)
  requireValue('backend/fixture/dataset_sha256', backend.fixture?.dataset_sha256, manifest.dataset_sha256)
  requireTruthy('backend/samples', Number.isInteger(backend.samples) && backend.samples >= 30, backend.samples)
  requireTruthy('backend/warmup', Number.isInteger(backend.warmup) && backend.warmup >= 3, backend.warmup)
  const operationNames = [
    'topology_derive_all_namespaces',
    'global_search_api',
    'pods_paginate_all',
    'events_paginate_all',
    'history_query_node_cpu',
    'history_query_pod_cpu',
    'history_evaluate_pod_cpu',
    'pod_stream_backpressure',
  ]
  const operations = Array.isArray(backend.operations) ? backend.operations : []
  requireValue('backend/operations', JSON.stringify(operations.map((operation) => operation.name)), JSON.stringify(operationNames))
  for (const name of operationNames) {
    const operation = operations.find((candidate) => candidate.name === name)
    requireTruthy(`backend/operation/${name}/stats`, Boolean(operation?.stats), operation?.stats)
    for (const field of ['p50_ms', 'p95_ms', 'p99_ms']) requireTruthy(`backend/operation/${name}/${field}`, typeof operation?.stats?.[field] === 'number' && Number.isFinite(operation.stats[field]), operation?.stats?.[field])
  }
  requireTruthy('backend/invariants', Array.isArray(backend.invariants) && backend.invariants.length >= 7, backend.invariants?.length)
  for (const invariant of backend.invariants ?? []) requireTruthy(`backend/invariant/${invariant.name}`, invariant.passed === true, invariant.observed)
  requireTruthy('backend/environment/commit', isCommit(backend.environment?.commit), backend.environment?.commit)
  validateOptionalExpectedCommit('backend/environment/commit', backend.environment?.commit)

  await validateFixtureStreams(manifest)
}

function validateOptionalExpectedCommit(name, actual) {
  if (!args['expected-commit']) return
  requireValue(name, actual, args['expected-commit'])
}

async function validateFixtureStreams(manifest) {
  const streamPaths = EXPECTED.fixture.requiredStreams.map((name) => findOptionalFile(name, /scale-fixture[\\/]m96-v1/))
  const present = streamPaths.filter(Boolean)
  if (present.length === 0) {
    addCheck('fixture/stream_revalidation', true, 'skipped: CI evidence contains manifest only')
    return
  }
  if (present.length !== streamPaths.length) {
    addCheck('fixture/stream_revalidation', false, 'partial fixture stream set is present')
    return
  }
  for (const artifact of manifest.artifacts) {
    const path = streamPaths.find((candidate) => basename(candidate) === artifact.name)
    const observed = await verifyGzip(path)
    requireValue(`fixture/stream/${artifact.name}/sha256`, observed.sha256, artifact.sha256)
    requireValue(`fixture/stream/${artifact.name}/compressed_bytes`, observed.compressedBytes, artifact.compressed_bytes)
    requireValue(`fixture/stream/${artifact.name}/uncompressed_bytes`, observed.uncompressedBytes, artifact.uncompressed_bytes)
  }
  addCheck('fixture/stream_revalidation', true, 'all five gzip streams rehashed')
}

function verifyGzip(path) {
  return new Promise((resolvePromise, reject) => {
    const hash = createHash('sha256')
    let uncompressedBytes = 0
    const stream = createReadStream(path)
    stream.on('error', reject)
    const gunzip = createGunzip()
    gunzip.on('data', (chunk) => { uncompressedBytes += chunk.length; hash.update(chunk) })
    gunzip.on('error', reject)
    gunzip.on('end', () => resolvePromise({
      compressedBytes: statSync(path).size,
      uncompressedBytes,
      sha256: hash.digest('hex'),
    }))
    stream.pipe(gunzip)
  })
}

function validateFrontend(baselinePath, samplesPath) {
  if (!baselinePath || !samplesPath) return
  const baseline = readJson(baselinePath)
  const samples = readJson(samplesPath)
  const fixture = samples.fixture ?? {}
  const config = fixture.config ?? {}

  requireValue('frontend/baseline/schema', baseline.schema, EXPECTED.frontend.schema)
  requireValue('frontend/baseline/version', baseline.version, EXPECTED.frontend.baselineVersion)
  requireValue('frontend/samples/schema', samples.schema, EXPECTED.frontend.schema)
  requireValue('frontend/samples/version', samples.version, EXPECTED.frontend.samplesVersion)
  requireValue('frontend/commit_match', baseline.commit, samples.commit)
  validateOptionalExpectedCommit('frontend/commit', samples.commit)
  requireValue('frontend/fixture/config_sha256', fixture.configSha256, EXPECTED.frontend.configSha256)
  requireValue('frontend/fixture/payload_sha256', fixture.payloadSha256, EXPECTED.frontend.payloadSha256)
  requireValue('frontend/fixture/payload_bytes', fixture.payloadBytes, EXPECTED.frontend.payloadBytes)
  requireValue('frontend/fixture/version', config.version, EXPECTED.frontend.podVersion)
  requireValue('frontend/fixture/pod_count', config.pod_count, EXPECTED.frontend.podCount)
  requireValue('frontend/fixture/node_count', config.node_count, 500)
  requireValue('frontend/fixture/namespace_count', config.namespace_count, 100)
  requireValue('frontend/budget/mode', baseline.budget?.mode, mode)
  requireValue('frontend/baseline/fixture/config_sha256', baseline.fixture?.configSha256, fixture.configSha256)
  requireValue('frontend/baseline/fixture/payload_sha256', baseline.fixture?.payloadSha256, fixture.payloadSha256)
  requireValue('frontend/baseline/fixture/payload_bytes', baseline.fixture?.payloadBytes, fixture.payloadBytes)

  const sampleList = Array.isArray(samples.samples) ? samples.samples : []
  requireValue('frontend/samples/count', sampleList.length, EXPECTED.frontend.profileNames.length * EXPECTED.frontend.repeatsPerProfile)
  for (const profile of EXPECTED.frontend.profileNames) {
    const profileSamples = sampleList.filter((sample) => sample.profile === profile)
    requireValue(`frontend/samples/${profile}/count`, profileSamples.length, EXPECTED.frontend.repeatsPerProfile)
    const iterations = profileSamples.map((sample) => sample.iteration).sort((left, right) => left - right)
    requireValue(`frontend/samples/${profile}/iterations`, JSON.stringify(iterations), JSON.stringify([1, 2, 3]))
  }

  requireTruthy('frontend/failures_empty', Array.isArray(samples.failures) && samples.failures.length === 0, samples.failures?.length)
  requireTruthy('frontend/invariant_failures_empty', Array.isArray(samples.invariantFailures) && samples.invariantFailures.length === 0, samples.invariantFailures?.length)
  for (const sample of sampleList) {
    for (const invariant of REQUIRED_SAMPLE_INVARIANTS) requireTruthy(`frontend/sample/${sample.profile}-${sample.iteration}/${invariant}`, sample.invariants?.[invariant] === true, sample.invariants?.[invariant])
    requireTruthy(`frontend/sample/${sample.profile}-${sample.iteration}/console_errors`, Array.isArray(sample.consoleErrors) && sample.consoleErrors.length === 0, sample.consoleErrors?.length)
    requireValue(`frontend/sample/${sample.profile}-${sample.iteration}/total_rows`, sample.initial?.totalRows, EXPECTED.frontend.podCount)
    requireTruthy(`frontend/sample/${sample.profile}-${sample.iteration}/rendered_rows_bound`, sample.initial?.renderedRows <= EXPECTED.frontend.hardRenderedRowLimit, sample.initial?.renderedRows)
    requireTruthy(`frontend/sample/${sample.profile}-${sample.iteration}/virtual_window_bound`, sample.initial?.virtualWindowSize <= EXPECTED.frontend.hardRenderedRowLimit, sample.initial?.virtualWindowSize)
    requireTruthy(`frontend/sample/${sample.profile}-${sample.iteration}/filter_target`, sample.filter?.matchedRows === 1 && sample.filter?.renderedRows === 1 && sample.filter?.firstRenderedName === 'pod-049999', sample.filter)
  }

  requireTruthy('frontend/baseline/failures_empty', Array.isArray(baseline.failures) && baseline.failures.length === 0, baseline.failures?.length)
  requireTruthy('frontend/baseline/invariant_failures_empty', Array.isArray(baseline.invariantFailures) && baseline.invariantFailures.length === 0, baseline.invariantFailures?.length)
  for (const invariant of REQUIRED_SAMPLE_INVARIANTS) {
    const baselineInvariant = baseline.invariants?.[invariant]
    requireTruthy(`frontend/baseline/invariant/${invariant}`, baselineInvariant?.passed === true && baselineInvariant.passedSamples === sampleList.length && baselineInvariant.totalSamples === sampleList.length, baselineInvariant)
  }
  for (const profile of EXPECTED.frontend.profileNames) {
    const budget = baseline.budget?.profiles?.[profile]
    requireTruthy(`frontend/budget/${profile}`, Boolean(budget), budget ? 'present' : 'missing')
    for (const [metric, report] of Object.entries(budget ?? {})) {
      requireTruthy(`frontend/budget/${profile}/${metric}/samples`, report.hasSamples === true && report.stats?.samples === EXPECTED.frontend.repeatsPerProfile, report.stats?.samples)
      requireTruthy(`frontend/budget/${profile}/${metric}/report_threshold`, typeof report.reportThreshold === 'number' && report.reportThreshold >= report.stats?.p95, report.reportThreshold)
    }
  }
}

function validateCss(stylePath) {
  if (!stylePath) return
  const style = readJson(stylePath)
  requireValue('css/schema', style.schema, EXPECTED.css.schema)
  requireValue('css/version', style.version, EXPECTED.css.version)
  requireValue('css/mode', style.mode, mode)
  requireValue('css/import_order', JSON.stringify(style.importOrder), JSON.stringify(EXPECTED.css.importOrder))
  requireValue('css/removed_layer', style.removedUnreferencedLayer ?? style.removed, EXPECTED.css.removedLayer)
  const layers = Array.isArray(style.layers) ? style.layers : []
  requireValue('css/layer_count', layers.length, EXPECTED.css.importOrder.length)
  requireValue('css/layer_names', JSON.stringify(layers.map((layer) => layer.name)), JSON.stringify(EXPECTED.css.importOrder))
  for (const layer of layers) {
    for (const field of ['bytes', 'lines', 'selectorCount', 'uniqueSelectorCount']) requireTruthy(`css/layer/${layer.name}/${field}`, Number.isInteger(layer[field]) && layer[field] >= 0, layer[field])
    requireTruthy(`css/layer/${layer.name}/sha256`, isSha256(layer.sha256), layer.sha256)
  }
  const sums = layers.reduce((totals, layer) => ({
    bytes: totals.bytes + layer.bytes,
    lines: totals.lines + layer.lines,
    selectorCount: totals.selectorCount + layer.selectorCount,
  }), { bytes: 0, lines: 0, selectorCount: 0 })
  for (const field of ['bytes', 'lines', 'selectorCount']) requireValue(`css/totals/${field}`, style.totals?.[field], sums[field])
  requireTruthy('css/totals/unique_selector_count', Number.isInteger(style.totals?.uniqueSelectorCount) && style.totals.uniqueSelectorCount > 0, style.totals?.uniqueSelectorCount)
}

function summarize() {
  return {
    schemaVersion: 'aiops.m96-gate-b/v1',
    milestone: 'M96',
    gate: 'B',
    mode: mode ?? 'report',
    generatedAt: new Date().toISOString(),
    expected: {
      fixture: EXPECTED.fixture,
      frontend: EXPECTED.frontend,
      css: EXPECTED.css,
    },
    source: {
      root,
      expectedCommit: args['expected-commit'] ?? null,
      evidence: checks.filter((check) => check.name.startsWith('evidence/')),
    },
    checks: checks.filter((check) => !check.name.startsWith('evidence/')),
    warnings,
    errors,
    result: errors.length === 0 ? 'passed' : 'failed',
    performanceThresholds: mode === 'fail-closed' ? 'fail-closed; latency, heap, long-task and CSS regressions block CI' : 'report-only; latency, heap and CSS drift are retained for two stable CI cycles before fail-closed consideration',
  }
}

function markdownReport(report) {
  const status = report.result === 'passed' ? 'PASS' : 'FAIL'
  const lines = [
    '# M96 Gate B scale evidence',
    '',
    `- Result: **${status}**`,
    `- Mode: \`${report.mode}\``,
    `- Generated: ${report.generatedAt}`,
    `- Expected commit: \`${report.source.expectedCommit ?? 'not supplied'}\``,
    '',
    '## Evidence',
    '',
    ...report.source.evidence.map((check) => `- ${check.status === 'passed' ? '[x]' : '[ ]'} \`${check.name.replace('evidence/', '')}\`: ${check.observed}`),
    '',
    '## Checks',
    '',
    ...report.checks.map((check) => `- ${check.status === 'passed' ? '[x]' : '[ ]'} \`${check.name}\`: ${check.observed}`),
    '',
    '## Notes',
    '',
    report.mode === 'fail-closed'
      ? '- Latency, heap, long-task and CSS budget values are treated as production gates (fail-closed); regressions block CI.'
      : '- Latency, heap, long-task and CSS budget values remain report-mode observations.',
    '- Fixture stream revalidation is performed when generated streams are available; hosted CI uploads the verified manifest and reports rather than the generated data streams.',
  ]
  if (report.warnings.length > 0) lines.push('', 'Warnings', '', ...report.warnings.map((warning) => `- ${warning}`))
  if (report.errors.length > 0) lines.push('', 'Errors', '', ...report.errors.map((error) => `- ${error}`))
  return `${lines.join('\n')}\n`
}

async function main() {
  const manifestPath = findRequiredFile('manifest.json', /scale-fixture[\\/]m96-v1/)
  const verificationPath = findRequiredFile('m96-verification.json', /scale-fixture/)
  const generationPath = findRequiredFile('m96-generation.json', /scale-fixture/)
  const backendPath = findRequiredFile('m96-backend-baseline-v1.json', /scale-bench/)
  const frontendBaselinePath = findRequiredFile('m96-pod-scale-baseline-v1.json', /pod-scale-perf/)
  const frontendSamplesPath = findRequiredFile('m96-pod-scale-samples-v1.json', /pod-scale-perf/)
  const stylePath = findRequiredFile('m96-style-baseline-v1.json', /style-audit/)

  try {
    await validateFixture(manifestPath, verificationPath, generationPath, backendPath)
    validateFrontend(frontendBaselinePath, frontendSamplesPath)
    validateCss(stylePath)
  } catch (error) {
    errors.push(error.message)
  }
  const report = summarize()
  mkdirSync(dirname(output), { recursive: true })
  mkdirSync(dirname(markdown), { recursive: true })
  writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`)
  writeFileSync(markdown, markdownReport(report))
  process.stdout.write(`${markdownReport(report)}\n`)
  if (report.result !== 'passed') process.exitCode = 1
}

main().catch((error) => {
  process.stderr.write(`${error.stack ?? error.message}\n`)
  process.exitCode = 1
})
