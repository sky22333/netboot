import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{vue,ts}'],
  theme: {
    extend: {
      colors: {
        border: 'hsl(214.3 31.8% 91.4%)',
        background: 'hsl(0 0% 100%)',
        foreground: 'hsl(222.2 84% 4.9%)',
        muted: 'hsl(210 40% 96.1%)',
        'muted-foreground': 'hsl(215.4 16.3% 46.9%)'
      },
      borderRadius: {
        xl: '0.75rem'
      }
    }
  },
  plugins: []
} satisfies Config

