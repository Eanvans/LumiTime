import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Home from './views/Home.vue'
import Schedule from './views/Schedule.vue'
import './styles/global.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Home },
    { path: '/schedule', component: Schedule }
  ]
})

const app = createApp(App)
app.use(router)
app.mount('#app')
