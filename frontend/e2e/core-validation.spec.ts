import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { readFile } from 'node:fs/promises'
import { mockAuthenticatedAPI } from './api-fixtures'

const ruleID = 'pod.image_pull_backoff.v1'
const entry = { id: 7, source_diagnosis_id: 42, rule_id: ruleID, severity: 'high', resource_kind: 'Pod', resource_name: 'web', summary: '镜像拉取失败 ImagePullBackOff', root_causes: ['镜像标签不存在'], recommendations: ['核对镜像标签'], noted_at: '2026-11-18T02:00:00Z' }
const report = {
  tool: 'aiopsbench diagnosis', corpus_version: '1.0', observed_at: '2026-08-23T08:30:00Z', generated_at: '2026-11-18T02:00:00Z', total_scenarios: 2,
  per_rule: [{ rule_id: ruleID, scenarios: 2, true_positives: 1, true_negatives: 1, false_positives: 0, false_negatives: 0, precision: 1, recall: 1, f1: 1 }],
  micro_f1: 1, macro_f1: 1, label_accuracy: 1, pipeline: { total_pod_scenarios: 2, correct_top1: 2, top1_accuracy: 1 },
  outcomes: [
    { id: 'ipb-positive', rule_id: ruleID, kind: 'Pod', note: 'waiting ImagePullBackOff', expected: true, matched: true, passed: true, pipeline_expected: ruleID, pipeline_actual: ruleID, pipeline_passed: true, input: { expected: true, pod: { spec: { image: 'registry.example.com/team/service:missing-release-tag-with-a-long-container-image-reference' }, status: { reason: 'ImagePullBackOff' } } }, record: { summary: entry.summary, root_causes: entry.root_causes, evidence: [{ type: 'container_state', source: 'pod.status.containerStatuses', content: { reason: 'ImagePullBackOff' } }] } },
    { id: 'ipb-healthy', rule_id: ruleID, kind: 'Pod', note: 'healthy running pod', expected: false, matched: false, passed: true, pipeline_expected: '', pipeline_actual: '', pipeline_passed: true, input: { expected: false, pod: { status: { phase: 'Running' } } } },
  ],
}
async function accessible(page: import('@playwright/test').Page) {
  await page.waitForTimeout(700)
  const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
  expect(result.violations.filter(item => ['serious', 'critical'].includes(item.impact ?? '')), JSON.stringify(result.violations)).toEqual([])
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
}

test('rule validation reveals positive evidence, negative labels and downloadable results', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/diagnoses/validation', route => route.fulfill({ json: report }))
  await page.goto('/diagnoses?view=validation')
  await expect(page.locator('.validation-intro')).toContainText('从输入快照验证输出结论')
  await page.getByRole('button', { name: '运行固定场景验证' }).click()
  await expect(page.locator('.validation-metrics')).toContainText('2 / 2')
  await expect(page.locator('.rule-matrix table')).toHaveAttribute('data-responsive', 'scroll')
  await expect(page.locator('.rule-matrix thead')).toBeVisible()
  await page.getByRole('button', { name: /ipb-positive/ }).click()
  await expect(page.locator('.scenario-detail')).toContainText('命中证据 · 1')
  await page.getByText('container_state · pod.status.containerStatuses', { exact: true }).click()
  await expect(page.locator('.scenario-detail pre').first()).toContainText('ImagePullBackOff')
  await page.getByRole('button', { name: /ipb-healthy/ }).click()
  await expect(page.locator('.scenario-detail')).toContainText('目标规则不应命中')
  await expect(page.locator('.scenario-detail')).toContainText('未命中 · 一致')
  await page.getByLabel('场景类型').selectOption('failed')
  await expect(page.locator('.scenario-list')).toContainText('没有符合筛选条件')
  await page.getByLabel('场景类型').selectOption('')
  await page.getByRole('button', { name: /ipb-positive/ }).click()
  await page.getByText('查看原始输入与人工标注', { exact: true }).click()
  const rawInput = page.locator('.scenario-input pre')
  await rawInput.focus()
  await expect(rawInput).toBeFocused()
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await accessible(page)
  }
  const downloaded = page.waitForEvent('download')
  await page.getByRole('button', { name: '导出结果', exact: true }).click()
  const file = await downloaded
  const data = JSON.parse(await readFile((await file.path())!, 'utf8'))
  expect(data.outcomes[1].expected).toBe(false)
  expect(data.outcomes[0].record.evidence[0].content.reason).toBe('ImagePullBackOff')
  await page.route('**/api/v1/diagnoses/validation', route => route.fulfill({ status: 503, json: { message: 'validation unavailable' } }))
  await page.getByRole('button', { name: '重新验证' }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByRole('button', { name: '导出结果', exact: true })).toHaveCount(0)
})

test('case memory compares actual stages for aligned and renamed query and exports its submitted context', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/aiops/knowledge?*', route => route.fulfill({ json: { items: [entry], total: 1 } }))
  await page.route('**/api/v1/aiops/knowledge/inspect?*', route => {
    const query = new URL(route.request().url()).searchParams
    const renamed = query.get('rule_id')?.endsWith('.renamed')
    return route.fulfill({ json: {
      query: { summary_hint: query.get('q'), rule_id: query.get('rule_id'), resource_kind: query.get('resource_kind') },
      structured: renamed ? [] : [entry], vector: [{ ...entry, score: 1 }], fused: [{ ...entry, score: (renamed ? 1 : 2) / 61 }], results: [entry],
      vector_status: 'complete', embedder: 'lexical-hash-v1', dimension: 256, pool_count: 1, pool_truncated: false, structured_truncated: false, shortlist_limit: 10, vector_limit: 10, result_limit: 3, rank_constant: 60,
    } })
  })
  await page.goto('/cases')
  await page.getByRole('button', { name: '查看检索过程' }).click()
  await expect(page).toHaveURL('/cases?view=compare')
  await expect(page.getByLabel('检索对比规则 ID')).toHaveValue(ruleID)
  await page.getByRole('button', { name: '运行检索对比' }).click()
  await expect(page.locator('.retrieval-comparison section').first()).toContainText('第 1 名')
  await page.getByRole('button', { name: '模拟规则更名' }).click()
  await page.getByRole('button', { name: '运行检索对比' }).click()
  await expect(page.locator('.retrieval-comparison section').first()).toContainText('未进入返回结果')
  await expect(page.locator('.retrieval-comparison section').last()).toContainText('第 1 名')
  await expect(page.locator('.retrieval-ranking')).toContainText('1/(60+1)')
  await expect(page.locator('.retrieval-ranking table')).toHaveAttribute('data-responsive', 'scroll')
  await expect(page.locator('.retrieval-ranking thead')).toBeVisible()
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await accessible(page)
  }
  const downloaded = page.waitForEvent('download')
  await page.getByRole('button', { name: '导出对比', exact: true }).click()
  const file = await downloaded
  const data = JSON.parse(await readFile((await file.path())!, 'utf8'))
  expect(data.reference_id).toBe(7)
  expect(data.trace.query.rule_id).toBe(ruleID + '.renamed')
  expect(data.trace.structured).toEqual([])
  await page.route('**/api/v1/aiops/knowledge/inspect?*', route => route.fulfill({ status: 503, json: { message: 'retrieval unavailable' } }))
  await page.getByRole('button', { name: '运行检索对比' }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByRole('button', { name: '导出对比', exact: true })).toHaveCount(0)
  await expect(page.locator('.navigation .nav-row')).toHaveCount(5)
})
