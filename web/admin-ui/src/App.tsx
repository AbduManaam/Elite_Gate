import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import RoutesPage from './pages/Routes'
import Upstreams from './pages/Upstreams'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/routes" element={<RoutesPage />} />
        <Route path="/upstreams" element={<Upstreams />} />
      </Routes>
    </BrowserRouter>
  )
}