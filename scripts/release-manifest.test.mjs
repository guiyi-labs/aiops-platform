import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { createManifest, createProvenance, verifyManifest } from './release-manifest.mjs'

const revision = '0123456789abcdef0123456789abcdef01234567'
const ociIndexMediaType = 'application/vnd.oci.image.index.v1+json'
const ociManifestMediaType = 'application/vnd.oci.image.manifest.v1+json'

async function withDirectory(run) {
  const directory = await mkdtemp(join(tmpdir(), 'aiops-release-manifest-'))
  try {
    await run(directory)
  } finally {
    await rm(directory, { recursive: true, force: true })
  }
}

async function writeNestedOCIArchive(archivePath) {
  const layout = `${archivePath}.layout`
  const blobs = join(layout, 'blobs', 'sha256')
  await mkdir(blobs, { recursive: true })
  const writeBlob = async document => {
    const contents = Buffer.from(`${JSON.stringify(document)}\n`)
    const digest = createHash('sha256').update(contents).digest('hex')
    await writeFile(join(blobs, digest), contents)
    return { digest: `sha256:${digest}`, size: contents.length }
  }
  const imageManifest = { schemaVersion: 2, mediaType: ociManifestMediaType, config: {}, layers: [] }
  const amd64 = await writeBlob(imageManifest)
  const arm64 = await writeBlob({ ...imageManifest, annotations: { platform: 'arm64' } })
  const attestation = await writeBlob({ schemaVersion: 2, mediaType: ociManifestMediaType, config: {}, layers: [] })
  const nested = await writeBlob({
    schemaVersion: 2,
    mediaType: ociIndexMediaType,
    manifests: [
      { mediaType: ociManifestMediaType, ...amd64, platform: { os: 'linux', architecture: 'amd64' } },
      { mediaType: ociManifestMediaType, ...arm64, platform: { os: 'linux', architecture: 'arm64' } },
      {
        mediaType: ociManifestMediaType,
        ...attestation,
        platform: { os: 'unknown', architecture: 'unknown' },
        annotations: { 'vnd.docker.reference.type': 'attestation-manifest' },
      },
    ],
  })
  await writeFile(join(layout, 'index.json'), `${JSON.stringify({
    schemaVersion: 2,
    mediaType: ociIndexMediaType,
    manifests: [{ mediaType: ociIndexMediaType, ...nested }],
  })}\n`)
  await writeFile(join(layout, 'oci-layout'), '{"imageLayoutVersion":"1.0.0"}\n')
  const packed = spawnSync('tar', ['-cf', archivePath, '-C', layout, 'index.json', 'oci-layout', 'blobs'], { encoding: 'utf8' })
  assert.equal(packed.status, 0, packed.stderr)
}

test('creates and verifies an RC manifest with provenance', async () => {
  await withDirectory(async directory => {
    await writeFile(join(directory, 'openapi.yaml'), 'openapi: 3.0.3\n')
    await createProvenance({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
    })
    await createManifest({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
      signatureMode: 'keyless',
      strict: false,
    })
    const manifest = await verifyManifest({ directory, strict: false, requireSignatures: false })
    assert.equal(manifest.schema, 'aiops.release-manifest/v1')
    assert.equal(manifest.releaseBoundary.ga, false)
    assert.equal(manifest.assets.length, 2)
    const checksums = await readFile(join(directory, 'SHA256SUMS'), 'utf8')
    assert.match(checksums, /release-manifest\.json/)
    assert.doesNotMatch(checksums, /SHA256SUMS  /)
  })
})

test('rejects checksum tampering', async () => {
  await withDirectory(async directory => {
    await writeFile(join(directory, 'openapi.yaml'), 'openapi: 3.0.3\n')
    await createManifest({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
      signatureMode: 'keyless',
      strict: false,
    })
    await writeFile(join(directory, 'openapi.yaml'), 'tampered\n')
    await assert.rejects(
      verifyManifest({ directory, strict: false, requireSignatures: false }),
      /SHA256 mismatch/,
    )
  })
})

test('rejects non-RC versions', async () => {
  await withDirectory(async directory => {
    await writeFile(join(directory, 'openapi.yaml'), 'openapi: 3.0.3\n')
    await assert.rejects(
      createManifest({
        directory,
        version: 'v0.3.0',
        revision,
        repository: 'guiyi-labs/aiops-platform',
        signatureMode: 'keyless',
        strict: false,
      }),
      /Invalid RC version/,
    )
  })
})

test('defaults keyless identity to the immutable tag ref', async () => {
  await withDirectory(async directory => {
    await writeFile(join(directory, 'openapi.yaml'), 'openapi: 3.0.3\n')
    const { manifest } = await createManifest({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
      signatureMode: 'keyless',
      strict: false,
    })
    assert.equal(
      manifest.verification.signature.certificateIdentity,
      'https://github.com/guiyi-labs/aiops-platform/.github/workflows/release.yml@refs/tags/v0.3.0-rc.1',
    )
  })
})

test('records an explicit identity ref for workflow_dispatch rehearsals', async () => {
  await withDirectory(async directory => {
    await writeFile(join(directory, 'openapi.yaml'), 'openapi: 3.0.3\n')
    const { manifest } = await createManifest({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
      signatureMode: 'keyless',
      identityRef: 'refs/heads/main',
      strict: false,
    })
    assert.equal(
      manifest.verification.signature.certificateIdentity,
      'https://github.com/guiyi-labs/aiops-platform/.github/workflows/release.yml@refs/heads/main',
    )
  })
})

test('reads platforms through a nested OCI index and ignores attestations', async () => {
  await withDirectory(async directory => {
    const archive = join(directory, 'aiops-platform-backend-v0.3.0-rc.1-linux-multiarch-oci.tar')
    await writeNestedOCIArchive(archive)
    const { manifest } = await createManifest({
      directory,
      version: 'v0.3.0-rc.1',
      revision,
      repository: 'guiyi-labs/aiops-platform',
      signatureMode: 'keyless',
      strict: false,
    })
    const image = manifest.assets.find(asset => asset.kind === 'backend_image').image
    assert.deepEqual(image.platforms, ['linux/amd64', 'linux/arm64'])
    assert.equal(image.manifestDigests.length, 2)
  })
})
