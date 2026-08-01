import { NavLink, Route, Routes } from 'react-router-dom'
import { isMockMode } from './api/client.js'
import AlertsPage from './pages/AlertsPage.jsx'
import InvestigationPage from './pages/InvestigationPage.jsx'

export default function App() {
  return (
    <div className="app-shell">
      <header className="app-header">
        <NavLink to="/" className="brand">
          <span className="brand-name">AdInsight</span>
          <span className="brand-tag">Automated root-cause analyst</span>
        </NavLink>
        <div className="header-meta">
          <span>{isMockMode() ? 'mock data' : 'live api'}</span>
          <span>·</span>
          <span>Click-a-thon 2026</span>
        </div>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<AlertsPage />} />
          <Route path="/investigations/:investigationId" element={<InvestigationPage />} />
        </Routes>
      </main>
    </div>
  )
}
