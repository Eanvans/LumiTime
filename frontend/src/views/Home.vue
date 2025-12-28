<template>
  <div class="home-page">
    <nav class="navbar">
      <div class="navbar-container">
        <router-link to="/" class="brand">订阅我推的主播</router-link>
        <ul class="nav-links">
          <li><router-link to="/" class="active">主播订阅</router-link></li>
          <li><router-link to="/schedule">直播日程</router-link></li>
        </ul>
        <button class="btn btn-primary">登录</button>
      </div>
    </nav>

    <main class="container" style="padding-top: 2.25rem; padding-bottom: 4.5rem;">
      <div class="card" style="max-width: 960px; margin: 0 auto;">
        <div class="text-center mb-3">
          <h2 style="font-size: 1.5rem; margin-bottom: 1rem; color: var(--muted);">
            查找你喜欢的主播
          </h2>
          <form @submit.prevent="handleSearch" class="flex justify-center items-center gap-3">
            <input
              v-model="searchQuery"
              class="input input-rounded"
              type="search"
              placeholder="输入主播名称，例如：某某"
              style="width: 70%;"
              @keydown.enter="handleSearch"
            />
            <button type="button" @click="handleSearch" class="btn btn-primary">
              搜索
            </button>
          </form>
        </div>

        <div class="results-box" aria-live="polite" aria-atomic="true">
          <p v-if="!loading && results.length === 0" class="text-center text-muted">
            搜索结果将显示在这里。
          </p>
          
          <div v-if="loading" class="text-center text-muted">
            正在查询...
          </div>

          <div v-for="(item, index) in results" :key="index" class="result-item">
            <div>
              <div><strong>{{ item.name }}</strong></div>
              <div class="text-muted" style="font-size: 0.875rem;">
                简介：{{ item.description }}
              </div>
            </div>
            <div>
              <button class="btn btn-sm btn-outline">订阅</button>
            </div>
          </div>
        </div>
      </div>
    </main>

    <footer class="footer-fixed">
      Privacy: 本站仅用于演示，不收集个人隐私信息。
    </footer>
  </div>
</template>

<script>
import { ref } from 'vue'

export default {
  name: 'Home',
  setup() {
    const searchQuery = ref('')
    const loading = ref(false)
    const results = ref([])

    const handleSearch = () => {
      const query = searchQuery.value.trim()
      if (!query) {
        return
      }

      loading.value = true
      results.value = []

      // 模拟搜索 - 可以替换为实际的API调用
      setTimeout(() => {
        results.value = Array.from({ length: 40 }, (_, i) => ({
          name: `${query} - 主播 ${i + 1}`,
          description: `这是示例主播 #${i + 1}`
        }))
        loading.value = false
      }, 450)
    }

    return {
      searchQuery,
      loading,
      results,
      handleSearch
    }
  }
}
</script>

<style scoped>
.home-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.results-box {
  margin-top: 1.125rem;
  height: 60vh;
  overflow-y: auto;
  padding: 0.75rem;
  background: #ffffff;
  border-radius: 8px;
  border: 1px solid var(--border);
}

.result-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem;
  border-radius: 8px;
  margin-bottom: 0.5rem;
  border: 1px solid var(--border);
  background: linear-gradient(180deg, #fff, #fbfdff);
  transition: background 0.2s;
}

.result-item:hover {
  background: var(--bg);
}

.footer-fixed {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  background: var(--card);
  padding: 0.625rem 0.75rem;
  text-align: center;
  color: var(--muted-2);
  font-size: 0.9rem;
  border-top: 1px solid var(--border);
}

@media (max-width: 576px) {
  .input-rounded {
    width: 100% !important;
  }
}
</style>
