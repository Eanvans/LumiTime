<template>
  <div class="schedule-page">
    <nav class="navbar">
      <div class="navbar-container">
        <router-link to="/" class="brand">订阅我推的主播</router-link>
        <ul class="nav-links">
          <li><router-link to="/">主播订阅</router-link></li>
          <li><router-link to="/schedule" class="active">直播日程</router-link></li>
        </ul>
        <button class="btn btn-primary">登录</button>
      </div>
    </nav>

    <main class="schedule-container">
      <div class="schedule-content">
        <h1>Lumi 直播日程</h1>
        
        <div v-if="loading" class="text-center text-muted">
          加载中...
        </div>

        <div v-else class="events-list">
          <a 
            v-for="event in events" 
            :key="event.timestamp"
            :href="event.link"
            target="_blank"
            rel="noopener noreferrer"
            class="event"
          >
            <div class="event-content">
              <div class="event-datetime">
                <div class="event-date">{{ formatDate(event.timestamp) }}</div>
                <div class="event-time">{{ formatTime(event.timestamp) }}</div>
              </div>
              <div class="event-details">
                <div class="event-title">{{ event.title }}</div>
                <div class="event-platform">{{ event.platform }}</div>
              </div>
            </div>
            <div class="event-arrow">
              <svg width="20" height="20" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
              </svg>
            </div>
          </a>
        </div>
      </div>
    </main>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'

export default {
  name: 'Schedule',
  setup() {
    const loading = ref(true)
    const events = ref([])

    // 从本地数据加载（可以替换为API调用）
    const scheduleData = [
      {
        timestamp: 1754323200,
        platform: "Twitch",
        title: "Stream",
        link: "https://www.twitch.tv/kanekolumi"
      },
      {
        timestamp: 1754344800,
        platform: "Twitch",
        title: "Werewolf Night with Filian and Friends",
        link: "https://www.twitch.tv/kanekolumi"
      },
      {
        timestamp: 1754409600,
        platform: "YouTube",
        title: "Stream",
        link: "https://www.youtube.com/@KanekoLumi"
      },
      {
        timestamp: 1754445600,
        platform: "Discord",
        title: "Movie Night",
        link: "https://www.twitch.tv/kanekolumi"
      },
      {
        timestamp: 1754755200,
        platform: "Twitch",
        title: "Strategy Saturday",
        link: "https://www.twitch.tv/kanekolumi"
      },
      {
        timestamp: 1754841600,
        platform: "Twitch",
        title: "Lumir....You're breaking the car!!!",
        link: "https://www.twitch.tv/kanekolumi"
      }
    ]

    const formatDate = (timestamp) => {
      const date = new Date(timestamp * 1000)
      const month = String(date.getMonth() + 1).padStart(2, '0')
      const day = String(date.getDate()).padStart(2, '0')
      return `${month}月${day}日`
    }

    const formatTime = (timestamp) => {
      const date = new Date(timestamp * 1000)
      const hours = String(date.getHours()).padStart(2, '0')
      const minutes = String(date.getMinutes()).padStart(2, '0')
      return `${hours}:${minutes}`
    }

    onMounted(() => {
      // 模拟加载
      setTimeout(() => {
        events.value = scheduleData
        loading.value = false
      }, 300)
    })

    return {
      loading,
      events,
      formatDate,
      formatTime
    }
  }
}
</script>

<style scoped>
.schedule-page {
  min-height: 100vh;
  background: rgba(0, 0, 0, 0.95);
  color: #f1f1f1;
}

.schedule-container {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  min-height: calc(100vh - 72px);
}

.schedule-content {
  max-width: 600px;
  width: 100%;
}

h1 {
  text-align: center;
  margin-bottom: 2rem;
  font-weight: 600;
  font-size: 1.8rem;
  color: #f1f1f1;
}

.events-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.event {
  background: rgba(255, 255, 255, 0.05);
  border-radius: 16px;
  box-shadow: 0 4px 30px rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transition: transform 0.2s ease;
  text-decoration: none;
  color: inherit;
  display: block;
}

.event:hover {
  transform: translateY(-4px);
}

.event-content {
  display: flex;
  padding: 1.25rem;
  gap: 1.25rem;
}

.event-datetime {
  flex-shrink: 0;
  text-align: center;
  min-width: 80px;
}

.event-date {
  font-size: 1.125rem;
  font-weight: 600;
  color: #60a5fa;
}

.event-time {
  font-size: 0.875rem;
  color: rgba(255, 255, 255, 0.7);
  margin-top: 0.25rem;
}

.event-details {
  flex: 1;
}

.event-title {
  font-size: 1.125rem;
  font-weight: 500;
  margin-bottom: 0.375rem;
  color: #f1f1f1;
}

.event-platform {
  font-size: 0.875rem;
  color: rgba(255, 255, 255, 0.6);
  display: inline-block;
  padding: 0.25rem 0.75rem;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 12px;
}

.event-arrow {
  display: flex;
  align-items: center;
  padding-right: 1rem;
  color: rgba(255, 255, 255, 0.4);
}

/* Override navbar styles for dark theme */
.schedule-page .navbar {
  background: rgba(0, 0, 0, 0.8);
  backdrop-filter: blur(10px);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.schedule-page .brand {
  color: #60a5fa;
}

.schedule-page .nav-links a {
  color: rgba(255, 255, 255, 0.8);
}

.schedule-page .nav-links a:hover,
.schedule-page .nav-links a.active {
  color: #60a5fa;
}

@media (max-width: 576px) {
  .event-content {
    gap: 1rem;
  }

  .event-datetime {
    min-width: 70px;
  }

  .event-date {
    font-size: 1rem;
  }

  .event-title {
    font-size: 1rem;
  }
}
</style>
