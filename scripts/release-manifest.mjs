import { createHash } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { open, readFile, readdir, stat, writeFile } from 'node:fs/promises'
import { basename, join, relative, sep } from 'node:path'
import { pathToFileURL } from 'node:url'
import { spawnSync } from 'node:child_process'

const MANIFEST_SCHEMA = 'aiops.release-manifest/v1'
const PROVENANCE_TYPE = 'https://slsa.dev/provenance/v1'
const OCI_INDEX_MEDIA_TYPE = 'application/vnd.oci.image.index.v1+json'
const OCI_MANIFEST_MEDIA_TYPE = 'application/vnd.oci.image.manifest.v1+json'
const RC_VERSION = /^v\d+\.\d+\.\d+-rc\.\d+$/
const REVISION = /^[0-9a-f]{40}$/
const GENERATED_FILES = new Set([
  'release-manifest.json',
  'SHA256SUMS',
  'SHA256SUMS.sig',
  'SHA256SUMS.cert.pem',
  'SHA256SUMS.bundle',
  'SIGNING_SKIPPED',
])

function parseArgs(values) {
  const result = { _: [] }
  for (let index = 0; index < values.length; index += 1) {
    const value = values[index]
    if (!value.startsWith('--')) {
      result._.push(value)
      continue
    }
    const key = value.slice(2)
    const next = values[index + 1]
    if (!next || next.startsWith('--')) {
      result[key] = true
      continue
    }
    result[key] = next
    index += 1
  }
  return result
}

function requireOption(args, name) {
  const value = args[name]
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`Missing required option --${name}`)
  }
  return value
}

function normalizeRelativePath(value) {
  return value.split(sep).join('/')
}

async function sha256File(filePath) {
  const hash = createHash('sha256')
  await new Promise((resolve, reject) => {
    const stream = createReadStream(filePath)
    stream.on('data', chunk => hash.update(chunk))
    stream.on('error', reject)
    stream.on('end', resolve)
  })
  return hash.digest('hex')
}

async function listFiles(directory) {
  const files = []
  async function visit(current) {
    const entries = await readdir(current, { withFileTypes: true })
    for (const entry of entries) {
      const fullPath = join(current, entry.name)
      if (entry.isDirectory()) {
        await visit(fullPath)
      } else if (entry.isFile()) {
        files.push(normalizeRelativePath(relative(directory, fullPath)))
      }
    }
  }
  await visit(directory)
  return files.sort((left, right) => left.localeCompare(right))
}

function classifyAsset(assetPath) {
  const name = basename(assetPath)
  if (/backend-.+-oci\.tar$/.test(name)) return 'backend_image'
  if (/frontend-.+-oci\.tar$/.test(name)) return 'frontend_image'
  if (/sbom-backend-.+\.json$/.test(name)) return 'backend_sbom'
  if (/sbom-frontend-.+\.json$/.test(name)) return 'frontend_sbom'
  if (/provenance\.in-toto\.json$/.test(name)) return 'provenance'
  if (/kustomize-.+\.tar\.gz$/.test(name)) return 'kustomize'
  if (/offline-.+\.tar\.gz$/.test(name)) return 'offline_bundle'
  if (/source-.+\.tar\.gz$/.test(name)) return 'source'
  if (/aiops-platform-.+\.tgz$/.test(name)) return 'helm_chart'
  if (name === 'openapi.yaml') return 'openapi'
  if (name === 'dependency-licenses.md') return 'dependency_licenses'
  if (name === 'license-allowlist.json') return 'license_allowlist'
  if (name === 'release-candidate-operations.md') return 'operations_runbook'
  if (name === 'SHA256SUMS.pub') return 'local_signature_public_key'
  return 'supporting'
}

function mediaTypeFor(assetPath) {
  if (assetPath.endsWith('.tar.gz') || assetPath.endsWith('.tgz')) return 'application/gzip'
  if (assetPath.endsWith('.tar')) return 'application/x-tar'
  if (assetPath.endsWith('.json')) return 'application/json'
  if (assetPath.endsWith('.yaml') || assetPath.endsWith('.yml')) return 'application/yaml'
  if (assetPath.endsWith('.md')) return 'text/markdown'
  return 'application/octet-stream'
}

function parseTarString(buffer, start, length) {
  return buffer.subarray(start, start + length).toString('utf8').replace(/\0.*$/, '').trim()
}

function parseTarSize(buffer) {
  const value = parseTarString(buffer, 124, 12)
  return value ? Number.parseInt(value, 8) : 0
}

