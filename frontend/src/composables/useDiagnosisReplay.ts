import { computed, ref, watch } from 'vue'
import type { DiagnosisReplayStageID, DiagnosisReplayStep } from '../types/diagnosis'

// useDiagnosisReplay drives the M94 replay mode over a diagnosis insight
// chain. It is a pure state machine over the stored steps: play auto-advances
// on a timer, prev/next/seek move the cursor, and a stage filter narrows the
// visible chain. Replay never calls the backend again — it only walks what was
// already persisted.
export function useDiagnosisReplay(
  getSteps: () => DiagnosisReplayStep[],
  intervalMs = 1500,
) {
  const cursor = ref(-1)
  const playing = ref(false)
  const activeStage = ref<DiagnosisReplayStageID | ''>('')
  let timer: ReturnType<typeof setInterval> | undefined

  const filtered = computed(() => {
    const steps = getSteps()
    if (!activeStage.value) return steps
    return steps.filter((step) => step.stage === activeStage.value)
  })
  const current = computed(() => (cursor.value >= 0 && cursor.value < filtered.value.length ? filtered.value[cursor.value] : null))
  const progress = computed(() => (filtered.value.length === 0 ? 0 : cursor.value + 1))
  const total = computed(() => filtered.value.length)
  const atStart = computed(() => cursor.value <= 0)
  const atEnd = computed(() => filtered.value.length === 0 || cursor.value >= filtered.value.length - 1)

  function clampSeek(index: number): number {
    if (filtered.value.length === 0) return -1
    return Math.max(0, Math.min(filtered.value.length - 1, index))
  }

  function pause() {
    playing.value = false
    if (timer !== undefined) {
      clearInterval(timer)
      timer = undefined
    }
  }

  function play() {
    if (filtered.value.length === 0) return
    if (atEnd.value || cursor.value < 0) cursor.value = 0
    playing.value = true
    if (timer !== undefined) clearInterval(timer)
    timer = setInterval(() => {
      if (atEnd.value) {
        pause()
        return
      }
      cursor.value += 1
    }, intervalMs)
  }

  function toggle() {
    if (playing.value) pause()
    else play()
  }

  function next() {
    pause()
    if (atEnd.value || cursor.value < 0) return
    cursor.value += 1
  }

  function prev() {
    pause()
    if (atStart.value || cursor.value < 0) return
    cursor.value -= 1
  }

  function seek(index: number) {
    pause()
    cursor.value = clampSeek(index)
  }

  function setStage(stage: DiagnosisReplayStageID | '') {
    pause()
    activeStage.value = stage
    cursor.value = stage === '' ? -1 : 0
  }

  function reset() {
    pause()
    cursor.value = -1
    activeStage.value = ''
  }

  watch(filtered, () => {
    if (cursor.value >= filtered.value.length) cursor.value = -1
  })

  return {
    cursor,
    playing,
    activeStage,
    current,
    progress,
    total,
    atStart,
    atEnd,
    play,
    pause,
    toggle,
    next,
    prev,
    seek,
    setStage,
    reset,
  }
}
