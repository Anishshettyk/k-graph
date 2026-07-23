import { create } from 'zustand'
import type { NodeInfo } from '../types/api'

type Page = 'explorer' | 'doctor' | 'orphan' | 'rbac' | 'events' | 'metrics' | 'images' | 'certificates' | 'network' | 'images' | 'certificates'

interface Store {
    context: string
    namespace: string
    selectedNode: NodeInfo | null
    page: Page

    setContext: (ctx: string) => void
    setNamespace: (ns: string) => void
    setSelectedNode: (node: NodeInfo | null) => void
    setPage: (page: Page) => void
}

export const useStore = create<Store>((set) => ({
    context: '',
    namespace: '',
    selectedNode: null,
    page: 'explorer',

    setContext: (context) => set({ context, selectedNode: null }),
    setNamespace: (namespace) => set({ namespace, selectedNode: null }),
    setSelectedNode: (selectedNode) => set({ selectedNode }),
    setPage: (page) => set({ page }),
}))
