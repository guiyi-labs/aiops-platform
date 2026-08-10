import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { createManifest, createProvenance, verifyManifest } from './release-manifest.mjs'

const revision = '0123456789abcdef0123456789abcdef01234567'

async function withDirectory(run) {
  const directory = await mkdtemp(join(tmpdir(), 'aiops-release-manifest-'))
  try {
    await run(directory)
  } finally {
    await rm(directory, { recursive: true, force: true })
  }
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
