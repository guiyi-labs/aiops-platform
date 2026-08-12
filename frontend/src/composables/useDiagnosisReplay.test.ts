import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useDiagnosisReplay } from './useDiagnosisReplay'
import type { DiagnosisReplayStep } from '../types/diagnosis'

function steps(): DiagnosisReplayStep[] {
  return [
    { index: 0, stage: 'diagnosis_created', type: 'diagnosis_created', summary: '创建', ref: 'diagnosis:1', missing: false },
    { index: 1, stage: 'evidence', type: 'node_condition', summary: 'Ready = False', ref: 'diagnosis:1:evidence:0', missing: false },
    { index: 2, stage: 'ai_explanation', type: 'ai_explanation', summary: '解释', ref: 'explanation:1', missing: false },
  ]
}

describe('useDiagnosisReplay', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('walks steps with next/prev/seek', () => {
    const replay = useDiagnosisReplay(() => steps())
    expect(replay.total.value).toBe(3)
    expect(replay.current.value).toBeNull()
    replay.seek(1)
    expect(replay.current.value?.summary).toBe('Ready = False')
    expect(replay.progress.value).toBe(2)
    replay.next()
    expect(replay.current.value?.stage).toBe('ai_explanation')
    replay.prev()
    expect(replay.current.value?.summary).toBe('Ready = False')
    replay.seek(99)
    expect(replay.current.value?.stage).toBe('ai_explanation')
  })

  it('auto-advances while playing and stops at the end', async () => {
    const replay = useDiagnosisReplay(() => steps(), 100)
    replay.play()
    expect(replay.playing.value).toBe(true)
    expect(replay.cursor.value).toBe(0)
    vi.advanceTimersByTime(250)
    await nextTick()
    expect(replay.cursor.value).toBe(2)
    vi.advanceTimersByTime(150)
    await nextTick()
    expect(replay.playing.value).toBe(false)
    expect(replay.cursor.value).toBe(2)
    replay.next()
    expect(replay.cursor.value).toBe(2)
  })

  it('filters by stage and resets the cursor', () => {
    const replay = useDiagnosisReplay(() => steps())
    replay.setStage('evidence')
    expect(replay.total.value).toBe(1)
    expect(replay.current.value?.summary).toBe('Ready = False')
    replay.setStage('')
    expect(replay.total.value).toBe(3)
    expect(replay.current.value).toBeNull()
    replay.seek(1)
    replay.setStage('activity')
    expect(replay.total.value).toBe(0)
    expect(replay.current.value).toBeNull()
    replay.reset()
    expect(replay.total.value).toBe(3)
  })

  it('toggle pauses an active playback', () => {
    const replay = useDiagnosisReplay(() => steps(), 100)
    replay.toggle()
    expect(replay.playing.value).toBe(true)
    replay.toggle()
    expect(replay.playing.value).toBe(false)
  })
})
