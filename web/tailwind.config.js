/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        /* 萤火量化首页：与策略构建器同一套 surface / 主色（primary-container） */
        lum: {
          bg: '#0b0b0b',
          primary: '#cafd00',
          'primary-dim': '#beee00',
          onSurface: '#f6f6fc',
          onSurfaceVariant: '#aaabb0',
          surfaceHigh: '#1c1c1c',
          surfaceLow: '#131313',
          surfaceHighest: '#1c1c1c',
          surfaceLowest: '#0b0b0b',
          error: '#ff7351',
        },
        /* Luminescent Quant 策略实验室（ai_1 / ai_2 设计稿 Tailwind 语义类） */
        'surface-container-high': '#1c1c1c',
        'surface-container-low': '#131313',
        'surface-container-lowest': '#0b0b0b',
        'surface-container-highest': '#1c1c1c',
        'surface-bright': '#1c1c1c',
        surface: '#0b0b0b',
        'on-surface': '#f6f6fc',
        'on-surface-variant': '#aaabb0',
        'outline-variant': '#46484d',
        'primary-container': '#cafd00',
        'on-primary-container': '#4a5e00',
        'primary-dim': '#beee00',
        'on-primary': '#516700',
        /** 设计稿里 text-primary / border-primary/40 */
        primary: '#f3ffca',
        error: '#ff7351',
        /* 芥末黄：与 lum.primary (#cafd00) 同系，偏青柠黄，避免币安橙 #F0B90B */
        'nofx-gold': {
          DEFAULT: '#C4CF45',
          dim: 'rgba(196, 207, 69, 0.14)',
          glow: 'rgba(196, 207, 69, 0.38)',
          highlight: '#DCE76A',
        },
        /** 交易端背景层级：略抬离纯黑减轻割裂；#0b0b0b 底 / #1c1c1c 面板 / #131313 内嵌条 */
        'nofx-bg': {
          DEFAULT: '#0b0b0b',
          secondary: '#131313',
          tertiary: '#1c1c1c',
          /** 兼容旧类名 */
          deeper: '#131313',
          lighter: '#1c1c1c',
        },
        'nofx-accent': '#00F0FF',
        'nofx-text': {
          DEFAULT: '#EAECEF',
          main: '#EAECEF',
          muted: '#848E9C',
        },
        'nofx-success': '#0ECB81',
        'nofx-danger': '#F6465D',
      },
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui'],
        mono: ['JetBrains Mono', 'Menlo', 'Monaco', 'Courier New', 'monospace'],
        lumheadline: ['"Space Grotesk"', '"Noto Sans SC"', 'ui-sans-serif', 'system-ui'],
        lumbody: ['Inter', '"Noto Sans SC"', 'ui-sans-serif', 'system-ui'],
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(circle at center, var(--tw-gradient-stops))',
        'gradient-conic': 'conic-gradient(from 180deg at 50% 50%, var(--tw-gradient-stops))',
        'scanlines': "url(\"data:image/svg+xml,%3Csvg width='4' height='4' viewBox='0 0 4 4' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M0 0H4V2H0V0Z' fill='rgba(0,0,0,0.4)'/%3E%3C/svg%3E\")",
        'grid-pattern': "linear-gradient(to right, #1f2937 1px, transparent 1px), linear-gradient(to bottom, #1f2937 1px, transparent 1px)",
      },
      animation: {
        'pulse-slow': 'pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'scan': 'scan 8s linear infinite',
        'scan-fast': 'scan 2s linear infinite',
        'float': 'float 6s ease-in-out infinite',
        'glitch': 'glitch 0.3s cubic-bezier(.25, .46, .45, .94) both infinite',
        'shimmer': 'shimmer 2s linear infinite',
      },
      keyframes: {
        scan: {
          '0%': { backgroundPosition: '0 0' },
          '100%': { backgroundPosition: '0 100%' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-10px)' },
        },
        glitch: {
          '0%': { transform: 'translate(0)' },
          '20%': { transform: 'translate(-2px, 2px)' },
          '40%': { transform: 'translate(-2px, -2px)' },
          '60%': { transform: 'translate(2px, 2px)' },
          '80%': { transform: 'translate(2px, -2px)' },
          '100%': { transform: 'translate(0)' },
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
      },
      boxShadow: {
        'neon': '0 0 5px theme("colors.nofx-gold.DEFAULT"), 0 0 20px theme("colors.nofx-gold.dim")',
        'neon-blue': '0 0 5px theme("colors.nofx-accent"), 0 0 20px rgba(0, 240, 255, 0.26)',
      },
    },
  },
  plugins: [],
}
