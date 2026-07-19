/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        space: {
          950: '#04060e',
          900: '#080d1a',
          850: '#0c1326',
          800: '#101b33',
          700: '#162440',
          600: '#1e3254',
        },
        accent: {
          DEFAULT: '#38bdf8',
          bright: '#7dd3fc',
          dim: '#0ea5e9',
          glow: 'rgba(56,189,248,0.15)',
        },
        status: {
          healthy: '#34d399',
          unhealthy: '#f87171',
          warning: '#fbbf24',
          unknown: '#94a3b8',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', '"Fira Code"', 'monospace'],
      },
      boxShadow: {
        glow: '0 0 20px rgba(56,189,248,0.2)',
        'glow-sm': '0 0 10px rgba(56,189,248,0.15)',
        'healthy': '0 0 8px rgba(52,211,153,0.4)',
        'unhealthy': '0 0 8px rgba(248,113,113,0.4)',
      },
      animation: {
        pulse_slow: 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s infinite',
      },
      keyframes: {
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
      },
    },
  },
  plugins: [],
}
