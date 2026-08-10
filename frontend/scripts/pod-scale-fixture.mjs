/* global process */

import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const defaultConfigPath = join(process.cwd(), 'testdata', 'scale', 'm96-pods-v1.json')

export function loadPodScaleConfig(configPath = defaultConfigPath) {
  const source = readFileSync(configPath, 'utf8')
  const config = JSON.parse(source)
  return {
    config,
    configPath,
    configSha256: sha256(source),
  }
}

export function buildPodFixture(config) {
  const items = Array.from({ length: config.pod_count }, (_, index) => {
    const id = String(index).padStart(6, '0')
    const namespaceIndex = (index + config.seed) % config.namespace_count
    const nodeIndex = (index * 17 + config.seed) % config.node_count
    const restartCount = (index + config.seed) % 5
    const phase = index % 29 === 0 ? 'Pending' : 'Running'
    return {
      metadata: {
        name: `pod-${id}`,
        namespace: `namespace-${String(namespaceIndex).padStart(3, '0')}`,
        uid: `m96-pod-${id}`,
        creationTimestamp: '2026-08-10T00:00:00Z',
      },
      spec: {
        nodeName: `node-${String(nodeIndex).padStart(3, '0')}`,
        containers: [{ name: 'app', image: `registry.example.invalid/aiops/workload:${index % 20}` }],
      },
      status: {
        phase,
        reason: phase === 'Pending' ? 'Pending' : '',
        containerStatuses: [{
          name: 'app',
          ready: phase === 'Running',
          restartCount,
          state: phase === 'Pending' ? { waiting: { reason: 'Pending' } } : {},
          lastState: {},
        }],
      },
    }
  })
  const body = JSON.stringify({ items, total: items.length, remaining: 0 })
  return { body, bytes: Buffer.byteLength(body), sha256: sha256(body) }
}

export function buildNamespaceFixture(config) {
  const items = Array.from({ length: config.namespace_count }, (_, index) => ({
    metadata: { name: `namespace-${String(index).padStart(3, '0')}` },
  }))
  return JSON.stringify({ items, total: items.length, remaining: 0 })
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}
