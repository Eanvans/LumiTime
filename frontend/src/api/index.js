import axios from 'axios'

const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 请求拦截器
apiClient.interceptors.request.use(
  config => {
    // 可以在这里添加 token 等认证信息
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// 响应拦截器
apiClient.interceptors.response.use(
  response => {
    return response.data
  },
  error => {
    console.error('API Error:', error)
    return Promise.reject(error)
  }
)

export const api = {
  // 获取服务器时间
  getTime() {
    return apiClient.get('/time')
  },

  // 获取主播列表
  getBenchlist() {
    return apiClient.get('/benchlist')
  },

  // 获取主播详细信息
  getNames() {
    return apiClient.get('/names')
  },

  // 搜索Twitch频道
  searchTwitch(query) {
    return apiClient.get('/search/twitch', {
      params: { q: query }
    })
  }
}

export default apiClient
