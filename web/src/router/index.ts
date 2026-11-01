import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import Inbounds from '../views/Inbounds.vue'
import Tunnels from '../views/Tunnels.vue'
import Subscriptions from '../views/Subscriptions.vue'
import Settings from '../views/Settings.vue'
import Login from '../views/Login.vue'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
  { path: '/inbounds', component: Inbounds },
  { path: '/tunnels', component: Tunnels },
  { path: '/subscriptions', component: Subscriptions },
  { path: '/settings', component: Settings },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  if (!to.meta.public && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    next()
  }
})

export default router

