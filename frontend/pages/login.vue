<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'

definePageMeta({ layout: 'auth', middleware: 'guest' })

const router = useRouter()
const { login } = useAuth()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({
  username: 'admin',
  password: 'admin123',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    await login(form.username, form.password)
    ElMessage.success('登录成功')
    router.push('/dashboard')
  } catch (err) {
    const message = (err as { data?: { message?: string } })?.data?.message || '登录失败'
    ElMessage.error(message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-card">
    <h2 class="login-title">登录</h2>
    <el-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="handleSubmit">
      <el-form-item prop="username">
        <el-input v-model="form.username" placeholder="用户名" clearable>
          <template #prefix><IconRenderer icon="mdi:account" /></template>
        </el-input>
      </el-form-item>
      <el-form-item prop="password">
        <el-input v-model="form.password" type="password" placeholder="密码" show-password>
          <template #prefix><IconRenderer icon="mdi:cog" /></template>
        </el-input>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" class="login-btn" :loading="loading" @click="handleSubmit">
          登 录
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<style scoped>
.login-card {
  width: min(360px, calc(100vw - 32px));
  padding: 32px;
  border-radius: 8px;
  background-color: var(--el-bg-color);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
  box-sizing: border-box;
}

.login-title {
  margin: 0 0 24px;
  text-align: center;
}

.login-btn {
  width: 100%;
}

@media (max-width: 767px) {
  .login-card {
    padding: 24px 20px;
  }
}
</style>
