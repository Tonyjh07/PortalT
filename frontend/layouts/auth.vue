<template>
  <div class="auth-layout">
    <div class="auth-brand">
      <IconRenderer icon="mdi:server" :size="40" />
      <h1>PortalT</h1>
      <p>HomeLab 统一门户</p>
    </div>
    <slot />
  </div>
</template>

<style scoped>
.auth-layout {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 8px;
  padding: 24px 16px;
  box-sizing: border-box;
}

.auth-brand {
  text-align: center;
  margin-bottom: 16px;
}

.auth-brand h1 {
  margin: 8px 0 4px;
  font-size: 28px;
}

.auth-brand p {
  margin: 0;
  color: #909399;
}
</style>
