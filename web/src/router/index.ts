import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import Components from '../views/Components.vue'
import Services from '../views/Services.vue'
import Settings from '../views/Settings.vue'
import Login from '../views/Login.vue'
import { getToken } from '../utils/auth'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
  { path: '/components', component: Components },
  { path: '/services', component: Services },
  { path: '/inbounds', redirect: '/services?tab=inbounds' },
  { path: '/tunnels', redirect: '/services?tab=tunnels' },
  { path: '/subscriptions', redirect: '/services?tab=subscriptions' },
  { path: '/settings', component: Settings },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to, from, next) => {
  const token = getToken()
  if (!to.meta.public && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    next()
  }
})

export default router