async function readTarEntry(archivePath, targetName) {
  const handle = await open(archivePath, 'r')
  try {
    const header = Buffer.alloc(512)
    let offset = 0
    while (true) {
      const { bytesRead } = await handle.read(header, 0, 512, offset)
      if (bytesRead === 0) return null
      if (bytesRead !== 512) throw new Error(`Truncated tar header in ${archivePath}`)
      if (header.every(byte => byte === 0)) return null
      const name = parseTarString(header, 0, 100)
      const prefix = parseTarString(header, 345, 155)
      const entryName = prefix ? `${prefix}/${name}` : name
      const size = parseTarSize(header)
      const dataOffset = offset + 512
      if (entryName === targetName) {
        const contents = Buffer.alloc(size)
        const result = await handle.read(contents, 0, size, dataOffset)
        if (result.bytesRead !== size) throw new Error(`Truncated tar entry ${targetName}`)
        return contents
      }
      offset = dataOffset + Math.ceil(size / 512) * 512
    }
  } finally {
    await handle.close()
  }
}

async function inspectOCIArchive(archivePath) {
  const indexBytes = await readTarEntry(archivePath, 'index.json')
  if (!indexBytes) throw new Error(`OCI archive ${archivePath} has no index.json`)
  const index = JSON.parse(indexBytes.toString('utf8'))
  if (index.schemaVersion !== 2 || !Array.isArray(index.manifests) || index.manifests.length === 0) {
    throw new Error(`OCI archive ${archivePath} must contain an OCI image index`)
  }

  const manifestDigests = []
  const platforms = []
  const visited = new Set()
  async function walkDescriptor(descriptor) {
    if (!/^sha256:[0-9a-f]{64}$/.test(descriptor.digest ?? '')) {
      throw new Error(`OCI archive ${archivePath} has an invalid manifest digest`)
    }
    if (visited.has(descriptor.digest)) return
    visited.add(descriptor.digest)

    const blobPath = `blobs/sha256/${descriptor.digest.slice('sha256:'.length)}`
    const blobBytes = await readTarEntry(archivePath, blobPath)
    if (!blobBytes) throw new Error(`OCI archive ${archivePath} is missing ${blobPath}`)
    const actualDigest = `sha256:${createHash('sha256').update(blobBytes).digest('hex')}`
    if (actualDigest !== descriptor.digest) {
      throw new Error(`OCI archive ${archivePath} has a digest mismatch for ${blobPath}`)
    }

    const document = JSON.parse(blobBytes.toString('utf8'))
    if (descriptor.mediaType === OCI_INDEX_MEDIA_TYPE) {
      if (document.schemaVersion !== 2 || !Array.isArray(document.manifests) || document.manifests.length === 0) {
        throw new Error(`OCI archive ${archivePath} contains an invalid nested image index`)
      }
      for (const child of document.manifests) await walkDescriptor(child)
      return
    }
    if (descriptor.mediaType !== OCI_MANIFEST_MEDIA_TYPE || document.schemaVersion !== 2) {
      throw new Error(`OCI archive ${archivePath} contains unsupported media type ${descriptor.mediaType}`)
    }
    if (descriptor.annotations?.['vnd.docker.reference.type'] === 'attestation-manifest') return

    manifestDigests.push(descriptor.digest)
    const platform = descriptor.platform
    if (platform?.os && platform?.architecture && platform.os !== 'unknown' && platform.architecture !== 'unknown') {
      platforms.push(`${platform.os}/${platform.architecture}${platform.variant ? `/${platform.variant}` : ''}`)
    }
  }
  for (const descriptor of index.manifests) await walkDescriptor(descriptor)
  if (manifestDigests.length === 0) throw new Error(`OCI archive ${archivePath} contains no image manifests`)

  return {
    indexDigest: `sha256:${createHash('sha256').update(indexBytes).digest('hex')}`,
    manifestDigests: manifestDigests.sort(),
    platforms: [...new Set(platforms)].sort(),
  }
}

function validateIdentity({ version, revision }) {
  if (!RC_VERSION.test(version)) {
    throw new Error(`Invalid RC version ${version}; expected vX.Y.Z-rc.N`)
  }
  if (!REVISION.test(revision)) {
    throw new Error(`Invalid Git revision ${revision}; expected a full 40-character SHA`)
  }
}

