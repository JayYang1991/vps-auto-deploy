<template>
  <div class="virtual-code-card">
    <!-- 顶部状态与工具栏 -->
    <div class="virtual-code-header">
      <div class="virtual-code-meta">
        <span class="status-indicator">✓</span>
        <span class="meta-title">转换成功</span>
        <span class="meta-divider">&middot;</span>
        <span class="meta-stat">共 {{ totalLines.toLocaleString() }} 行</span>
        <span class="meta-divider">&middot;</span>
        <span class="meta-stat">{{ sizeKB }} KB</span>
        <span class="virtual-badge" title="采用定高视口滑动窗口，仅渲染可见区域，无论几十万行均保 60 FPS 流畅">
          ⚡ 视口虚拟滚动
        </span>
      </div>

      <div class="virtual-code-actions">
        <!-- 快捷首尾跳转 -->
        <button
          type="button"
          class="btn-tool"
          @click="scrollToLine(1)"
          title="跳转到首行"
        >
          ⬆ 首行
        </button>
        <button
          type="button"
          class="btn-tool"
          @click="scrollToLine(totalLines)"
          title="跳转到底部"
        >
          ⬇ 尾行
        </button>

        <!-- 行号快速跳转输入框 (当行数较多时显示) -->
        <div v-if="totalLines > 100" class="jump-box">
          <input
            v-model.number="jumpLineInput"
            type="number"
            min="1"
            :max="totalLines"
            placeholder="跳转行"
            class="jump-input"
            @keydown.enter="handleJump"
          />
          <button type="button" class="btn-tool" @click="handleJump">跳</button>
        </div>

        <button
          type="button"
          class="btn-action btn-download"
          @click="downloadFile"
          title="直接下载为本地客户端配置文件"
        >
          📥 下载配置 ({{ ext.toUpperCase() }})
        </button>

        <button
          type="button"
          class="btn-action btn-copy"
          @click="copyContent"
          title="复制全量完整配置文本到剪贴板"
        >
          📋 复制全量
        </button>
      </div>
    </div>

    <!-- 虚拟滚动外层视口容器 -->
    <div
      ref="containerRef"
      class="virtual-viewport"
      :style="{ maxHeight: maxHeight || '340px' }"
      @scroll.passive="onScroll"
    >
      <!-- 高度占位骨架：按真实行数撑开原生滚动条滑块 -->
      <div
        class="virtual-phantom"
        :style="{ height: totalHeight + 'px', minWidth: '100%' }"
      >
        <!-- 视口绝对偏移滑动窗口 -->
        <div
          class="virtual-window"
          :style="{ transform: `translateY(${offsetY}px)` }"
        >
          <div
            v-for="item in visibleLines"
            :key="item.lineNum"
            class="code-row"
            :style="{ height: LINE_HEIGHT + 'px' }"
          >
            <span class="code-lineno">{{ item.lineNum }}</span>
            <span class="code-content">{{ item.text }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, shallowRef } from 'vue'
import { useToast, copyToClipboard } from '../composables/useToast'

const props = withDefaults(
  defineProps<{
    content: string
    target?: string
    maxHeight?: string
    lineHeight?: number
  }>(),
  {
    target: 'clash',
    maxHeight: '340px',
    lineHeight: 20,
  }
)

const { toast } = useToast()

const LINE_HEIGHT = props.lineHeight || 20
const BUFFER_LINES = 8 // 上下预渲染缓冲行数

const containerRef = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportHeight = ref(340)
const jumpLineInput = ref<number | ''>('')

// 使用 Uint32Array 存储每一行在 raw string 中的起始字符偏移下标
// 5 万行数据仅占用 ~200KB 内存，且完全避免了在堆中生成 5 万个微小 string 对象
const lineOffsets = shallowRef<Uint32Array>(new Uint32Array([0]))

// 构建行偏移量索引 (单趟极速扫描，耗时 < 3ms)
function rebuildLineIndex(text: string) {
  if (!text) {
    lineOffsets.value = new Uint32Array([0])
    return
  }
  const len = text.length
  const offsets: number[] = [0]
  for (let i = 0; i < len; i++) {
    if (text.charCodeAt(i) === 10) { // '\n'
      offsets.push(i + 1)
    }
  }
  lineOffsets.value = new Uint32Array(offsets)
}

watch(
  () => props.content,
  (newContent) => {
    rebuildLineIndex(newContent)
    scrollTop.value = 0
    if (containerRef.value) {
      containerRef.value.scrollTop = 0
      viewportHeight.value = containerRef.value.clientHeight || 340
    }
  },
  { immediate: true }
)

const totalLines = computed(() => {
  if (!props.content) return 0
  return lineOffsets.value.length
})

const sizeKB = computed(() => {
  if (!props.content) return '0.0'
  return (props.content.length / 1024).toFixed(1)
})

const totalHeight = computed(() => totalLines.value * LINE_HEIGHT)

// 当前可视容纳行数
const visibleCount = computed(() => Math.ceil(viewportHeight.value / LINE_HEIGHT))

// 视口起始与结束行索引
const startIndex = computed(() => {
  const idx = Math.floor(scrollTop.value / LINE_HEIGHT) - BUFFER_LINES
  return Math.max(0, idx)
})

const endIndex = computed(() => {
  const idx = startIndex.value + visibleCount.value + BUFFER_LINES * 2
  return Math.min(totalLines.value, idx)
})

// 滑动窗口的垂直 translateY 偏移
const offsetY = computed(() => startIndex.value * LINE_HEIGHT)

