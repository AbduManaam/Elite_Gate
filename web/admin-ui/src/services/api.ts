import axios from 'axios'

const api = axios.create({ baseURL: '/admin/v1' })

// Attach JWT if stored
api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers = config.headers || {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

export const routesApi = {
  getAll: () => api.get('/routes'),
  create: (data: unknown) => api.post('/routes', data),
  update: (id: string, data: unknown) => api.put(`/routes/${id}`, data),
  remove: (id: string) => api.delete(`/routes/${id}`),
}

export const upstreamsApi = {
  getAll: () => api.get('/upstreams'),
  create: (data: unknown) => api.post('/upstreams', data),
  remove: (id: string) => api.delete(`/upstreams/${id}`),
}

export default api