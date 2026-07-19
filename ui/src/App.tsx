import { TopBar } from './components/TopBar'
import { Explorer } from './pages/Explorer'
import { Doctor } from './pages/Doctor'
import { Orphan } from './pages/Orphan'
import { RBAC } from './pages/RBAC'
import { useStore } from './store/useStore'

export default function App() {
    const page = useStore(s => s.page)

    return (
        <div className="flex flex-col h-full overflow-hidden bg-space-950">
            <TopBar />
            <main className="flex-1 overflow-hidden flex">
                {page === 'explorer' && <Explorer />}
                {page === 'doctor' && <Doctor />}
                {page === 'orphan' && <Orphan />}
                {page === 'rbac' && <RBAC />}
                {page === 'events' && (
                    <div className="flex-1 flex items-center justify-center text-slate-600 text-sm">
                        Select a resource in Explorer and click the Events tab
                    </div>
                )}
            </main>
        </div>
    )
}