// 动态提取可视区域的切片行数据 (仅对可视区 ~30 行执行 slice，零内存浪费)
const visibleLines = computed(() => {
  const text = props.content
  if (!text || totalLines.value === 0) return []

  const offsets = lineOffsets.value
  const start = startIndex.value
  const end = endIndex.value
  const len = text.length
  const lines: { lineNum: number; text: string }[] = []

  for (let i = start; i < end; i++) {
    const startPos = offsets[i]
    let endPos = (i + 1 < offsets.length) ? offsets[i + 1] - 1 : len
    // 去除末尾的 '\r' 回车符
    if (endPos > startPos && text.charCodeAt(endPos - 1) === 13) {
      endPos--
    }
    lines.push({
      lineNum: i + 1,
      text: text.slice(startPos, endPos),
    })
  }

  return lines
})

// 滚动节流监听
let rafId: number | null = null
function onScroll(e: Event) {
  const target = e.target as HTMLElement
  if (rafId !== null) return
  rafId = requestAnimationFrame(() => {
    scrollTop.value = target.scrollTop
    viewportHeight.value = target.clientHeight || 340
    rafId = null
  })
}

// 快速跳转至指定行号
function scrollToLine(lineNum: number) {
  if (!containerRef.value || totalLines.value === 0) return
  const safeLine = Math.max(1, Math.min(totalLines.value, lineNum))
  const targetScrollTop = (safeLine - 1) * LINE_HEIGHT
  containerRef.value.scrollTo({
    top: targetScrollTop,
    behavior: totalLines.value > 1000 ? 'auto' : 'smooth',
  })
  scrollTop.value = targetScrollTop
}

function handleJump() {
  if (typeof jumpLineInput.value === 'number' && jumpLineInput.value > 0) {
    scrollToLine(jumpLineInput.value)
    jumpLineInput.value = ''
  }
}

// 文件扩展名推断
const ext = computed(() => {
  const extMap: Record<string, string> = {
    clash: 'yaml',
    singbox: 'json',
    surge: 'conf',
    quantumultx: 'conf',
    loon: 'conf',
    shadowrocket: 'txt',
  }
  return extMap[props.target] || 'txt'
})

// 原生 Blob 文件下载
function downloadFile() {
  if (!props.content) {
    toast.warning('暂无预览内容可供下载')
    return
  }
  const blob = new Blob([props.content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `subscription-${props.target}-${new Date().toISOString().slice(0, 10)}.${ext.value}`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  toast.success(`已开始下载 ${a.download}`)
}

// 全量文本复制
function copyContent() {
  if (!props.content) {
    toast.warning('暂无内容可供复制')
    return
  }
  copyToClipboard(props.content, '转换结果完整配置')
}
</script>

<style scoped>
.virtual-code-card {
  background: #0b0f19;
  border: 1px solid #1e293b;
  border-radius: 0.5rem;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.virtual-code-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.85rem;
  background: rgba(15, 23, 42, 0.8);
  border-bottom: 1px solid rgba(51, 65, 85, 0.6);
  flex-wrap: wrap;
  gap: 0.5rem;
}

.virtual-code-meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.78rem;
}

.status-indicator {
  color: #34d399;
  font-weight: 700;
}

.meta-title {
  color: #34d399;
  font-weight: 600;
}

.meta-divider {
  color: #475569;
}

.meta-stat {
  color: #94a3b8;
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.75rem;
}

.virtual-badge {
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.3);
  font-size: 0.68rem;
  padding: 0.1rem 0.4rem;
  border-radius: 0.25rem;
  font-weight: 500;
  cursor: help;
}

.virtual-code-actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.btn-tool {
  background: #1e293b;
  border: 1px solid #334155;
  color: #94a3b8;
  font-size: 0.7rem;
  padding: 0.15rem 0.45rem;
  border-radius: 0.25rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-tool:hover {
  background: #334155;
  color: #f8fafc;
}

.jump-box {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.jump-input {
  width: 55px;
  background: #0f172a;
  border: 1px solid #334155;
  color: #cbd5e1;
  font-size: 0.7rem;
  padding: 0.12rem 0.3rem;
  border-radius: 0.25rem;
  text-align: center;
}
.jump-input:focus {
  outline: none;
  border-color: #38bdf8;
}

.btn-action {
  font-size: 0.725rem;
  padding: 0.18rem 0.55rem;
  border-radius: 0.25rem;
  border: 1px solid transparent;
  cursor: pointer;
  font-weight: 500;
  transition: all 0.15s ease;
}

.btn-download {
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  border-color: rgba(56, 189, 248, 0.3);
}
.btn-download:hover {
  background: rgba(56, 189, 248, 0.22);
}

.btn-copy {
  background: rgba(16, 185, 129, 0.12);
  color: #34d399;
  border-color: rgba(16, 185, 129, 0.3);
}
.btn-copy:hover {
  background: rgba(16, 185, 129, 0.22);
}

/* 视口滚动容器 */
.virtual-viewport {
  overflow-y: auto;
  overflow-x: auto;
  position: relative;
  background: #090d16;
  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  font-size: 0.78rem;
  line-height: 20px;
}

.virtual-phantom {
  position: relative;
  width: 100%;
}

.virtual-window {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  will-change: transform;
}

.code-row {
  display: flex;
  align-items: center;
  white-space: pre;
  min-width: fit-content;
}
.code-row:hover {
  background: rgba(255, 255, 255, 0.04);
}

.code-lineno {
  width: 58px;
  min-width: 58px;
  padding-right: 0.85rem;
  text-align: right;
  color: #475569;
  user-select: none;
  font-size: 0.72rem;
  border-right: 1px solid rgba(51, 65, 85, 0.4);
}

.code-content {
  padding-left: 0.85rem;
  padding-right: 1rem;
  color: #cbd5e1;
}
</style>
