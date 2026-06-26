import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import ContentList from './pages/ContentList'
import ContentEdit from './pages/ContentEdit'
import ContentNew from './pages/ContentNew'
import MediaPage from './pages/MediaPage'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/admin/" element={<Dashboard />} />
        <Route path="/admin/content" element={<ContentList />} />
        <Route path="/admin/content/new" element={<ContentNew />} />
        <Route path="/admin/media" element={<MediaPage />} />
      </Route>
      {/* Full-screen editor outside layout — no sidebar, no chrome */}
      <Route path="/admin/content/edit" element={<ContentEdit />} />
    </Routes>
  )
}