async function collectAssets(directory) {
  const paths = (await listFiles(directory)).filter(assetPath => !GENERATED_FILES.has(assetPath))
  const assets = []
  for (const assetPath of paths) {
    const fullPath = join(directory, ...assetPath.split('/'))
    const fileStat = await stat(fullPath)
    const kind = classifyAsset(assetPath)
    const asset = {
      path: assetPath,
      kind,
      mediaType: mediaTypeFor(assetPath),
      size: fileStat.size,
      sha256: await sha256File(fullPath),
    }
    if (kind === 'backend_image' || kind === 'frontend_image') {
      const image = await inspectOCIArchive(fullPath)
      asset.image = {
        name: kind === 'backend_image' ? 'k8s-aiops-backend' : 'k8s-aiops-frontend',
        format: 'oci-archive',
        indexDigest: image.indexDigest,
        manifestDigests: image.manifestDigests,
        platforms: image.platforms,
      }
    }
    assets.push(asset)
  }
  return assets
}

function assertStrictAssets(assets) {
  const requiredKinds = [
    'backend_image',
    'frontend_image',
    'backend_sbom',
    'frontend_sbom',
    'provenance',
    'kustomize',
    'offline_bundle',
    'source',
    'helm_chart',
    'openapi',
    'dependency_licenses',
    'license_allowlist',
    'operations_runbook',
  ]
  const kinds = new Set(assets.map(asset => asset.kind))
  const missing = requiredKinds.filter(kind => !kinds.has(kind))
  if (missing.length > 0) throw new Error(`Release assets are incomplete: ${missing.join(', ')}`)
  for (const kind of ['backend_image', 'frontend_image']) {
    const asset = assets.find(item => item.kind === kind)
    for (const platform of ['linux/amd64', 'linux/arm64']) {
      if (!asset.image.platforms.includes(platform)) {
        throw new Error(`${kind} is missing platform ${platform}`)
      }
    }
  }
}

function findAsset(assets, kind) {
  return assets.find(asset => asset.kind === kind)?.path ?? null
}

function signatureConfiguration({ signatureMode, repository, version }) {
  if (signatureMode === 'key') {
    return {
      mode: 'key',
      required: true,
      signature: 'SHA256SUMS.sig',
      publicKey: 'SHA256SUMS.pub',
    }
  }
  if (signatureMode !== 'keyless') throw new Error(`Unsupported signature mode ${signatureMode}`)
  return {
    mode: 'keyless',
    required: true,
    signature: 'SHA256SUMS.sig',
    certificate: 'SHA256SUMS.cert.pem',
    bundle: 'SHA256SUMS.bundle',
    oidcIssuer: 'https://token.actions.githubusercontent.com',
    certificateIdentity: `https://github.com/${repository}/.github/workflows/release.yml@refs/tags/${version}`,
  }
}

async function writeChecksums(directory, assets) {
  const manifestPath = join(directory, 'release-manifest.json')
  const manifestStat = await stat(manifestPath)
  const entries = [
    ...assets.map(asset => ({ path: asset.path, size: asset.size, sha256: asset.sha256 })),
    {
      path: 'release-manifest.json',
      size: manifestStat.size,
      sha256: await sha256File(manifestPath),
    },
  ].sort((left, right) => left.path.localeCompare(right.path))
  const contents = `${entries.map(entry => `${entry.sha256}  ${entry.path}`).join('\n')}\n`
  await writeFile(join(directory, 'SHA256SUMS'), contents, 'utf8')
  return entries
}

export async function createProvenance(options) {
  validateIdentity(options)
  const assets = (await collectAssets(options.directory)).filter(asset => asset.kind !== 'provenance')
  const fileName = `aiops-platform-${options.version}-provenance.in-toto.json`
  const builderId = options.builderId || `https://github.com/${options.repository}/blob/${options.revision}/scripts/release-verify.ps1`
  const now = new Date().toISOString()
  const provenance = {
    _type: 'https://in-toto.io/Statement/v1',
    subject: assets.map(asset => ({ name: asset.path, digest: { sha256: asset.sha256 } })),
    predicateType: PROVENANCE_TYPE,
    predicate: {
      buildDefinition: {
        buildType: `https://github.com/${options.repository}/.github/workflows/release.yml@v1`,
        externalParameters: {
          version: options.version,
          revision: options.revision,
          source: `git+https://github.com/${options.repository}.git@${options.revision}`,
        },
        internalParameters: { channel: 'rc' },
        resolvedDependencies: [
          {
            uri: `git+https://github.com/${options.repository}.git@${options.revision}`,
            digest: { gitCommit: options.revision },
          },
        ],
      },
      runDetails: {
        builder: { id: builderId },
        metadata: {
          invocationId: options.invocationId || `local:${options.revision}`,
          startedOn: now,
          finishedOn: now,
        },
      },
    },
  }
  await writeFile(join(options.directory, fileName), `${JSON.stringify(provenance, null, 2)}\n`, 'utf8')
  return { fileName, provenance }
}

