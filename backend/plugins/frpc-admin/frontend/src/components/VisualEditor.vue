<script setup lang="ts">
import { computed } from 'vue'
import { ElMessageBox } from 'element-plus'
import type { ConfigResponse, Proxy } from '../api'

const props = defineProps<{
  config: ConfigResponse | null
  disabled?: boolean
}>()

const emit = defineEmits<{ (e: 'dirty'): void }>()

const serverForm = computed(() =>
  props.config?.server ?? { server_addr: '', server_port: 0, token: '' },
)

function onServerChange() {
  emit('dirty')
}

const proxyTypes = ['tcp', 'udp', 'http', 'https', 'stcp', 'xtcp', 'sudp']

function addProxy() {
  if (!props.config) return
  props.config.proxies.push({
    name: '',
    type: 'tcp',
    local_ip: '127.0.0.1',
    local_port: 0,
    remote_port: 0,
    custom_domains: [],
  })
  emit('dirty')
}

async function removeProxy(index: number) {
  if (!props.config) return
  const p = props.config.proxies[index]
  try {
    await ElMessageBox.confirm(
      `删除代理「${p.name || '(未命名)'}」？`,
      '确认删除',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  props.config.proxies.splice(index, 1)
  emit('dirty')
}

function onProxyChange() {
  emit('dirty')
}

// 类型相关字段展示
function domainsOf(p: Proxy): string {
  return (p.custom_domains || []).join(', ')
}

function setDomains(p: Proxy, v: string) {
  p.custom_domains = v
    .split(/[,，\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}
</script>

<template>
  <div class="visual">
    <el-card shadow="never" class="server-card">
      <template #header>
        <div class="card-head">
          <span>服务端设置</span>
          <el-text v-if="config?.format" size="small" type="info">
            格式：{{ config.format.toUpperCase() }}
          </el-text>
        </div>
      </template>
      <el-form label-width="110px" size="default" :disabled="disabled">
        <el-row :gutter="12">
          <el-col :span="8">
            <el-form-item label="服务器地址">
              <el-input
                :model-value="serverForm.server_addr"
                placeholder="frps 服务器地址"
                @update:model-value="serverForm.server_addr = $event; onServerChange()"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="端口">
              <el-input-number
                :model-value="serverForm.server_port"
                :min="0"
                :max="65535"
                :controls="false"
                style="width: 100%"
                placeholder="7000"
                @update:model-value="serverForm.server_port = $event ?? 0; onServerChange()"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="token">
              <el-input
                :model-value="serverForm.token"
                type="password"
                show-password
                placeholder="认证 token"
                @update:model-value="serverForm.token = $event; onServerChange()"
              />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
    </el-card>

    <el-card shadow="never" class="proxies-card">
      <template #header>
        <div class="card-head">
          <span>代理列表（{{ config?.proxies?.length ?? 0 }}）</span>
          <el-button size="small" :disabled="disabled" @click="addProxy">添加代理</el-button>
        </div>
      </template>

      <el-table
        v-if="config && config.proxies.length"
        :data="config.proxies"
        size="default"
        class="proxies-table"
      >
        <el-table-column label="名称" min-width="140">
          <template #default="{ row }">
            <el-input
              v-model="row.name"
              size="small"
              placeholder="代理名称"
              :disabled="disabled"
              @input="onProxyChange"
            />
          </template>
        </el-table-column>
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-select
              v-model="row.type"
              size="small"
              style="width: 100%"
              :disabled="disabled"
              @change="onProxyChange"
            >
              <el-option v-for="t in proxyTypes" :key="t" :label="t" :value="t" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="本地 IP" min-width="130">
          <template #default="{ row }">
            <el-input
              v-model="row.local_ip"
              size="small"
              placeholder="127.0.0.1"
              :disabled="disabled"
              @input="onProxyChange"
            />
          </template>
        </el-table-column>
        <el-table-column label="本地端口" width="120">
          <template #default="{ row }">
            <el-input-number
              v-model="row.local_port"
              :min="0"
              :max="65535"
              :controls="false"
              size="small"
              style="width: 100%"
              :disabled="disabled"
              @change="onProxyChange"
            />
          </template>
        </el-table-column>
        <el-table-column label="远端端口" width="120">
          <template #default="{ row }">
            <el-input-number
              v-model="row.remote_port"
              :min="0"
              :max="65535"
              :controls="false"
              size="small"
              style="width: 100%"
              :disabled="disabled"
              @change="onProxyChange"
            />
          </template>
        </el-table-column>
        <el-table-column label="自定义域名" min-width="180">
          <template #default="{ row }">
            <el-input
              :model-value="domainsOf(row)"
              size="small"
              placeholder="逗号分隔多个域名"
              :disabled="disabled"
              @update:model-value="setDomains(row, $event); onProxyChange()"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ $index }">
            <el-button size="small" type="danger" text :disabled="disabled" @click="removeProxy($index)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else-if="!config?.proxies?.length" description="暂无代理，点击「添加代理」开始" />
    </el-card>
  </div>
</template>

<style scoped>
.visual {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
