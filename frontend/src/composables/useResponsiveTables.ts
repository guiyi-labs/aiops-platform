import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

/**
 * 移动端表格卡片化。
 *
 * 问题：桌面表格直接缩小到 375px 时，列宽被压到放不下内容——「INC-000006」
 * 断成两行、状态徽章竖排成一条。这不是样式微调能解决的：8 列的表格在
 * 375px 宽下每列只有 47px。
 *
 * 常见做法是给每个 <td> 手写 data-label，再由 CSS 用 ::before 把列名画出来。
 * 但这里有 27 个视图、上百个表格列，手写既容易漏也容易和表头文案分叉。
 *
 * 所以改成运行时注入：读 thead 的列名，写到对应 td 的 data-label 上。
 * 表头是唯一事实来源，改表头文案时卡片标签自动跟着变。
 *
 * 只在 ≤720px 启用（matchMedia 变化时自动挂载/清理），桌面端零开销。
 */

const MOBILE_QUERY = '(max-width: 720px)'

function columnLabels(table: HTMLTableElement): string[] {
  const headRow = table.querySelector('thead tr')
  if (!headRow) return []
  return Array.from(headRow.children).map((cell) => (cell.textContent ?? '').trim())
}

/**
 * 没有文字表头的列（勾选列、操作列）怎么办。
 *
 * 勾选列给「选择」——这一行是明确的可操作项。操作列的 th 常常是空的，
 * 此时**不设标签**：编一个「第 8 列」比空着更糟，它既不是业务字段名，
 * 又会让读者以为那里本来就该有个标题。
 */
function fallbackLabel(cell: HTMLTableCellElement): string | null {
  return cell.querySelector('input[type="checkbox"], input[type="radio"]') ? '选择' : null
}

function labelizeTable(table: HTMLTableElement): void {
  const headers = columnLabels(table)
  if (headers.length === 0) return

  for (const row of Array.from(table.querySelectorAll('tbody tr'))) {
    const cells = Array.from(row.children).filter(
      (cell): cell is HTMLTableCellElement => cell.tagName === 'TD',
    )
    // colspan 的「暂无数据」行、分组行：列数与表头不一致，跳过。
    // 硬按索引贴标签会把「暂无数据」标成一个字段名，比不标更误导。
    if (cells.length !== headers.length) continue

    cells.forEach((cell, index) => {
      // 每次重新写上而不是「已有就跳过」：表头文案改了要能跟上，
      // 且重复调用必须幂等（观察器会反复扫同一张表）。
      const label = headers[index] || fallbackLabel(cell)
      const current = cell.getAttribute('data-label')
      if (label === null) {
        if (current !== null) cell.removeAttribute('data-label')
        return
      }
      if (current !== label) cell.setAttribute('data-label', label)
    })
  }

  // 用属性而不是 :has(td[data-label]) 选择器：:has() 需要 Chrome 105+，
  // 而这个平台还要在旧版 Safari / 国产内核浏览器上跑。
  table.setAttribute('data-responsive', 'card')
}

function scan(root: ParentNode): void {
  for (const table of Array.from(root.querySelectorAll('table'))) {
    labelizeTable(table as HTMLTableElement)
  }
}

export function useResponsiveTables(): { enabled: typeof enabled } {
  const enabled = ref(false)
  let observer: MutationObserver | null = null
  let media: MediaQueryList | null = null

  function start() {
    const content = document.querySelector('.page-content')
    if (!content) {
      // 首次挂载时 .page-content 可能还没渲染出来，下一帧再试。
      void nextTick(() => {
        const late = document.querySelector('.page-content')
        if (late && media?.matches) scan(late)
      })
      return
    }
    scan(content)
    // 表格行是异步插入的：首帧扫到的往往还是空 tbody，数据到达后才长出行。
    // 因此不能只在「新增节点本身是 table」时处理——那时插入的是 <tr>，
    // 会被漏掉，表现为卡片化生效了但一格标签都没有。
    // 改为任何子树变化都重扫，用 rAF 节流到每帧最多一次。
    let frame = 0
    observer = new MutationObserver(() => {
      if (frame) return
      frame = requestAnimationFrame(() => {
        frame = 0
        const host = document.querySelector('.page-content')
        if (host && media?.matches) scan(host)
      })
    })
    // 不监听 attributes：labelize 自己会写 data-label，监听会自我触发。
    observer.observe(content, { childList: true, subtree: true })
  }

  function stop() {
    observer?.disconnect()
    observer = null
  }

  function apply() {
    if (media?.matches) {
      enabled.value = true
      start()
    } else {
      enabled.value = false
      stop()
    }
  }

  onMounted(() => {
    media = window.matchMedia(MOBILE_QUERY)
    apply()
    media.addEventListener('change', apply)
  })

  onBeforeUnmount(() => {
    stop()
    media?.removeEventListener('change', apply)
  })

  return { enabled }
}
