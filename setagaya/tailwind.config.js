/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./ui/templates/**/*.html",
    "./ui/static/js/**/*.js",
    "./src/**/*.{html,js}"
  ],
  theme: {
    extend: {
      colors: {
        'setagaya': {
          50: '#f0f9ff',
          100: '#e0f2fe',
          200: '#bae6fd',
          300: '#7dd3fc',
          400: '#38bdf8',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          800: '#1e40af',
          900: '#1e3a8a'
        },
        primary: {
          50: '#eff6ff',
          100: '#dbeafe',
          200: '#bfdbfe',
          300: '#93c5fd',
          400: '#60a5fa',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          800: '#1e40af',
          900: '#1e3a8a',
        },
        admin: {
          sidebar: '#667eea',
          'sidebar-dark': '#764ba2',
          accent: '#f093fb',
          'accent-dark': '#f5576c'
        }
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      animation: {
        'fade-in': 'fadeIn 0.5s ease-in-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'bounce-slow': 'bounce 2s infinite',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { transform: 'translateY(10px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        }
      },
      boxShadow: {
        'soft': '0 2px 15px 0 rgba(0, 0, 0, 0.08)',
        'admin': '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
      },
      spacing: {
        '18': '4.5rem',
        '88': '22rem',
      },
      minHeight: {
        'screen-minus-nav': 'calc(100vh - 4rem)',
      }
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
    require('@tailwindcss/typography'),
    function({ addUtilities }) {
      const newUtilities = {
        '.drag-over': {
          borderColor: '#3b82f6',
          backgroundColor: '#eff6ff',
        },
        '.status-indicator': {
          width: '12px',
          height: '12px',
          borderRadius: '50%',
          display: 'inline-block',
          marginRight: '8px',
        },
        '.status-online': {
          backgroundColor: '#10b981',
        },
        '.status-offline': {
          backgroundColor: '#ef4444',
        },
        '.status-warning': {
          backgroundColor: '#f59e0b',
        },
        '.admin-gradient': {
          background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
        },
        '.metric-gradient': {
          background: 'linear-gradient(135deg, #f093fb 0%, #f5576c 100%)',
        }
      }
      addUtilities(newUtilities)
    }
  ],
  darkMode: 'class', // Enable dark mode with class strategy
}