import { useState } from 'react'
import { Copy, Check } from 'lucide-react'

interface Props {
    yaml: string
    isLoading: boolean
    error?: string
}

// Very minimal YAML syntax highlighting — no library needed.
// Matches: key: → blue, strings → green, booleans/numbers → amber.
function highlightYAML(line: string): string {
    // Comment
    if (/^\s*#/.test(line)) {
        return `<span class="text-slate-600">${esc(line)}</span>`
    }
    // Key: value
    const keyMatch = line.match(/^(\s*)([\w.\-\/]+)(:)(\s*)(.*)?$/)
    if (keyMatch) {
        const [, indent, key, colon, space, rest] = keyMatch
        const restHtml = rest ? colorValue(rest) : ''
        return `<span class="text-slate-400">${esc(indent)}</span><span class="text-blue-400">${esc(key)}</span><span class="text-slate-500">${esc(colon)}</span>${esc(space)}${restHtml}`
    }
    // List item
    const listMatch = line.match(/^(\s*)(-)(\s+)(.*)$/)
    if (listMatch) {
        const [, indent, dash, space, rest] = listMatch
        return `${esc(indent)}<span class="text-slate-500">${esc(dash)}</span>${esc(space)}${colorValue(rest)}`
    }
    return `<span class="text-slate-300">${esc(line)}</span>`
}

function colorValue(s: string): string {
    const t = s.trim()
    if (t === 'true' || t === 'false' || t === 'null' || t === '~') {
        return `<span class="text-amber-400">${esc(s)}</span>`
    }
    if (/^-?\d[\d.]*$/.test(t)) {
        return `<span class="text-amber-300">${esc(s)}</span>`
    }
    if (/^["']/.test(t) || t.includes(': ')) {
        return `<span class="text-emerald-400">${esc(s)}</span>`
    }
    return `<span class="text-slate-300">${esc(s)}</span>`
}

function esc(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function YAMLPanel({ yaml, isLoading, error }: Props) {
    const [copied, setCopied] = useState(false)

    const copy = () => {
        navigator.clipboard.writeText(yaml).then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
        })
    }

    if (isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center text-slate-600 text-xs font-mono animate-pulse">
                fetching manifest…
            </div>
        )
    }
    if (error) {
        return <div className="p-3 text-xs text-status-unhealthy font-mono">{error}</div>
    }

    const lines = yaml.split('\n')

    return (
        <div className="flex flex-col h-full overflow-hidden">
            {/* Toolbar */}
            <div className="flex items-center gap-2 px-3 py-1.5 border-b border-space-700 bg-space-900 flex-shrink-0">
                <span className="text-[10px] text-slate-600 font-mono flex-1">
                    {lines.length} lines
                </span>
                <button onClick={copy} className="p-1 text-slate-600 hover:text-slate-300 transition-colors">
                    {copied ? <Check className="w-3.5 h-3.5 text-status-healthy" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
            </div>

            {/* YAML content */}
            <div className="flex-1 overflow-auto">
                <table className="w-full text-[11px] font-mono">
                    <tbody>
                        {lines.map((line, i) => (
                            <tr key={i} className="hover:bg-white/[0.02] group">
                                <td className="text-right text-slate-700 select-none pr-3 pl-2 w-10 group-hover:text-slate-600 border-r border-space-800">
                                    {i + 1}
                                </td>
                                <td
                                    className="pl-3 pr-4 py-px whitespace-pre leading-5"
                                    dangerouslySetInnerHTML={{ __html: highlightYAML(line) }}
                                />
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    )
}