export async function createManifest(options) {
  validateIdentity(options)
  const assets = await collectAssets(options.directory)
  if (options.strict) assertStrictAssets(assets)
  const signature = signatureConfiguration(options)
  const manifest = {
    schema: MANIFEST_SCHEMA,
    status: 'release_candidate',
    channel: 'rc',
    version: options.version,
    tag: options.version,
    revision: options.revision,
    repository: options.repository,
    generatedAt: new Date().toISOString(),
    releaseBoundary: {
      ga: false,
      productionReady: false,
      blockers: [
        'M89 production OIDC/MFA integration is not verified',
        'M90 WAL/PITR/HA recovery is not verified',
      ],
    },
    assets,
    images: assets.filter(asset => asset.image).map(asset => ({ asset: asset.path, ...asset.image })),
    installation: {
      helm: { asset: findAsset(assets, 'helm_chart'), runbook: findAsset(assets, 'operations_runbook') },
      kustomize: { asset: findAsset(assets, 'kustomize'), runbook: findAsset(assets, 'operations_runbook') },
      offline: { asset: findAsset(assets, 'offline_bundle'), runbook: findAsset(assets, 'operations_runbook') },
    },
    verification: {
      checksumManifest: 'SHA256SUMS',
      signature,
      commands: [
        'sha256sum -c SHA256SUMS',
        signature.mode === 'keyless'
          ? `cosign verify-blob --bundle ${signature.bundle} --certificate-identity ${signature.certificateIdentity} --certificate-oidc-issuer ${signature.oidcIssuer} SHA256SUMS`
          : `cosign verify-blob --key ${signature.publicKey} --signature ${signature.signature} SHA256SUMS`,
      ],
    },
  }
  await writeFile(join(options.directory, 'release-manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, 'utf8')
  const checksums = await writeChecksums(options.directory, assets)
  return { manifest, checksums }
}

function parseChecksums(contents) {
  const entries = new Map()
  for (const line of contents.split(/\r?\n/)) {
    if (!line) continue
    const match = line.match(/^([0-9a-f]{64})  ([^\r\n]+)$/)
    if (!match) throw new Error(`Invalid SHA256SUMS line: ${line}`)
    if (entries.has(match[2])) throw new Error(`Duplicate SHA256SUMS entry: ${match[2]}`)
    entries.set(match[2], match[1])
  }
  return entries
}

async function verifyProvenance(directory, manifest) {
  const provenanceAsset = manifest.assets.find(asset => asset.kind === 'provenance')
  if (!provenanceAsset) return
  const provenance = JSON.parse(await readFile(join(directory, ...provenanceAsset.path.split('/')), 'utf8'))
  if (provenance._type !== 'https://in-toto.io/Statement/v1' || provenance.predicateType !== PROVENANCE_TYPE) {
    throw new Error('Provenance statement has an unsupported type')
  }
  const subjects = new Map(provenance.subject.map(subject => [subject.name, subject.digest?.sha256]))
  for (const asset of manifest.assets) {
    if (asset.kind === 'provenance') continue
    if (subjects.get(asset.path) !== asset.sha256) {
      throw new Error(`Provenance subject mismatch for ${asset.path}`)
    }
  }
}

function runCosign(directory, signature) {
  let args
  if (signature.mode === 'keyless') {
    args = [
      'verify-blob',
      '--bundle', signature.bundle,
      '--certificate-identity', signature.certificateIdentity,
      '--certificate-oidc-issuer', signature.oidcIssuer,
      'SHA256SUMS',
    ]
  } else {
    args = ['verify-blob', '--key', signature.publicKey, '--signature', signature.signature, 'SHA256SUMS']
  }
  const result = spawnSync('cosign', args, { cwd: directory, encoding: 'utf8', shell: false })
  if (result.error) throw new Error(`Unable to execute cosign: ${result.error.message}`)
  if (result.status !== 0) throw new Error(`Cosign verification failed: ${(result.stderr || result.stdout).trim()}`)
}

export async function verifyManifest(options) {
  const manifest = JSON.parse(await readFile(join(options.directory, 'release-manifest.json'), 'utf8'))
  if (manifest.schema !== MANIFEST_SCHEMA) throw new Error(`Unsupported manifest schema ${manifest.schema}`)
  validateIdentity(manifest)
  if (manifest.status !== 'release_candidate' || manifest.channel !== 'rc' || manifest.tag !== manifest.version) {
    throw new Error('Release manifest must remain an RC and tag must equal version')
  }
  if (manifest.releaseBoundary?.ga !== false || manifest.releaseBoundary?.productionReady !== false) {
    throw new Error('Release manifest cannot claim GA or production-ready status')
  }
  if (!Array.isArray(manifest.assets)) throw new Error('Release manifest assets must be an array')
  if (options.strict) assertStrictAssets(manifest.assets)
  const checksums = parseChecksums(await readFile(join(options.directory, 'SHA256SUMS'), 'utf8'))
  const expectedPaths = new Set([...manifest.assets.map(asset => asset.path), 'release-manifest.json'])
  if (checksums.size !== expectedPaths.size) throw new Error('SHA256SUMS does not exactly cover the manifest payload')
  for (const assetPath of expectedPaths) {
    if (assetPath.startsWith('/') || assetPath.includes('..')) throw new Error(`Unsafe asset path ${assetPath}`)
    const expectedHash = checksums.get(assetPath)
    if (!expectedHash) throw new Error(`SHA256SUMS is missing ${assetPath}`)
    const fullPath = join(options.directory, ...assetPath.split('/'))
    const actualHash = await sha256File(fullPath)
    if (actualHash !== expectedHash) throw new Error(`SHA256 mismatch for ${assetPath}`)
    const asset = manifest.assets.find(item => item.path === assetPath)
    if (asset) {
      const fileStat = await stat(fullPath)
      if (asset.sha256 !== actualHash || asset.size !== fileStat.size) {
        throw new Error(`Manifest metadata mismatch for ${assetPath}`)
      }
      if (asset.image) {
        const image = await inspectOCIArchive(fullPath)
        if (
          image.indexDigest !== asset.image.indexDigest ||
          JSON.stringify(image.manifestDigests) !== JSON.stringify(asset.image.manifestDigests) ||
          JSON.stringify(image.platforms) !== JSON.stringify(asset.image.platforms)
        ) {
          throw new Error(`OCI index metadata mismatch for ${assetPath}`)
        }
      }
    }
  }
  await verifyProvenance(options.directory, manifest)
  if (options.requireSignatures) {
    const signature = manifest.verification?.signature
    if (!signature?.required) throw new Error('Release manifest does not require a signature')
    const requiredFiles = signature.mode === 'keyless'
      ? [signature.signature, signature.certificate, signature.bundle]
      : [signature.signature, signature.publicKey]
    for (const fileName of requiredFiles) {
      if (!fileName) throw new Error(`Signature metadata is incomplete for mode ${signature.mode}`)
      await stat(join(options.directory, fileName))
    }
    runCosign(options.directory, signature)
  }
  return manifest
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  const command = args._[0]
  const directory = requireOption(args, 'directory')
  if (command === 'provenance') {
    const result = await createProvenance({
      directory,
      version: requireOption(args, 'version'),
      revision: requireOption(args, 'revision'),
      repository: requireOption(args, 'repository'),
      builderId: args['builder-id'],
      invocationId: args['invocation-id'],
    })
    process.stdout.write(`Created ${result.fileName}\n`)
    return
  }
  if (command === 'create') {
    const result = await createManifest({
      directory,
      version: requireOption(args, 'version'),
      revision: requireOption(args, 'revision'),
      repository: requireOption(args, 'repository'),
      signatureMode: args['signature-mode'] || 'keyless',
      strict: Boolean(args.strict),
    })
    process.stdout.write(`Created release-manifest.json and SHA256SUMS for ${result.checksums.length} files\n`)
    return
  }
  if (command === 'verify') {
    const manifest = await verifyManifest({
      directory,
      strict: Boolean(args.strict),
      requireSignatures: Boolean(args['require-signatures']),
    })
    process.stdout.write(`Verified ${manifest.version} release manifest (${manifest.assets.length} assets)\n`)
    return
  }
  throw new Error('Usage: release-manifest.mjs <provenance|create|verify> --directory <path> [options]')
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  main().catch(error => {
    process.stderr.write(`${error.message}\n`)
    process.exitCode = 1
  })
}
