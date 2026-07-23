import { TopBar } from './components/TopBar'
import { Explorer } from './pages/Explorer'
import { Doctor } from './pages/Doctor'
import { Orphan } from './pages/Orphan'
import { RBAC } from './pages/RBAC'
import { Metrics } from './pages/Metrics'
import { Events } from './pages/Events'
import { Images } from './pages/Images'
import { Certificates } from './pages/Certificates'
import { useStore } from './store/useStore'

export default function App() {
    const page = useStore(s => s.page)

    return (
        <div className="flex flex-col h-full overflow-hidden bg-space-950">
            <TopBar />
            <main className="flex-1 overflow-hidden flex">
                {page === 'explorer'     && <Explorer />}
                {page === 'doctor'       && <Doctor />}
                {page === 'orphan'       && <Orphan />}
                {page === 'rbac'         && <RBAC />}
                {page === 'events'       && <Events />}
                {page === 'metrics'      && <Metrics />}
                {page === 'images'       && <Images />}
                {page === 'certificates' && <Certificates />}
            </main>
        </div>
    )
}

