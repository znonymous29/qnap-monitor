import { NavLink, Route, Routes } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Storage from './pages/Storage'
import History from './pages/History'
import Alerts from './pages/Alerts'
import Settings from './pages/Settings'

export default function App() {
  return (
    <div className="min-h-screen">
      <header className="bg-white border-b border-slate-200">
        <div className="max-w-7xl mx-auto px-6 py-4 flex items-center gap-8">
          <h1 className="text-xl font-bold text-slate-800">QNAP 监控面板</h1>
          <nav className="flex gap-2 text-sm">
            <NavTab to="/">仪表盘</NavTab>
            <NavTab to="/storage">存储</NavTab>
            <NavTab to="/history">历史</NavTab>
            <NavTab to="/alerts">告警</NavTab>
            <NavTab to="/settings">设置</NavTab>
          </nav>
        </div>
      </header>
      <main className="max-w-7xl mx-auto px-6 py-6">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/storage" element={<Storage />} />
          <Route path="/history" element={<History />} />
          <Route path="/alerts" element={<Alerts />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </main>
    </div>
  )
}

function NavTab({ to, children }: { to: string; children: React.ReactNode }) {
  return (
    <NavLink
      to={to}
      end
      className={({ isActive }) =>
        `px-3 py-1.5 rounded-md ${
          isActive ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
        }`
      }
    >
      {children}
    </NavLink>
  )
}
